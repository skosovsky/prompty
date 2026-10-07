package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

const streamReasoningItem = `{"id":"rs_fixture","type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"opaque"}`

const streamMessageItem = `{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer","annotations":[{"type":"url_citation","url":"https://example.org","title":"source","start_index":0,"end_index":6}]}]}`

const streamHostedItem = `{"id":"ws_fixture","type":"web_search_call","status":"completed","action":{"type":"search","queries":["reference"]}}`

func mixedResponseSSEFixture(complete bool) []string {
	events := []string{
		`{"type":"response.created","response":{"id":"resp_fixture","model":"fixture","status":"in_progress","output":[]}}`,
		`{"type":"response.in_progress","response":{"id":"resp_fixture","model":"fixture","status":"in_progress","output":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_fixture","type":"reasoning","status":"in_progress","summary":[]}}`,
		`{"type":"response.reasoning_summary_part.added","item_id":"rs_fixture","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.reasoning_summary_text.delta","item_id":"rs_fixture","output_index":0,"summary_index":0,"delta":"plan"}`,
		`{"type":"response.reasoning_summary_text.done","item_id":"rs_fixture","output_index":0,"summary_index":0,"text":"plan"}`,
		`{"type":"response.output_item.done","output_index":0,"item":` + streamReasoningItem + `}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"id":"msg_fixture","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.content_part.added","item_id":"msg_fixture","output_index":1,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","item_id":"msg_fixture","output_index":1,"content_index":0,"delta":"ans"}`,
		`{"type":"response.output_item.added","output_index":2,"item":{"id":"fc_fixture","type":"function_call","call_id":"call_fixture","name":"lookup","arguments":"","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_fixture","output_index":2,"delta":"{\"id\":"}`,
		`{"type":"response.output_text.delta","item_id":"msg_fixture","output_index":1,"content_index":0,"delta":"wer"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_fixture","output_index":2,"delta":"9007199254740993}"}`,
		`{"type":"response.output_text.annotation.added","item_id":"msg_fixture","output_index":1,"content_index":0,"annotation_index":0,"annotation":{"type":"url_citation","url":"https://example.org","title":"source","start_index":0,"end_index":6}}`,
		`{"type":"response.output_text.done","item_id":"msg_fixture","output_index":1,"content_index":0,"text":"answer"}`,
		`{"type":"response.output_item.done","output_index":1,"item":` + streamMessageItem + `}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_fixture","output_index":2,"name":"lookup","arguments":"{\"id\":9007199254740993}"}`,
		`{"type":"response.output_item.done","output_index":2,"item":` + responsesFunctionFixture + `}`,
		`{"type":"response.output_item.added","output_index":3,"item":{"id":"ws_fixture","type":"web_search_call","status":"in_progress","action":{"type":"search","queries":["reference"]}}}`,
		`{"type":"response.web_search_call.completed","item_id":"ws_fixture","output_index":3}`,
		`{"type":"response.output_item.done","output_index":3,"item":` + streamHostedItem + `}`,
	}
	if complete {
		events = append(
			events,
			`{"type":"response.completed","response":{"id":"resp_fixture","model":"fixture","status":"completed","output":[`+
				streamReasoningItem+`,`+streamMessageItem+`,`+responsesFunctionFixture+`,`+streamHostedItem+
				`],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12,"output_tokens_details":{"reasoning_tokens":0}}}}`,
		)
	}
	return events
}

func writeResponseSSEFixture(w http.ResponseWriter, events []string) {
	writeResponseSSEFixtureFrom(w, events, 0)
}

func writeResponseSSEFixtureFrom(w http.ResponseWriter, events []string, start int) {
	w.Header().Set("Content-Type", "text/event-stream")
	for sequence, event := range events {
		_, _ = fmt.Fprintf(w, "data: {\"sequence_number\":%d,%s\n\n", start+sequence, strings.TrimPrefix(event, "{"))
	}
}

func TestResponsesCreateStreamActualSDKMixedAndPrematureEOF(t *testing.T) {
	t.Parallel()
	for _, complete := range []bool{true, false} {
		t.Run(strconv.FormatBool(complete), func(t *testing.T) {
			t.Parallel()
			// Arrange: mixed native output and interleaved text/function fragments.
			var calls atomic.Int32
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(request.Body)
				requests <- body
				writeResponseSSEFixture(w, mixedResponseSSEFixture(complete))
			}))
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			task43ResponsesControlsProfile(manager)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			exec.ModelOptions = &prompty.ModelOptions{
				ProviderSettings: []byte(`{"reasoning_effort":"low","reasoning_summary":"concise"}`),
			}
			stream := manager.CreateStream(t.Context(), prompty.RemoteResponseRequest{
				Execution: withResponseHostedTools(exec,

					[]prompty.HostedToolRequest{{Kind: HostedWebSearch}}), Store: new(false),
				StrictTools: true,
			})
			require.Zero(t, calls.Load())
			// Act.
			terminals := 0
			var terminalErr error
			for frame, err := range stream.Events() {
				if frame != nil && frame.Kind == prompty.StreamTerminal {
					terminals++
					terminalErr = err
				}
			}
			result := stream.RemoteResult()
			// Assert: partial failure still retains canonical state and the accepted resource.
			require.Equal(t, 1, terminals)
			require.EqualValues(t, 1, calls.Load())
			require.NotNil(t, result.Handle)
			require.Equal(t, "resp_fixture", result.Handle.ProviderID)
			if complete {
				require.NoError(t, terminalErr)
				require.Equal(t, prompty.StreamCompleted, stream.Status().State)
				require.Equal(t, prompty.RemoteResponseCompleted, result.Handle.Status)
				require.Equal(t, prompty.OutcomeToolCalls, result.Response.Outcome)
				require.Equal(t, 12, result.Response.Usage.TotalTokens)
				require.Contains(t, result.Response.Usage.Known, prompty.UsageReasoning)
			} else {
				require.ErrorIs(t, terminalErr, prompty.ErrStreamProtocol)
				require.Equal(t, prompty.RemoteResponseRunning, result.Handle.Status)
				require.True(t, result.Response.ContinuationUnavailable)
				require.Equal(t, 10, result.Response.Usage.PromptTokens)
				require.Contains(t, result.Response.Usage.Known, prompty.UsageCompletion)
			}
			require.Equal(
				t,
				[]prompty.ContentPart{prompty.ReasoningPart{Text: "plan"}, prompty.TextPart{Text: "answer"},
					prompty.ToolCallPart{
						ID:   "call_fixture",
						Name: "lookup",
						Args: `{"id":9007199254740993}`,
					}},
				result.Response.Content,
			)
			require.Len(t, result.Response.ProviderState, 2)
			require.Equal(t, 0, result.Response.ProviderState[0].PartIndex)
			require.Equal(t, 2, result.Response.ProviderState[1].PartIndex)
			require.Len(t, result.Response.Annotations, 1)
			require.Equal(t, 1, result.Response.Annotations[0].PartIndex)
			hosted, err := HostedSearchItems(result.Response)
			require.NoError(t, err)
			require.Len(t, hosted, 1)
			var request map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(<-requests, &request))
			require.Equal(t, "true", string(request["stream"]))
			require.JSONEq(t, `{"effort":"low","summary":"concise"}`, string(request["reasoning"]))
			var tools []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(request["tools"], &tools))
			for _, tool := range tools {
				if string(tool["type"]) == `"function"` {
					require.Equal(t, "true", string(tool["strict"]))
				}
			}
		})
	}
}
