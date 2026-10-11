package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty/adapter"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func hostedNativeEvents(t *testing.T) []string {
	t.Helper()
	var fixture struct {
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(hostedHistoryFixture), &fixture))
	events := []string{messageStartFixture}
	for index, raw := range fixture.Content {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		var kind string
		require.NoError(t, json.Unmarshal(fields["type"], &kind))
		var input json.RawMessage
		if kind == "server_tool_use" {
			input = fields["input"]
			fields["input"] = json.RawMessage(`{}`)
			raw, _ = json.Marshal(fields)
		}
		events = append(events, fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":%s}`, index, raw))
		if input != nil {
			split := len(input) / 2
			for _, part := range []string{string(input[:split]), string(input[split:])} {
				delta, _ := json.Marshal(
					map[string]any{
						"type":  "content_block_delta",
						"index": index,
						"delta": map[string]any{"type": "input_json_delta", "partial_json": part},
					},
				)
				events = append(events, string(delta))
			}
		}
		events = append(events, fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, index))
	}
	return append(
		events,
		`{"type":"message_delta","delta":{"stop_reason":"pause_turn"},"usage":{"output_tokens":3}}`,
		`{"type":"message_stop"}`,
	)
}

func TestHostedNativeMixedSourceWire(t *testing.T) {
	t.Parallel()
	// Arrange: actual SDK SSE mixed private/local/hosted content, PDF and error.
	client, calls, requests := hostedStreamingFixture(t, hostedNativeEvents(t))
	exec := prompty.SimplePrompt("fixture")
	exec.HostedTools = []prompty.HostedToolRequest{{Kind: HostedWebSearch}, {Kind: HostedWebFetch}}
	exec.Tools = []prompty.ToolDefinition{
		{
			Name: "lookup",
			Parameters: prompty.JSONDocument(
				`{"type":"object","properties":{"q":{"type":"integer"}},"required":["q"]}`,
			),
		},
	}
	// Act.
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
	terminals := 0
	for chunk, err := range stream.Events() {
		require.NoError(t, err)
		if chunk.Kind == prompty.StreamFinish {
			terminals++
		}
	}
	result := stream.Status().Result
	// Assert.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.OutcomePaused, result.Outcome)
	require.Len(t, result.Content, 6)
	require.Len(t, result.ProviderState, 2)
	items, err := HostedItems(result)
	require.NoError(t, err)
	require.Len(t, items, 6)
	require.JSONEq(t, `{"query":"fixture"}`, string(items[0].Input))
	require.Equal(t, 2, items[0].BlockIndex)
	require.Equal(t, 9, items[5].BlockIndex)
	require.Len(t, result.MessageAnnotations, 7)
	encoded, err := prompty.MarshalResponse(result, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(encoded)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	exec.Messages = append(exec.Messages, message)
	_, err = client.Execute(t.Context(), exec)
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load())
	<-requests
	var replay struct {
		Messages []struct {
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &replay))
	var original struct {
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(hostedHistoryFixture), &original))
	require.Len(t, replay.Messages[1].Content, len(original.Content))
	for index, block := range original.Content {
		require.JSONEq(t, string(block), string(replay.Messages[1].Content[index]))
	}
}

func TestHostedNativePrematureRetainsVerifiedPrefix(t *testing.T) {
	t.Parallel()
	// Arrange.
	events := hostedNativeEvents(t)
	client, _ := nativeFixture(t, events[:len(events)-2])
	// Act.
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	var failure error
	for _, err := range stream.Events() {
		if err != nil {
			failure = err
		}
	}
	result := stream.Status().Result
	// Assert.
	require.ErrorIs(t, failure, prompty.ErrStreamProtocol)
	require.True(t, result.ContinuationUnavailable)
	items, err := HostedItems(result)
	require.NoError(t, err)
	require.Len(t, items, 6)
	require.Len(t, result.MessageAnnotations, 6)
}

