package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const responseReasoningFixture = `{"id":"rs_fixture","type":"reasoning","summary":[{"type":"summary_text","text":"First"},{"type":"summary_text","text":"Second"}],"encrypted_content":"opaque-secret","status":"completed","future":{"number":9007199254740993},"content":[{"type":"reasoning_text","text":"private-content"}]}`

func writeReasoningResponse(w http.ResponseWriter, reasoning string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"id":"resp_fixture","model":"fixture","status":"completed","output":[`+reasoning+
		`,{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"output_tokens":4,"output_tokens_details":{"reasoning_tokens":3}}}`)
}

func reasoningFixtureAdapter(server *httptest.Server) *Responses {
	adp := responseFixtureAdapter(server)
	adp.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	return adp
}

func TestResponsesReasoningActualSDKWireAndStatelessContinuation(t *testing.T) {
	t.Parallel()
	// Arrange.
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- body
		writeReasoningResponse(w, responseReasoningFixture)
	}))
	defer server.Close()
	adp := reasoningFixtureAdapter(server)
	exec := prompty.SimplePrompt("initial")
	exec.Requirements.Capabilities = []string{prompty.CapabilityProviderState}
	// Act.
	first, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(false)})
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(first.Response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	message := prompty.ChatMessage{
		Role: prompty.RoleAssistant, Content: restored.Content, ProviderState: restored.ProviderState,
	}
	next := prompty.SimplePrompt("next")
	next.Messages = append([]prompty.ChatMessage{message}, next.Messages...)
	_, err = adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: next, Store: new(false)})
	// Assert.
	require.NoError(t, err)
	require.Equal(t, []prompty.ContentPart{
		prompty.ReasoningPart{Text: "First\n\nSecond"}, prompty.TextPart{Text: "answer"},
	}, restored.Content)
	require.Len(t, restored.ProviderState, 1)
	//nolint:testifylint // Opaque source must remain byte-identical; JSONEq would miss re-encoding.
	require.Equal(t, []byte(responseReasoningFixture), restored.ProviderState[0].Envelope.Payload)
	require.Equal(t, adp.RuntimeDescriptor().Profiles[0].Identity, restored.ProviderState[0].Envelope.Scope)
	require.True(t, restored.ProviderState[0].Envelope.Required)
	require.Empty(t, restored.Annotations)
	require.Contains(t, restored.Usage.Known, prompty.UsageReasoning)
	require.NotContains(t, restored.Usage.Known, prompty.UsagePrompt)
	for range 2 {
		request := <-requests
		require.Contains(t, string(request), `"reasoning.encrypted_content"`)
		require.Contains(t, string(request), `"store":false`)
		if strings.Contains(string(request), "rs_fixture") {
			var body struct {
				Input []json.RawMessage `json:"input"`
			}
			require.NoError(t, json.Unmarshal(request, &body))
			require.Len(t, body.Input, 3)
			require.JSONEq(t, responseReasoningFixture, string(body.Input[0]))
			require.Contains(t, string(body.Input[0]), "9007199254740993")
			require.Contains(t, string(body.Input[1]), "answer")
			require.Contains(t, string(body.Input[2]), "next")
		}
	}
	public, err := prompty.MarshalResponse(restored, prompty.WirePolicy{State: prompty.StateOmit})
	require.NoError(t, err)
	publicJSON, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(publicJSON), "opaque-secret")
	require.NotContains(t, string(publicJSON), "private-content")
	require.True(t, public.ContinuationUnavailable)
}

func TestResponsesReasoningReceivedShapeAndMissingState(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		item        string
		state       bool
		unavailable bool
		incomplete  bool
		failure     bool
	}{
		{"empty_summary", `{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":"opaque-secret"}`, true, false, false, false},
		{"absent_encrypted", `{"id":"rs_fixture","type":"reasoning","summary":[]}`, false, true, false, false},
		{"null_encrypted", `{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":null}`, false, true, false, false},
		{"empty_encrypted", `{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":""}`, false, true, false, false},
		{"unfinished", strings.Replace(responseReasoningFixture, `"completed"`, `"in_progress"`, 1), true, false, true, true},
		{"summary_missing", `{"id":"rs_fixture","type":"reasoning","encrypted_content":"opaque-secret"}`, false, false, true, true},
		{"summary_null", `{"id":"rs_fixture","type":"reasoning","summary":null}`, false, false, true, true},
		{"summary_text_null", `{"id":"rs_fixture","type":"reasoning","summary":[{"type":"summary_text","text":null}]}`, false, false, true, true},
		{"status_unknown", strings.Replace(responseReasoningFixture, `"completed"`, `"new_status"`, 1), false, false, true, true},
		{"encrypted_number", `{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":3}`, false, false, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeReasoningResponse(w, test.item)
			}))
			defer server.Close()
			adp := reasoningFixtureAdapter(server)
			// Act.
			result, err := adp.Create(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("input"), Store: new(false)},
			)
			// Assert.
			if test.failure {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NotNil(t, result.Handle)
			require.Equal(t, test.unavailable, result.Response.ContinuationUnavailable)
			require.Equal(t, test.state, len(result.Response.ProviderState) == 1)
			if test.incomplete {
				require.Equal(t, prompty.OutcomeIncomplete, result.Response.Outcome)
			}
			if test.unavailable {
				exec := prompty.SimplePrompt("input")
				exec.Requirements.Capabilities = []string{prompty.CapabilityProviderState}
				known, stateErr := adp.Create(
					t.Context(),
					prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
				)
				require.ErrorIs(t, stateErr, prompty.ErrStateUnavailable)
				require.NotNil(t, known.Response)
			}
		})
	}
}

