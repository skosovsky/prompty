package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func responseFixtureAdapter(server *httptest.Server) *Responses {
	client := sdk.NewClient(option.WithAPIKey("fixture-secret"), option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()))
	capabilities := make(map[string]prompty.CapabilitySupport)
	for _, capability := range []string{prompty.CapabilityResponseCreate, prompty.CapabilityResponseRetrieve,
		prompty.CapabilityResponseCancel, prompty.CapabilityResponseContinuation, prompty.CapabilityResponseBackground,
		prompty.CapabilityStructuredOutput} {
		capabilities[capability] = prompty.CapabilitySupported
	}
	return NewResponses("host-account", WithClient(&client), WithModel("fixture"), WithProfiles(
		server.URL+"/v1",
		prompty.CapabilityProfile{
			Identity: prompty.ProfileIdentity{
				Provider: "openai.responses",
				Endpoint: server.URL + "/v1",
				Model:    "fixture",
			},
			Provenance:   "actual SDK local fixture, not live model support",
			Capabilities: capabilities,
		},
	))
}

func remoteFixtureHandle(adp *Responses) prompty.RemoteResponseHandle {
	return prompty.RemoteResponseHandle{
		Format:         prompty.RemoteResponseWireFormat,
		Identity:       adp.RuntimeDescriptor().Profiles[0].Identity,
		AccountBinding: "host-account",
		ProviderID:     "resp_fixture",
		Status:         prompty.RemoteResponseQueued,
		Background:     true, Store: true,
	}
}

func writeResponseFixture(w http.ResponseWriter, status, text string) {
	w.Header().Set("Content-Type", "application/json")
	if status == "queued" || status == "in_progress" {
		_, _ = fmt.Fprintf(w, `{"id":"resp_fixture","model":"fixture","status":%q,"output":[],"usage":null}`, status)
		return
	}
	_, _ = fmt.Fprintf(
		w,
		`{"id":"resp_fixture","model":"fixture","status":%q,"output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":%q,"annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":0}}}`,
		status,
		text,
	)
}

func TestResponsesActualSDKLifecycleAndContinuation(t *testing.T) {
	t.Parallel()
	// Arrange: independently observed states, then explicit next-turn settings.
	var calls atomic.Int32
	requests := make(chan string, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- request.Method + " " + request.URL.Path + " " + string(body)
		call := calls.Add(1)
		status := "completed"
		if call == 1 {
			status = "queued"
		}
		if call == 2 {
			status = "in_progress"
		}
		writeResponseFixture(w, status, "answer")
	}))
	defer server.Close()
	adp := responseFixtureAdapter(server)
	exec := prompty.NewExecution([]prompty.ChatMessage{
		{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "first instructions"}}},
		{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "document"}}},
	})
	original := exec.Clone()
	// Act.
	created, err := adp.Create(
		t.Context(),
		prompty.RemoteResponseRequest{Execution: exec, Background: true, Store: new(true)},
	)
	require.NoError(t, err)
	wire, err := prompty.EncodeRemoteResponseHandle(*created.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeRemoteResponseHandle(wire)
	require.NoError(t, err)
	running, err := adp.Get(t.Context(), handle)
	require.NoError(t, err)
	completed, err := adp.Get(t.Context(), *running.Handle)
	require.NoError(t, err)
	canceled, err := adp.Cancel(t.Context(), *completed.Handle)
	require.NoError(t, err)
	next, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("clarify"), Previous: canceled.Handle, Store: new(false),
	})
	// Assert: cancel race retains completed, no implicit instructions/tool inheritance.
	require.NoError(t, err)
	require.Equal(t, prompty.RemoteResponseQueued, created.Handle.Status)
	require.Equal(t, prompty.RemoteResponseRunning, running.Handle.Status)
	require.Equal(t, prompty.RemoteResponseCompleted, completed.Handle.Status)
	require.Equal(t, prompty.RemoteResponseCompleted, canceled.Handle.Status)
	require.False(t, next.Handle.Store)
	require.Equal(t, original, exec)
	require.Equal(t, prompty.UsageUnknown, created.Response.Usage.Availability())
	require.Equal(t, 12, completed.Response.Usage.TotalTokens)
	require.Equal(t, 3, completed.Response.Usage.PromptTokensCached)
	require.Contains(t, completed.Response.Usage.Known, prompty.UsageReasoning)
	require.Zero(t, completed.Response.Usage.CompletionTokensReasoning)
	first := <-requests
	require.Contains(t, first, "POST /v1/responses ")
	require.Contains(t, first, `"instructions":"first instructions"`)
	require.Contains(t, first, `"background":true`)
	require.Contains(t, <-requests, "GET /v1/responses/resp_fixture ")
	require.Contains(t, <-requests, "GET /v1/responses/resp_fixture ")
	require.Contains(t, <-requests, "POST /v1/responses/resp_fixture/cancel ")
	last := <-requests
	require.Contains(t, last, `"previous_response_id":"resp_fixture"`)
	require.Contains(t, last, `"instructions":""`)
	require.Contains(t, last, `"store":false`)
	require.NotContains(t, last, "first instructions")
	require.NotContains(t, last, "chat/completions")
	require.EqualValues(t, 5, calls.Load())
}

