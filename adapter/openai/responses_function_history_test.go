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
)

func TestResponsesFunctionHistorySDKWireAndPreviousResults(t *testing.T) {
	t.Parallel()
	// Arrange: two calls of one function, with reasoning and text between items.
	firstCall := strings.Replace(responsesFunctionFixture, "call_fixture", "call_a", 1)
	secondCall := strings.Replace(
		strings.Replace(responsesFunctionFixture, "call_fixture", "call_b", 1),
		"fc_fixture",
		"fc_second",
		1,
	)
	text := `{"id":"msg_between","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"between","annotations":[]}]}`
	requests := make(chan []byte, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- body
		if strings.Contains(string(body), "function_call_output") {
			writeResponseFixture(w, "completed", "fresh answer")
			return
		}
		toolResponseFixture(w, responseReasoningFixture+","+firstCall+","+text+","+secondCall)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	exec := prompty.SimplePrompt("initial")
	exec.Tools = responseToolDefinitions()
	// Act.
	created, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(created.Response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	handleJSON, err := prompty.EncodeRemoteResponseHandle(*created.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeRemoteResponseHandle(handleJSON)
	require.NoError(t, err)
	results := prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{
		prompty.ToolResultPart{
			ToolCallID: "call_b",
			Name:       "lookup",
			Content:    []prompty.ContentPart{prompty.TextPart{Text: "first"}, prompty.TextPart{Text: "second"}},
		},
		prompty.ToolResultPart{
			ToolCallID: "call_a",
			Content:    []prompty.ContentPart{prompty.TextPart{Text: `{"value":9007199254740993}`}},
		},
	}}
	stateless := prompty.NewExecution([]prompty.ChatMessage{
		{Role: prompty.RoleAssistant, Content: restored.Content, ProviderState: restored.ProviderState}, results,
		{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "new question"}}},
	})
	continued, err := manager.Create(
		t.Context(),
		prompty.RemoteResponseRequest{Execution: stateless, Store: new(false)},
	)
	require.NoError(t, err)
	previousOnly := prompty.NewExecution([]prompty.ChatMessage{results})
	remote, err := manager.Create(
		t.Context(),
		prompty.RemoteResponseRequest{Execution: previousOnly, Previous: &handle, Store: new(true)},
	)
	// Assert: original items, order and explicit empty current declarations survive.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeCompleted, continued.Response.Outcome)
	require.Equal(t, prompty.OutcomeCompleted, remote.Response.Outcome)
	require.Equal(t, map[string]string{"call_a": "lookup", "call_b": "lookup"}, handle.ToolCalls)
	require.Empty(t, remote.Handle.ToolCalls)
	<-requests
	var input struct {
		Items []json.RawMessage `json:"input"`
		Tools []json.RawMessage `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &input))
	require.Len(t, input.Items, 7)
	require.JSONEq(t, responseReasoningFixture, string(input.Items[0]))
	require.JSONEq(t, firstCall, string(input.Items[1]))
	require.Contains(t, string(input.Items[2]), "between")
	require.JSONEq(t, secondCall, string(input.Items[3]))
	require.Contains(t, string(input.Items[4]), `"call_id":"call_b"`)
	require.Contains(t, string(input.Items[4]), `"output":[`)
	require.Contains(t, string(input.Items[4]), "first")
	require.Contains(t, string(input.Items[4]), "second")
	require.Contains(t, string(input.Items[5]), "9007199254740993")
	require.Contains(t, string(input.Items[6]), "new question")
	require.Empty(t, input.Tools)
	last := string(<-requests)
	require.Contains(t, last, `"previous_response_id":"resp_fixture"`)
	require.NotContains(t, last, `"type":"function_call"`)
	require.NotContains(t, last, "reasoning.encrypted_content")
	require.Contains(t, last, `"tools":[]`)
	require.Equal(t, map[string]string{"call_a": "lookup", "call_b": "lookup"}, created.Handle.ToolCalls)
}

func TestResponsesFunctionHistoryInvalidReferencesRejectBeforeIO(t *testing.T) {
	t.Parallel()
	// Arrange: a checked original item and a matching explicit result.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseFixture(w, "completed", "unexpected")
	}))
	t.Cleanup(server.Close)
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	base := prompty.NewExecution([]prompty.ChatMessage{
		{
			Role: prompty.RoleAssistant,
			Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "call_fixture", Name: "lookup", Args: `{"id":9007199254740993}`},
			},
			ProviderState: []prompty.PartState{
				{
					PartIndex: 0,
					Envelope: prompty.ProviderEnvelope{
						Format:   prompty.ProviderStateFormat,
						Scope:    manager.RuntimeDescriptor().Profiles[0].Identity,
						Codec:    responsesFunctionCodec,
						Required: true,
						Payload:  []byte(responsesFunctionFixture),
					},
				},
			},
		},
		{
			Role: prompty.RoleTool,
			Content: []prompty.ContentPart{
				prompty.ToolResultPart{
					ToolCallID: "call_fixture",
					Name:       "lookup",
					Content:    []prompty.ContentPart{prompty.TextPart{Text: "result"}},
				},
			},
		},
	})
	for _, test := range []struct {
		name string
		edit func(*prompty.PromptExecution)
	}{
		{"edited_args", func(exec *prompty.PromptExecution) {
			call := exec.Messages[0].Content[0].(prompty.ToolCallPart)
			call.Args = `{"id":0}`
			exec.Messages[0].Content[0] = call
		}},
		{"missing_state", func(exec *prompty.PromptExecution) { exec.Messages[0].ProviderState = nil }},
		{"unfinished", func(exec *prompty.PromptExecution) {
			exec.Messages[0].ProviderState[0].Envelope.Payload = []byte(strings.Replace(responsesFunctionFixture, "completed", "incomplete", 1))
		}},
		{"unknown_result", func(exec *prompty.PromptExecution) {
			result := exec.Messages[1].Content[0].(prompty.ToolResultPart)
			result.ToolCallID = "unknown"
			exec.Messages[1].Content[0] = result
		}},
		{"wrong_name", func(exec *prompty.PromptExecution) {
			result := exec.Messages[1].Content[0].(prompty.ToolResultPart)
			result.Name = "other"
			exec.Messages[1].Content[0] = result
		}},
		{"duplicate_result", func(exec *prompty.PromptExecution) {
			exec.Messages[1].Content = append(exec.Messages[1].Content, exec.Messages[1].Content[0])
		}},
		{"premature_result", func(exec *prompty.PromptExecution) {
			exec.Messages[0], exec.Messages[1] = exec.Messages[1], exec.Messages[0]
		}},
		{"duplicate_call", func(exec *prompty.PromptExecution) {
			exec.Messages = append([]prompty.ChatMessage{exec.Messages[0]}, exec.Messages...)
		}},
		{"unsupported_error_flag", func(exec *prompty.PromptExecution) {
			result := exec.Messages[1].Content[0].(prompty.ToolResultPart)
			result.IsError = true
			exec.Messages[1].Content[0] = result
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Act.
			exec := base.Clone()
			test.edit(exec)
			_, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(false)})
			// Assert.
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}