func TestResponsesReasoningContinuationRejectsBeforeIO(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseFixture(w, "completed", "should not run")
	}))
	t.Cleanup(server.Close)
	adp := reasoningFixtureAdapter(server)
	base := prompty.SimplePrompt("next")
	base.Messages = []prompty.ChatMessage{{
		Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.ReasoningPart{Text: "First\n\nSecond"}},
		ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
			Format: prompty.ProviderStateFormat, Scope: adp.RuntimeDescriptor().Profiles[0].Identity,
			Codec: responsesReasoningCodec, Required: true, Payload: []byte(responseReasoningFixture),
		}}},
	}}
	for _, test := range []struct {
		name string
		edit func(*prompty.PromptExecution)
	}{
		{"edited_summary", func(exec *prompty.PromptExecution) {
			exec.Messages[0].Content[0] = prompty.ReasoningPart{Text: "edited"}
		}},
		{"missing_state", func(exec *prompty.PromptExecution) { exec.Messages[0].ProviderState = nil }},
		{"wrong_scope", func(exec *prompty.PromptExecution) {
			exec.Messages[0].ProviderState[0].Envelope.Scope.Endpoint = "https://other.example"
		}},
		{"not_required", func(exec *prompty.PromptExecution) { exec.Messages[0].ProviderState[0].Envelope.Required = false }},
		{"wrong_codec", func(exec *prompty.PromptExecution) { exec.Messages[0].ProviderState[0].Envelope.Codec = "other" }},
		{"expired", func(exec *prompty.PromptExecution) {
			past := time.Now().Add(-time.Hour)
			exec.Messages[0].ProviderState[0].Envelope.ExpiresAt = &past
		}},
		{"malformed", func(exec *prompty.PromptExecution) {
			exec.Messages[0].ProviderState[0].Envelope.Payload = []byte(`{"type":"reasoning"}`)
		}},
		{"unfinished", func(exec *prompty.PromptExecution) {
			exec.Messages[0].ProviderState[0].Envelope.Payload = []byte(strings.Replace(responseReasoningFixture, `"completed"`, `"incomplete"`, 1))
		}},
		{"non_assistant", func(exec *prompty.PromptExecution) { exec.Messages[0].Role = prompty.RoleUser }},
		{"edited_type", func(exec *prompty.PromptExecution) {
			exec.Messages[0].Content[0] = prompty.TextPart{Text: "First\n\nSecond"}
		}},
		{"state_omitted", func(exec *prompty.PromptExecution) { exec.Messages[0].ContinuationUnavailable = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Act.
			exec := base.Clone()
			test.edit(exec)
			_, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(false)})
			// Assert.
			require.Error(t, err)
			require.NotContains(t, err.Error(), "opaque-secret")
			require.Zero(t, calls.Load())
		})
	}
	// A caller override cannot grant missing endpoint/model state support.
	unknown := reasoningFixtureAdapter(server)
	unknown.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilityUnknown
	_, err := unknown.Create(t.Context(), prompty.RemoteResponseRequest{Execution: base, Store: new(false)})
	require.ErrorIs(t, err, adapter.ErrUnknownProfile)
	require.Zero(t, calls.Load())
}

func TestResponsesReasoningOriginalSchemaAndGet(t *testing.T) {
	t.Parallel()
	// Arrange: summary is not a JSON answer; only native output_text is validated.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"resp_fixture","model":"fixture","status":"completed","output":[`+responseReasoningFixture+
				`,{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"id\":9007199254740993}","annotations":[]}]}]}`,
		)
	}))
	defer server.Close()
	adp := reasoningFixtureAdapter(server)
	exec := prompty.SimplePrompt("extract")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Name: "answer",
		Schema: []byte(
			`{"type":"object","properties":{"id":{"type":"integer","enum":[9007199254740993]}},"required":["id"],"additionalProperties":false}`,
		),
	}
	// Act.
	created, err := adp.Create(
		t.Context(),
		prompty.RemoteResponseRequest{Execution: exec, Store: new(true), Background: true},
	)
	require.NoError(t, err)
	// Already-produced output requires original validation, not generation support.
	adp.base.profiles[0].Capabilities[prompty.CapabilityStructuredOutput] = prompty.CapabilityUnsupported
	observed, err := adp.Get(
		t.Context(),
		*created.Handle,
		prompty.RemoteResponseOperationOptions{OutputSchema: exec.ResponseFormat},
	)
	require.NoError(t, err)
	canceled, err := adp.Cancel(
		t.Context(),
		*observed.Handle,
		prompty.RemoteResponseOperationOptions{OutputSchema: exec.ResponseFormat},
	)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeCompleted, observed.Response.Outcome)
	require.Equal(t, created.Response.Content, observed.Response.Content)
	require.Equal(t, created.Response.ProviderState, observed.Response.ProviderState)
	require.Len(t, observed.Response.ProviderState, 1)
	require.Empty(t, observed.Response.Usage.Known)
	require.Equal(t, observed.Response.Content, canceled.Response.Content)
	require.Equal(t, prompty.RemoteResponseCompleted, canceled.Handle.Status)
}

func TestResponsesReasoningFailureRetainsEarlierText(t *testing.T) {
	t.Parallel()
	// Arrange: malformed reasoning follows already observed canonical output.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"resp_fixture","model":"fixture","status":"completed","output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"prefix","annotations":[]}]},{"id":"rs_fixture","type":"reasoning","summary":null}]}`,
		)
	}))
	defer server.Close()
	// Act.
	result, err := reasoningFixtureAdapter(server).Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("input"), Store: new(false),
	})
	// Assert.
	require.ErrorIs(t, err, prompty.ErrProviderState)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "prefix"}}, result.Response.Content)
	require.Equal(t, prompty.OutcomeIncomplete, result.Response.Outcome)
	require.Equal(t, "resp_fixture", result.Handle.ProviderID)
}