func TestResponsesUnknownAcceptanceNoRetry(t *testing.T) {
	t.Parallel()
	for _, cancel := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancel), func(t *testing.T) {
			t.Parallel()
			// Arrange: remote acceptance is unknowable; SDK would ordinarily retry 500.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error":{"message":"provider-secret"}}`)
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			handle := remoteFixtureHandle(adp)
			// Act.
			var result *prompty.RemoteResponseResult
			var err error
			if cancel {
				result, err = adp.Cancel(t.Context(), handle)
			} else {
				result, err = adp.Create(t.Context(), prompty.RemoteResponseRequest{
					Execution: prompty.SimplePrompt("document"), Store: new(true),
				})
			}
			// Assert: one dispatch, safe error, no invented ID; known cancel identity retained.
			require.ErrorIs(t, err, prompty.ErrRemoteResponseUnknown)
			require.NotContains(t, err.Error(), "provider-secret")
			require.EqualValues(t, 1, calls.Load())
			if cancel {
				require.Equal(t, handle.ProviderID, result.Handle.ProviderID)
				require.Equal(t, prompty.RemoteResponseUnknown, result.Handle.Status)
				require.Equal(t, prompty.RemoteResponseQueued, handle.Status)
			} else {
				require.Nil(t, result.Handle)
			}
		})
	}
}

func TestResponsesScopeAndCapabilityPreflight(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseFixture(w, "completed", "answer")
	}))
	defer server.Close()
	adp := responseFixtureAdapter(server)
	for _, mutate := range []func(*prompty.RemoteResponseHandle){
		func(h *prompty.RemoteResponseHandle) { h.Identity.Model = "foreign" },
		func(h *prompty.RemoteResponseHandle) { h.Identity.Endpoint = "https://foreign.invalid/v1" },
		func(h *prompty.RemoteResponseHandle) { h.AccountBinding = "foreign" },
		func(h *prompty.RemoteResponseHandle) { h.ProviderID = "resp_fixture/../secret" },
		func(h *prompty.RemoteResponseHandle) { h.Identity.Provider = "openai" },
	} {
		handle := remoteFixtureHandle(adp)
		mutate(&handle)
		// Act/Assert.
		_, err := adp.Get(t.Context(), handle)
		require.Error(t, err)
	}
	for _, capability := range []string{prompty.CapabilityResponseCreate, prompty.CapabilityResponseBackground, prompty.CapabilityResponseContinuation} {
		profile := adp.RuntimeDescriptor().Profiles[0]
		profile.Capabilities[capability] = prompty.CapabilityUnsupported
		isolated := NewResponses(
			"host-account",
			WithClient(adp.base.client),
			WithModel("fixture"),
			WithProfiles(server.URL+"/v1", profile),
		)
		handle := remoteFixtureHandle(adp)
		handle.Status = prompty.RemoteResponseCompleted
		_, err := isolated.Create(
			t.Context(),
			prompty.RemoteResponseRequest{
				Execution:  prompty.SimplePrompt("question"),
				Store:      new(true),
				Background: true,
				Previous:   &handle,
			},
		)
		require.ErrorIs(t, err, adapter.ErrUnsupportedCapability)
	}
	_, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("question")})
	require.ErrorIs(t, err, prompty.ErrRemoteResponse)
	require.Zero(t, calls.Load())
}

func TestResponsesUnavailableAndCanceledContextRetainObservation(t *testing.T) {
	t.Parallel()
	// Arrange: failed retrieval is not proof of a different execution state.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	adp := responseFixtureAdapter(server)
	handle := remoteFixtureHandle(adp)
	// Act/Assert.
	result, err := adp.Get(t.Context(), handle)
	require.ErrorIs(t, err, prompty.ErrRemoteResponseUnavailable)
	require.Equal(t, handle, *result.Handle)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = adp.Create(
		ctx,
		prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("question"), Store: new(true)},
	)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 1, calls.Load())
}