func TestHostedNativeRejectsBadArgumentsAndLinkage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, call, delta, result string }{
		{"nonobject", `{"type":"server_tool_use","id":"srv","name":"web_search","input":{}}`, `[]`, ""},
		{"duplicate key", `{"type":"server_tool_use","id":"srv","name":"web_search","input":{}}`, `{"query":"a","query":"b"}`, ""},
		{"unknown kind", `{"type":"server_tool_use","id":"srv","name":"computer","input":{}}`, "", ""},
		{"unknown result id", `{"type":"server_tool_use","id":"srv","name":"web_search","input":{}}`, "", `{"type":"web_search_tool_result","tool_use_id":"other","content":[]}`},
		{"wrong result kind", `{"type":"server_tool_use","id":"srv","name":"web_search","input":{}}`, "", `{"type":"web_fetch_tool_result","tool_use_id":"srv","content":{"type":"web_fetch_tool_result_error","error_code":"unavailable"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			events := []string{
				messageStartFixture,
				fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":%s}`, test.call),
			}
			if test.delta != "" {
				delta, _ := json.Marshal(
					map[string]any{
						"type":  "content_block_delta",
						"index": 0,
						"delta": map[string]any{"type": "input_json_delta", "partial_json": test.delta},
					},
				)
				events = append(events, string(delta))
			}
			events = append(events, `{"type":"content_block_stop","index":0}`)
			if test.result != "" {
				events = append(
					events,
					fmt.Sprintf(`{"type":"content_block_start","index":1,"content_block":%s}`, test.result),
					`{"type":"content_block_stop","index":1}`,
				)
			}
			events = append(
				events,
				`{"type":"message_delta","delta":{"stop_reason":"pause_turn"},"usage":{"output_tokens":1}}`,
				`{"type":"message_stop"}`,
			)
			client, _ := nativeFixture(t, events)
			// Act.
			stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			var failure error
			for _, err := range stream.Events() {
				if err != nil {
					failure = err
				}
			}
			// Assert.
			require.Error(t, failure)
			if len(stream.Status().Result.MessageAnnotations) > 0 {
				require.True(t, stream.Status().Result.ContinuationUnavailable)
			}
			require.Empty(t, stream.Status().Result.Content)
		})
	}
}

func hostedStreamingFixture(t *testing.T, events []string) (prompty.Invoker, *atomic.Int32, <-chan []byte) {
	t.Helper()
	calls := new(atomic.Int32)
	requests := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		requests <- body
		var request struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &request)
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, hostedHistoryFixture)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			var header struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(event), &header)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", header.Type, event)
		}
	}))
	t.Cleanup(server.Close)
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "actual SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState:   prompty.CapabilitySupported,
			prompty.CapabilityNativeStream:    prompty.CapabilitySupported,
			prompty.CapabilityTools:           prompty.CapabilitySupported,
			prompty.CapabilityHostedWebSearch: prompty.CapabilitySupported,
			prompty.CapabilityHostedWebFetch:  prompty.CapabilitySupported,
		},
	}
	return adapter.NewClient(
		New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)),
	), calls, requests
}

func TestHostedNativeInterleavedClosesPreserveOrderAndVerifiedPrefix(t *testing.T) {
	t.Parallel()
	for _, bad := range []bool{false, true} {
		t.Run(strconv.FormatBool(bad), func(t *testing.T) {
			t.Parallel()
			// Arrange: server/result close before the preceding text block. Flush is ordered.
			resultID := "srv"
			if bad {
				resultID = "other"
			}
			events := []string{
				messageStartFixture,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"first"}}`,
				`{"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"srv","name":"web_search","input":{"query":"fixture"}}}`,
				`{"type":"content_block_stop","index":1}`,
				fmt.Sprintf(
					`{"type":"content_block_start","index":2,"content_block":{"type":"web_search_tool_result","tool_use_id":"%s","content":[]}}`,
					resultID,
				),
				`{"type":"content_block_stop","index":2}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"last"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"pause_turn"},"usage":{"output_tokens":1}}`,
				`{"type":"message_stop"}`,
			}
			client, _ := nativeFixture(t, events)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			var failure error
			for _, err := range stream.Events() {
				if err != nil {
					failure = err
				}
			}
			result := stream.Status().Result
			// Assert: even a later invalid result retains the verified preceding server call.
			items, err := HostedItems(result)
			require.NoError(t, err)
			require.Equal(t, "firstlast", result.Content[0].(prompty.TextPart).Text)
			if bad {
				require.Error(t, failure)
				require.Len(t, items, 1)
				require.True(t, result.ContinuationUnavailable)
			} else {
				require.NoError(t, failure)
				require.Len(t, items, 2)
			}
			require.Equal(t, 1, items[0].BlockIndex)
		})
	}
}
