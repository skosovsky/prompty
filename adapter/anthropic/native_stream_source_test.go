package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestNativeDeltaRawSourceFields(t *testing.T) {
	t.Parallel()
	for _, variant := range []struct{ kind, key string }{{"text_delta", "text"}, {"input_json_delta", "partial_json"}, {"thinking_delta", "thinking"}, {"signature_delta", "signature"}} {
		t.Run(variant.kind, func(t *testing.T) {
			t.Parallel()
			for _, value := range []string{"null", "42", "true", "[]", "{}"} {
				raw := `{"type":"content_block_delta","index":0,"delta":{"type":"` + variant.kind + `","` + variant.key + `":` + value + `}}`
				var event anthropic.MessageStreamEventUnion
				require.NoError(t, json.Unmarshal([]byte(raw), &event))
				require.Error(t, validateNativeEventSource(event), raw)
			}
			for _, fields := range []string{``, `,"` + variant.key + `":"lost","` + variant.key + `":"kept"`} {
				raw := `{"type":"content_block_delta","index":0,"delta":{"type":"` + variant.kind + `"` + fields + `}}`
				var event anthropic.MessageStreamEventUnion
				require.NoError(t, json.Unmarshal([]byte(raw), &event))
				require.Error(t, validateNativeEventSource(event), raw)
			}
			// Empty valid fragments and additional optional fields stay representable.
			raw := `{"type":"content_block_delta","index":0,"optional":"value","delta":{"type":"` + variant.kind + `","` + variant.key + `":"","optional":"value"}}`
			var event anthropic.MessageStreamEventUnion
			require.NoError(t, json.Unmarshal([]byte(raw), &event))
			require.NoError(t, validateNativeEventSource(event))
		})
	}
}

func TestNativeMalformedDeltaActualSDKRetainsKnownPrefix(t *testing.T) {
	t.Parallel()
	for _, variant := range []struct{ kind, key, block string }{
		{"text_delta", "text", `{"type":"text","text":""}`},
		{"input_json_delta", "partial_json", `{"type":"tool_use","id":"local","name":"lookup","input":{}}`},
		{"thinking_delta", "thinking", `{"type":"thinking","thinking":"","signature":""}`},
		{"signature_delta", "signature", `{"type":"thinking","thinking":"","signature":""}`},
		{"citations_delta", "citation", `{"type":"text","text":""}`},
	} {
		t.Run(variant.kind, func(t *testing.T) {
			t.Parallel()
			values := []string{"null", "42", "true", "[]", "{}", "missing", "duplicate"}
			for _, value := range values {
				t.Run(value, func(t *testing.T) {
					t.Parallel()
					// Arrange: only the preceding block is source-proven.
					fields := `,"` + variant.key + `":` + value
					if value == "missing" {
						fields = ""
					}
					if value == "duplicate" {
						fragment := `"lost"`
						if variant.kind == "citations_delta" {
							fragment = `{"type":"web_search_result_location","cited_text":"source","url":"https://example.com","title":null,"encrypted_index":"opaque"}`
						}
						fields = `,"` + variant.key + `":` + fragment + `,"` + variant.key + `":` + fragment
					}
					malformed := `{"type":"content_block_delta","index":1,"delta":{"type":"` + variant.kind + `"` + fields + `}}`
					client, _ := nativeFixture(t, []string{messageStartFixture,
						`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"known"}}`,
						`{"type":"content_block_stop","index":0}`,
						`{"type":"content_block_start","index":1,"content_block":` + variant.block + `}`,
						malformed,
						`{"type":"content_block_stop","index":1}`,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
						`{"type":"message_stop"}`,
					})
					// Act.
					stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
					terminals := 0
					for frame := range stream.Events() {
						if frame != nil && frame.Kind == prompty.StreamFinish {
							terminals++
						}
					}
					status := stream.Status()
					// Assert: SDK string coercion must never become a verified fragment/state.
					require.Error(t, status.Err)
					require.Equal(t, 0, terminals)
					require.True(t, status.Result.ContinuationUnavailable)
					require.Equal(t, "known", status.Result.Content[0].(prompty.TextPart).Text)
					require.Empty(t, status.Result.ProviderState)
					require.Empty(t, status.Result.Annotations)
					require.NotEqual(t, prompty.OutcomeCompleted, status.Result.Outcome)
				})
			}
		})
	}
}

func TestNativeMalformedIndexActualSDK(t *testing.T) {
	t.Parallel()
	for _, index := range []string{"null", "-1", "0.5", `"0"`, "true", "missing", "duplicate"} {
		t.Run(index, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			field := `,"index":` + index
			if index == "missing" {
				field = ""
			}
			if index == "duplicate" {
				field = `,"index":0,"index":0`
			}
			client, _ := nativeFixture(t, []string{messageStartFixture,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"known"}}`,
				`{"type":"content_block_delta"` + field + `,"delta":{"type":"text_delta","text":"lost"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
				`{"type":"message_stop"}`,
			})
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert.
			require.Error(t, stream.Status().Err)
			require.Equal(t, "known", stream.Status().Result.Content[0].(prompty.TextPart).Text)
			require.True(t, stream.Status().Result.ContinuationUnavailable)
		})
	}
}

