package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesHostedStatelessHistoryOriginalItems(t *testing.T) {
	t.Parallel()
	// Arrange: hosted calls surround a cited assistant message.
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- body
		if strings.Contains(string(body), `"type":"web_search_call"`) {
			writeResponseFixture(w, "completed", "next answer")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, hostedOutputFixture)
	}))
	defer server.Close()
	manager := hostedFixtureAdapter(server)
	// Act: Create -> response wire -> assistant history -> next stateless Create.
	created, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: withResponseHostedTools(prompty.SimplePrompt("search"),
			[]prompty.HostedToolRequest{
				{
					Kind: HostedWebSearch,
				}, {Kind: HostedFileSearch, Settings: []byte(`{"vector_store_ids":["vs_fixture"]}`)},
			}), Store: new(false),
	})
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(created.Response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	history, err := restored.AssistantMessage()
	require.NoError(t, err)
	next := prompty.NewExecution([]prompty.ChatMessage{history, {
		Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "clarify"}},
	}})
	result, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: next, Store: new(false)})
	// Assert: original public items and annotations are native inputs, not flattened.
	require.NoError(t, err)
	<-requests
	second := <-requests
	require.Equal(t, prompty.OutcomeCompleted, result.Response.Outcome)
	var actual struct {
		Input   []json.RawMessage `json:"input"`
		Tools   []json.RawMessage `json:"tools"`
		Include []string          `json:"include"`
	}
	require.NoError(t, json.Unmarshal(second, &actual))
	var original struct {
		Output []json.RawMessage `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(hostedOutputFixture), &original))
	require.Len(t, actual.Input, 4)
	for index, item := range original.Output {
		require.JSONEq(t, string(item), string(actual.Input[index]))
	}
	require.Contains(t, string(actual.Input[2]), "9007199254740993")
	require.Empty(t, actual.Tools)
	require.NotContains(t, actual.Include, "reasoning.encrypted_content")
	require.False(t, result.Handle.ProviderStateRequired)
}

func TestResponsesHostedHistoryInvalidProjectionRejectsBeforeIO(t *testing.T) {
	t.Parallel()
	// Arrange: public history carries exact source items and a destination scope.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseFixture(w, "completed", "unexpected")
	}))
	t.Cleanup(server.Close)
	manager := hostedFixtureAdapter(server)
	var raw struct {
		Output []responses.ResponseOutputItemUnion `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(hostedOutputFixture), &raw))
	response := prompty.NewResponse(nil)
	require.NoError(t, parseRemoteItems(raw.Output, response, []string{HostedWebSearch, HostedFileSearch},
		manager.RuntimeDescriptor().Profiles[0].Identity, responseToolContract{}))
	history, err := response.AssistantMessage()
	require.NoError(t, err)
	base := prompty.NewExecution([]prompty.ChatMessage{history})
	for _, test := range []struct {
		name string
		edit func(*prompty.PromptExecution)
	}{
		{"edited_text", func(e *prompty.PromptExecution) {
			e.Messages[0].Content[0] = prompty.TextPart{Text: "edited"}
		}},
		{"removed_citation", func(e *prompty.PromptExecution) { e.Messages[0].Annotations = nil }},
		{"missing_manifest", func(e *prompty.PromptExecution) {
			e.Messages[0].MessageAnnotations = e.Messages[0].MessageAnnotations[:2]
		}},
		{"duplicate_manifest", func(e *prompty.PromptExecution) {
			e.Messages[0].MessageAnnotations = append(e.Messages[0].MessageAnnotations,
				e.Messages[0].MessageAnnotations[2])
		}},
		{"wrong_scope", func(e *prompty.PromptExecution) {
			annotation := &e.Messages[0].MessageAnnotations[2]
			var manifest responseHistoryManifest
			_ = json.Unmarshal(annotation.Value, &manifest)
			manifest.Scope.Provider = "foreign"
			annotation.Value, _ = json.Marshal(manifest)
		}},
		{"duplicate_item_across_history", func(e *prompty.PromptExecution) {
			e.Messages = append(e.Messages, e.Messages[0])
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exec := base.Clone()
			test.edit(exec)
			// Act.
			_, createErr := manager.Create(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
			)
			// Assert.
			require.Error(t, createErr)
			require.Zero(t, calls.Load())
		})
	}
}

func TestResponsesHostedHistoryMixedPrivateStateAndOmission(t *testing.T) {
	t.Parallel()
	// Arrange: private items are referenced by manifest indices, never copied there.
	var fixture struct {
		Output []json.RawMessage `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(hostedOutputFixture), &fixture))
	outputParts := []string{responseReasoningFixture, responsesFunctionFixture}
	for _, item := range fixture.Output {
		outputParts = append(outputParts, string(item))
	}
	output := strings.Join(outputParts, ",")
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- body
		if strings.Contains(string(body), `"type":"web_search_call"`) {
			writeResponseFixture(w, "completed", "fresh")
			return
		}
		toolResponseFixture(w, output)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	for _, capability := range []string{prompty.CapabilityProviderState, prompty.CapabilityHostedWebSearch,
		prompty.CapabilityHostedFileSearch} {
		manager.base.profiles[0].Capabilities[capability] = prompty.CapabilitySupported
	}
	exec := prompty.SimplePrompt("search and lookup")
	exec.Tools = responseToolDefinitions()
	created, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: withResponseHostedTools(exec, []prompty.HostedToolRequest{
			{Kind: HostedWebSearch}, {Kind: HostedFileSearch, Settings: []byte(`{"vector_store_ids":["vs_fixture"]}`)},
		}), Store: new(false),
	})
	require.NoError(t, err)
	history, err := created.Response.AssistantMessage()
	require.NoError(t, err)
	public, err := json.Marshal(history.MessageAnnotations)
	require.NoError(t, err)
	// Assert: no encrypted reasoning, private content, or function arguments in manifest.
	require.NotContains(t, string(public), "opaque-secret")
	require.NotContains(t, string(public), "private-content")
	require.NotContains(t, string(public), "fc_fixture")
	// Act: replay all original items and provide the host's explicit result.
	next := prompty.NewExecution([]prompty.ChatMessage{history, {
		Role: prompty.RoleTool, Content: []prompty.ContentPart{prompty.ToolResultPart{
			ToolCallID: "call_fixture", Content: []prompty.ContentPart{prompty.TextPart{Text: "found"}},
		}},
	}})
	_, err = manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: next, Store: new(false)})
	require.NoError(t, err)
	<-requests
	var actual struct {
		Input []json.RawMessage `json:"input"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &actual))
	require.Len(t, actual.Input, 6)
	require.JSONEq(t, responseReasoningFixture, string(actual.Input[0]))
	require.JSONEq(t, responsesFunctionFixture, string(actual.Input[1]))
	for index, item := range fixture.Output {
		require.JSONEq(t, string(item), string(actual.Input[index+2]))
	}
	omitted, err := prompty.MarshalResponse(created.Response, prompty.WirePolicy{State: prompty.StateOmit})
	require.NoError(t, err)
	encoded, err := json.Marshal(omitted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "opaque-secret")
	restored, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	unavailable, err := restored.AssistantMessage()
	require.NoError(t, err)
	_, err = manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.NewExecution([]prompty.ChatMessage{unavailable}), Store: new(false),
	})
	require.ErrorIs(t, err, prompty.ErrStateUnavailable)
	require.Empty(t, requests)
}
