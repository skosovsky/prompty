package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const responsesFunctionFixture = `{"id":"fc_fixture","type":"function_call","call_id":"call_fixture","name":"lookup","arguments":"{\"id\":9007199254740993}","status":"completed","future":true}`

func responseToolDefinitions() []prompty.ToolDefinition {
	return []prompty.ToolDefinition{
		{
			Name: "lookup",
			Parameters: []byte(
				`{"type":"object","properties":{"id":{"type":"integer","enum":[9007199254740993]}},"required":["id"],"additionalProperties":false}`,
			),
		},
	}
}

func toolResponseFixture(w http.ResponseWriter, items string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(
		w,
		`{"id":"resp_fixture","model":"fixture","status":"completed","output":[`+items+`],"usage":{"output_tokens":3}}`,
	)
}

func localToolsFixtureAdapter(server *httptest.Server) *Responses {
	manager := hostedFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityTools] = prompty.CapabilitySupported
	manager.base.profiles[0].Capabilities[prompty.CapabilityForcedToolSelection] = prompty.CapabilitySupported
	return manager
}

func TestResponsesNativeFunctionsSDKAndOriginalSchemaRebinding(t *testing.T) {
	t.Parallel()
	// Arrange: named selection plus hosted search, and exact original arguments.
	requests := make(chan []byte, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- body
		toolResponseFixture(w, responsesFunctionFixture)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	exec := prompty.SimplePrompt("lookup")
	exec.Tools, exec.ForcedTool = responseToolDefinitions(), "lookup"
	// Act.
	created, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: withResponseHostedTools(exec,
			[]prompty.HostedToolRequest{{Kind: HostedWebSearch}}), Store: new(true), Background: true,
	})
	require.NoError(t, err)
	document, err := prompty.EncodeRemoteResponseHandle(*created.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeRemoteResponseHandle(document)
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(created.Response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	manager.base.profiles[0].Capabilities[prompty.CapabilityTools] = prompty.CapabilityUnsupported
	manager.base.profiles[0].Capabilities[prompty.CapabilityForcedToolSelection] = prompty.CapabilityUnsupported
	observed, err := manager.Get(t.Context(), handle, prompty.RemoteResponseOperationOptions{Tools: exec.Tools})
	require.NoError(t, err)
	canceled, err := manager.Cancel(
		t.Context(),
		*observed.Handle,
		prompty.RemoteResponseOperationOptions{Tools: exec.Tools},
	)
	// Assert: retrieval does not reauthorize or replay generation.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeToolCalls, observed.Response.Outcome)
	require.Equal(t, created.Response.Content, canceled.Response.Content)
	require.Equal(t, created.Response.Content, restored.Content)
	require.Equal(t, created.Response.ProviderState, restored.ProviderState)
	require.Equal(
		t,
		prompty.ToolCallPart{ID: "call_fixture", Name: "lookup", Args: `{"id":9007199254740993}`},
		created.Response.Content[0],
	)
	require.Len(t, created.Response.ProviderState, 1)
	require.Equal(t, responsesFunctionCodec, created.Response.ProviderState[0].Envelope.Codec)
	require.True(t, created.Response.ProviderState[0].Envelope.Required)
	require.Contains(t, string(created.Response.ProviderState[0].Envelope.Payload), `"future":true`)
	require.Len(t, handle.ToolSchemaDigests, 1)
	require.Equal(t, "lookup", handle.ForcedTool)
	var request struct {
		Tools  []json.RawMessage `json:"tools"`
		Choice json.RawMessage   `json:"tool_choice"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.Len(t, request.Tools, 2)
	require.Contains(t, string(request.Tools[0]), "web_search")
	require.Contains(t, string(request.Tools[1]), `"strict":false`)
	require.Contains(t, string(request.Tools[1]), "9007199254740993")
	require.JSONEq(t, `{"type":"function","name":"lookup"}`, string(request.Choice))
	require.Empty(t, <-requests)
	require.Empty(t, <-requests)
	require.Contains(t, string(exec.Tools[0].Parameters), "9007199254740993")
}

func TestResponsesFunctionsMalformedAndSelectionFailuresRetainKnownOutput(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, items, forced string
		incomplete          bool
	}{
		{"invalid_enum", strings.Replace(responsesFunctionFixture, "9007199254740993", "1", 1), "", true},
		{"unknown_name", strings.Replace(responsesFunctionFixture, `"lookup"`, `"unknown"`, 1), "", true},
		{"duplicate_call_id", responsesFunctionFixture + "," + strings.Replace(responsesFunctionFixture, "fc_fixture", "fc_other", 1), "", true},
		{"unfinished", strings.Replace(responsesFunctionFixture, `"completed"`, `"incomplete"`, 1), "", true},
		{"malformed_args", strings.Replace(responsesFunctionFixture, `{\"id\":9007199254740993}`, `bad`, 1), "", true},
		{"forced_text", `{"id":"msg_fixture","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"not a tool","annotations":[]}]}`, "lookup", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { toolResponseFixture(w, test.items) }),
			)
			defer server.Close()
			exec := prompty.SimplePrompt("lookup")
			exec.Tools, exec.ForcedTool = responseToolDefinitions(), test.forced
			// Act.
			known, err := localToolsFixtureAdapter(
				server,
			).Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
			// Assert.
			require.Error(t, err)
			require.Equal(t, "resp_fixture", known.Handle.ProviderID)
			require.NotEmpty(t, known.Response.Content)
			if test.incomplete {
				require.Equal(t, prompty.OutcomeIncomplete, known.Response.Outcome)
			}
		})
	}
}