func TestResponsesAcceptedMalformedSourceRetainsIdentity(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"id":"resp_fixture","model":"fixture","status":"unrecognized","output":[]}`,
		`{"id":"resp_fixture","model":"fixture","status":"completed","status":"queued","output":[]}`,
		`{"id":"resp_fixture","model":"foreign","status":"completed","output":[]}`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			// Act.
			result, err := adp.Create(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("question"), Store: new(true)},
			)
			// Assert: available identity is reconciliation evidence, not usable success.
			require.ErrorIs(t, err, prompty.ErrRemoteResponseUnknown)
			require.NotNil(t, result.Handle)
			require.Equal(t, "resp_fixture", result.Handle.ProviderID)
			require.Equal(t, prompty.RemoteResponseUnknown, result.Handle.Status)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestResponsesOriginalOutputSchemaAndRetrievalBinding(t *testing.T) {
	t.Parallel()
	// Arrange: a required native schema with a literal beyond float exactness.
	var calls atomic.Int32
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(request.Body)
		requests <- body
		writeResponseFixture(w, "completed", `{"id":9007199254740993}`)
	}))
	defer server.Close()
	adp := responseFixtureAdapter(server)
	exec := prompty.SimplePrompt("extract")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Name: "answer",
		Schema: []byte(
			`{"type":"object","properties":{"id":{"type":"integer","enum":[9007199254740993]}},"required":["id"],"additionalProperties":false}`,
		),
	}
	// Act.
	created, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
	require.NoError(t, err)
	_, err = adp.Get(t.Context(), *created.Handle)
	// Assert: missing/mismatching original schema fails before retrieval.
	require.ErrorIs(t, err, prompty.ErrSchemaValidatorRequired)
	changed := *exec.ResponseFormat
	changed.Schema = []byte(strings.Replace(string(changed.Schema), "9007199254740993", "9007199254740992", 1))
	_, err = adp.Get(t.Context(), *created.Handle, prompty.RemoteResponseOperationOptions{OutputSchema: &changed})
	require.ErrorIs(t, err, prompty.ErrRemoteResponseScope)
	changedDialect := *exec.ResponseFormat
	changedDialect.Dialect = prompty.SchemaDialect7
	_, err = adp.Get(
		t.Context(),
		*created.Handle,
		prompty.RemoteResponseOperationOptions{OutputSchema: &changedDialect},
	)
	require.ErrorIs(t, err, prompty.ErrRemoteResponseScope)
	changedName := *exec.ResponseFormat
	changedName.Name = "different"
	_, err = adp.Get(t.Context(), *created.Handle, prompty.RemoteResponseOperationOptions{OutputSchema: &changedName})
	require.ErrorIs(t, err, prompty.ErrRemoteResponseScope)
	retrieved, err := adp.Get(
		t.Context(),
		*created.Handle,
		prompty.RemoteResponseOperationOptions{OutputSchema: exec.ResponseFormat},
	)
	require.NoError(t, err)
	require.Equal(t, created.Response.Content, retrieved.Response.Content)
	var request map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.Contains(t, string(request["text"]), "9007199254740993")
	require.Contains(t, string(request["text"]), `"strict":true`)
	require.EqualValues(t, 2, calls.Load())
}

func TestResponsesTerminalOutcomesAndOriginalValidation(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"failed", "incomplete", "cancelled", "completed"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			// Arrange: valid JSON is not schema success or successful remote completion.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeResponseFixture(w, status, `{"id":"invalid"}`)
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			exec := prompty.SimplePrompt("extract")
			exec.ResponseFormat = &prompty.SchemaDefinition{
				Name: "answer",
				Schema: []byte(
					`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`,
				),
			}
			// Act.
			result, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
			// Assert: partial data/known identity/usage retained, no repair or second request.
			require.Error(t, err)
			if status != "completed" {
				require.ErrorIs(t, err, prompty.ErrResponseOutcome)
			}
			if status == "cancelled" {
				require.Equal(t, prompty.RemoteResponseCanceled, result.Handle.Status)
			}
			require.JSONEq(t, `{"id":"invalid"}`, result.Response.Content[0].(prompty.TextPart).Text)
			require.Equal(t, 12, result.Response.Usage.TotalTokens)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
