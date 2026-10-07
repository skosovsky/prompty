package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const hostedHistoryFixture = `{"id":"msg_hosted","type":"message","role":"assistant","model":"fixture","content":[` +
	`{"type":"thinking", "thinking":"plan", "signature":"signed=="},` +
	`{"type":"text","text":"first"},` +
	`{"type":"server_tool_use","id":"srv_search","name":"web_search","input":{"query":"fixture"}},` +
	`{"type":"web_search_tool_result","tool_use_id":"srv_search","content":[{"type":"web_search_result","url":"https://example.com","title":"Source","encrypted_content":"opaque=="}]},` +
	`{"type":"text","text":"citation","citations":[{"type":"web_search_result_location","cited_text":"source","url":"https://example.com","title":null,"encrypted_index":"index=="}]},` +
	`{"type":"server_tool_use","id":"srv_fetch","name":"web_fetch","input":{"url":"https://example.com/doc.pdf"},"caller":{"type":"direct"}},` +
	`{"type":"web_fetch_tool_result","tool_use_id":"srv_fetch","content":{"type":"web_fetch_result","url":"https://example.com/doc.pdf","content":{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="},"citations":{"enabled":true}}}},` +
	`{"type":"tool_use","id":"local_1","name":"lookup","input":{"q": 9007199254740993}},` +
	`{"type":"server_tool_use","id":"srv_error","name":"web_search","input":{"query":""}},` +
	`{"type":"web_search_tool_result","tool_use_id":"srv_error","content":{"type":"web_search_tool_result_error","error_code":"invalid_tool_input"}},` +
	`{"type":"redacted_thinking","data":"redacted=="},` +
	`{"type":"text","text":"last"}],"stop_reason":"pause_turn","usage":{"input_tokens":7,"output_tokens":3,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`

func parseHostedFixture(t *testing.T, a *Adapter, profile prompty.ProfileIdentity) *prompty.Response {
	t.Helper()
	var message anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(hostedHistoryFixture), &message))
	response, err := a.ParseResponseForProfile(&message, profile)
	require.NoError(t, err)
	return response
}

func TestHostedHistoryActualSDKWireAndExplicitPause(t *testing.T) {
	t.Parallel()
	// Arrange: mixed hosted success/error, citations, local tools and private reasoning.
	requests := make(chan []byte, 4)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(request.Body)
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, hostedHistoryFixture)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "actual SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState:   prompty.CapabilitySupported,
			prompty.CapabilityTools:           prompty.CapabilitySupported,
			prompty.CapabilityHostedWebSearch: prompty.CapabilitySupported,
			prompty.CapabilityHostedWebFetch:  prompty.CapabilitySupported,
		},
	}
	a := New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile))
	client := adapter.NewClient(a)
	type lookupArgs struct {
		Q int64 `json:"q"`
	}
	var handlers atomic.Int32
	tool, err := prompty.NewTypedTool(
		"lookup",
		func(context.Context, lookupArgs) (string, error) { handlers.Add(1); return "ok", nil },
	)
	require.NoError(t, err)
	registry := prompty.NewTypedToolRegistry()
	require.NoError(t, prompty.RegisterTool(registry, tool))
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = registry.Definitions()
	exec.HostedTools = []prompty.HostedToolRequest{{Kind: HostedWebSearch}, {Kind: HostedWebFetch}}
	// Act: one initial request, wire restore, and one caller-invoked continuation.
	response, err := client.Execute(t.Context(), exec)
	require.NoError(t, err)
	<-requests
	require.EqualValues(t, 1, calls.Load())
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	document, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(document)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	exec.Messages = append(exec.Messages, message)
	executionWire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	document, err = json.Marshal(executionWire)
	require.NoError(t, err)
	exec, err = prompty.DecodeExecutionJSON(document)
	require.NoError(t, err)
	_, err = client.Execute(t.Context(), exec)
	require.NoError(t, err)
	// Assert: ordered native source, nullable/absent fields and private state survive.
	require.EqualValues(t, 2, calls.Load())
	require.Equal(t, prompty.OutcomePaused, restored.Outcome)
	require.Len(t, restored.Content, 6)
	require.Len(t, restored.ProviderState, 2)
	items, err := HostedItems(restored)
	require.NoError(t, err)
	require.Len(t, items, 6)
	require.Equal(
		t,
		[]int{2, 3, 5, 6, 8, 9},
		[]int{
			items[0].BlockIndex,
			items[1].BlockIndex,
			items[2].BlockIndex,
			items[3].BlockIndex,
			items[4].BlockIndex,
			items[5].BlockIndex,
		},
	)
	require.Contains(t, string(items[3].Content), `"type":"document"`)
	require.NotContains(t, string(items[3].Content), `"title"`)
	require.Contains(t, string(items[5].Content), `"invalid_tool_input"`)
	items[0].SourceJSON[0] = 'X'
	owned, err := HostedItems(restored)
	require.NoError(t, err)
	require.Equal(t, byte('{'), owned[0].SourceJSON[0])
	annotations, err := json.Marshal(restored.MessageAnnotations)
	require.NoError(t, err)
	require.NotContains(t, string(annotations), "signed==")
	require.NotContains(t, string(annotations), "redacted==")
	var request struct {
		Messages []struct {
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	var original struct {
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(hostedHistoryFixture), &original))
	require.Len(t, request.Messages[1].Content, len(original.Content))
	for index, source := range original.Content {
		require.JSONEq(t, string(source), string(request.Messages[1].Content[index]))
	}
	// Act / Assert: even mixed local-looking content on pause authorizes no handler.
	_, err = prompty.ExecuteWithTypedTools(t.Context(), client, exec, registry)
	require.ErrorIs(t, err, prompty.ErrResponseOutcome)
	require.Zero(t, handlers.Load())
	require.EqualValues(t, 3, calls.Load())
}

func TestHostedSourceAdversarialShapesAndLinkage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, old, replacement string }{
		{"duplicate ID", `"id":"srv_fetch"`, `"id":"srv_search"`},
		{"orphan result", `"tool_use_id":"srv_search"`, `"tool_use_id":"unknown"`},
		{"cross kind", `"tool_use_id":"srv_fetch"`, `"tool_use_id":"srv_search"`},
		{"unknown caller", `{"type":"direct"}`, `{"type":"code_execution_20260120","tool_id":"remote"}`},
		{"null caller", `{"type":"direct"}`, `null`},
		{"unknown source", `"media_type":"application/pdf"`, `"media_type":"application/octet-stream"`},
		{"bad base64", `"data":"JVBERi0="`, `"data":"not base64"`},
		{"unknown error", `"error_code":"invalid_tool_input"`, `"error_code":"future"`},
		{"null search content", `[{"type":"web_search_result","url":"https://example.com","title":"Source","encrypted_content":"opaque=="}]`, `null`},
		{"unknown citation", `"type":"web_search_result_location"`, `"type":"future_citation"`},
		{"duplicate escaped input", `"input":{"query":"fixture"}`, `"input":{"query":"fixture","qu\u0065ry":"other"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var raw anthropic.Message
			require.Contains(t, hostedHistoryFixture, test.old)
			require.NoError(
				t,
				json.Unmarshal([]byte(strings.Replace(hostedHistoryFixture, test.old, test.replacement, 1)), &raw),
			)
			// Act.
			_, err := New().ParseResponse(&raw)
			// Assert.
			require.ErrorIs(t, err, ErrHostedTool)
		})
	}
}