func TestHostedMalformedPartialJSONAfterValidArgumentsActualSDK(t *testing.T) {
	t.Parallel()
	// Arrange: reproduce the independent completeness audit against checked hosted tools.
	events := hostedNativeEvents(t)
	stop := `{"type":"content_block_stop","index":2}`
	malformed := `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":null}}`
	var injected []string
	for _, event := range events {
		if event == stop {
			injected = append(injected, malformed)
		}
		injected = append(injected, event)
	}
	client, calls, _ := hostedStreamingFixture(t, injected)
	exec := prompty.SimplePrompt("fixture")
	exec.HostedTools = []prompty.HostedToolRequest{{Kind: HostedWebSearch}, {Kind: HostedWebFetch}}
	// Act.
	stream := client.ExecuteStream(t.Context(), exec, prompty.StreamNative)
	for range stream.Events() {
	}
	status := stream.Status()
	// Assert: a previously valid object does not excuse a malformed later fragment.
	require.EqualValues(t, 1, calls.Load())
	require.Error(t, status.Err)
	require.True(t, status.Result.ContinuationUnavailable)
	require.Len(t, status.Result.Content, 2)
	require.Len(t, status.Result.ProviderState, 1)
	require.Empty(t, status.Result.MessageAnnotations)
	require.NotEqual(t, prompty.OutcomePaused, status.Result.Outcome)
}

func TestNativeValidEmptyAndOptionalDeltaFieldsActualSDK(t *testing.T) {
	t.Parallel()
	// Arrange: additional optional fields are not a reason to reject a valid supported event.
	client, _ := nativeFixture(t, []string{
		messageStartFixture,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"known"}}`,
		`{"type":"content_block_delta","index":0,"optional":"extra","delta":{"type":"text_delta","text":"","optional":"extra"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		`{"type":"message_stop"}`,
	})
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	// Assert.
	require.NoError(t, stream.Status().Err)
	require.Equal(t, prompty.OutcomeCompleted, stream.Status().Result.Outcome)
	require.Equal(t, "known", stream.Status().Result.Content[0].(prompty.TextPart).Text)
}

func TestNativeConsumedMessageFieldsActualSDK(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, event string }{
		{"start id number", strings.Replace(messageStartFixture, `"id":"msg_fixture"`, `"id":42`, 1)},
		{"start model null", strings.Replace(messageStartFixture, `"model":"fixture"`, `"model":null`, 1)},
		{"start role number", strings.Replace(messageStartFixture, `"role":"assistant"`, `"role":42`, 1)},
		{"start content null", strings.Replace(messageStartFixture, `"content":[]`, `"content":null`, 1)},
		{"start usage null", strings.Replace(messageStartFixture, `"input_tokens":3`, `"input_tokens":null`, 1)},
		{"stop reason number", `{"type":"message_delta","delta":{"stop_reason":42},"usage":{"output_tokens":1}}`},
		{"stop reason unknown", `{"type":"message_delta","delta":{"stop_reason":"unknown"},"usage":{"output_tokens":1}}`},
		{"usage string", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":"1"}}`},
		{"usage float", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1.5}}`},
		{"usage null", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":null}}`},
		{"local id number", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":42,"name":"lookup","input":{}}}`},
		{"local name number", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"local","name":42,"input":{}}}`},
		{"thinking missing", `{"type":"content_block_start","index":1,"content_block":{"type":"thinking","signature":"opaque"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			events := []string{messageStartFixture,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"known"}}`,
				`{"type":"content_block_stop","index":0}`,
				test.event,
				`{"type":"message_stop"}`,
			}
			if strings.Contains(test.name, "start ") {
				events = []string{test.event, `{"type":"message_stop"}`}
			}
			client, _ := nativeFixture(t, events)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert.
			require.Error(t, stream.Status().Err)
			require.NotEqual(t, prompty.OutcomeCompleted, stream.Status().Result.Outcome)
		})
	}
}

func TestNativeUsageOnlyNullableStopReasonActualSDK(t *testing.T) {
	t.Parallel()
	// Arrange: top-level updates may carry usage before the eventual stop reason.
	client, _ := nativeFixture(t, []string{
		messageStartFixture,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"known"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":null,"stop_sequence":null,"optional":"value"},"usage":{"output_tokens":0,"optional":"value"}}`,
		`{"type":"message_delta","delta":{},"usage":{"output_tokens":1}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	})
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	// Assert.
	require.NoError(t, stream.Status().Err)
	require.Equal(t, prompty.OutcomeCompleted, stream.Status().Result.Outcome)
	require.Equal(t, 2, stream.Status().Result.Usage.CompletionTokens)
}