func TestResponsesFunctionPreflightAndRebindingZeroIO(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		toolResponseFixture(w, responsesFunctionFixture)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	base := prompty.SimplePrompt("lookup")
	base.Tools = responseToolDefinitions()
	for _, edit := range []func(*prompty.PromptExecution){
		func(exec *prompty.PromptExecution) { exec.Tools = append(exec.Tools, exec.Tools[0]) },
		func(exec *prompty.PromptExecution) { exec.Tools[0].Name = " " },
		func(exec *prompty.PromptExecution) { exec.Tools[0].Parameters = []byte(`{"type":7}`) },
		func(exec *prompty.PromptExecution) { exec.RequiredTools = []string{"undeclared"} },
		func(exec *prompty.PromptExecution) { exec.ForcedTool = "undeclared" },
	} {
		// Act/Assert.
		exec := base.Clone()
		edit(exec)
		_, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
		require.Error(t, err)
		require.Zero(t, calls.Load())
	}
	manager.base.profiles[0].Capabilities[prompty.CapabilityTools] = prompty.CapabilityUnknown
	_, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: base, Store: new(true)})
	require.ErrorIs(t, err, adapter.ErrUnknownProfile)
	manager.base.profiles[0].Capabilities[prompty.CapabilityTools] = prompty.CapabilitySupported
	accepted, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: base, Store: new(true)})
	require.NoError(t, err)
	_, err = manager.Get(t.Context(), *accepted.Handle)
	require.Error(t, err)
	changed := responseToolDefinitions()
	changed[0].Parameters = []byte(`{"type":"object"}`)
	_, err = manager.Get(t.Context(), *accepted.Handle, prompty.RemoteResponseOperationOptions{Tools: changed})
	require.ErrorIs(t, err, prompty.ErrRemoteResponseScope)
	changedDialect := responseToolDefinitions()
	changedDialect[0].Dialect = prompty.SchemaDialect7
	_, err = manager.Get(t.Context(), *accepted.Handle, prompty.RemoteResponseOperationOptions{Tools: changedDialect})
	require.ErrorIs(t, err, prompty.ErrRemoteResponseScope)
	require.EqualValues(t, 1, calls.Load())
}

func TestResponsesFunctionOptionalItemIDDoesNotReplaceCallID(t *testing.T) {
	t.Parallel()
	// Arrange: the native item ID is optional, while call_id remains required.
	item := strings.Replace(responsesFunctionFixture, `"id":"fc_fixture",`, "", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolResponseFixture(w, item)
	}))
	defer server.Close()
	exec := prompty.SimplePrompt("lookup")
	exec.Tools = responseToolDefinitions()
	// Act.
	result, err := localToolsFixtureAdapter(
		server,
	).Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(false)})
	// Assert: no item identity is fabricated from call correlation.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeToolCalls, result.Response.Outcome)
	require.Equal(t, "call_fixture", result.Response.Content[0].(prompty.ToolCallPart).ID)
	require.NotContains(t, string(result.Response.ProviderState[0].Envelope.Payload), "fc_fixture")
}
