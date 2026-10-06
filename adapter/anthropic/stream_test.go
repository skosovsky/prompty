package anthropic

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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func nativeFixture(t *testing.T, events []string) (prompty.Invoker, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
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
		Identity: prompty.ProfileIdentity{
			Provider: "anthropic", Endpoint: server.URL, Model: "fixture", ModelFamily: "fixture-family",
		},
		Provenance:   "local SSE fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	return adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile))), &calls
}

const messageStartFixture = `{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture","content":[],"usage":{"input_tokens":3,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`

func TestNativeSignedThinkingAndRefusal(t *testing.T) {
	t.Parallel()
	// Arrange: signature arrives after thinking; refusal is independent of text.
	client, calls := nativeFixture(t, []string{
		messageStartFixture,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"private thought"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"refusal"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	})
	// Act.
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	status := stream.Status()
	// Assert.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, prompty.OutcomeRefusal, status.Result.Outcome)
	require.Len(t, status.Result.ProviderState, 1)
	require.Equal(t, status.Result.Diagnostics.Profile, status.Result.ProviderState[0].Envelope.Scope)
	require.Equal(t, "private thought", status.Result.Content[0].(prompty.ReasoningPart).Text)
	require.JSONEq(t, `{"type":"thinking","thinking":"private thought","signature":"opaque"}`,
		string(status.Result.ProviderState[0].Envelope.Payload))
	data, err := prompty.MarshalResponse(status.Result, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(data)
	require.NoError(t, err)
	require.Equal(t, status.Result.ProviderState, restored.ProviderState)
}

type nativeBody struct {
	io.ReadCloser

	closed *atomic.Int32
}

func (b nativeBody) Close() error {
	b.closed.Add(1)
	return b.ReadCloser.Close()
}

type nativeTransport func(*http.Request) (*http.Response, error)

func (f nativeTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestNativeBreakClosesTransportWithoutReplay(t *testing.T) {
	t.Parallel()
	// Arrange: instrument the injected transport, no network or credentials.
	var calls, closed atomic.Int32
	transport := nativeTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		payload, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(payload), `"stream":true`)
		body := "event: message_start\ndata: " + messageStartFixture + "\n\n" +
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"partial\"}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: nativeBody{ReadCloser: io.NopCloser(strings.NewReader(body)), closed: &closed}, Request: req}, nil
	})
	sdk := anthropic.NewClient(option.WithBaseURL("https://fixture.invalid"), option.WithAPIKey("fixture"),
		option.WithHTTPClient(&http.Client{Transport: transport}))
	a := New(WithClient(&sdk))
	// Act.
	for frame, err := range a.StreamEvents(context.Background(), &anthropic.MessageNewParams{Model: "fixture", MaxTokens: 10}) {
		require.NoError(t, err)
		if len(frame.Content) > 0 {
			break
		}
	}
	// Assert.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, int32(1), closed.Load())
}

func TestNativeProtocolFaults(t *testing.T) {
	t.Parallel()
	for name, events := range map[string][]string{
		"duplicate message start": {messageStartFixture, messageStartFixture},
		"missing index":           {messageStartFixture, `{"type":"content_block_start","content_block":{"type":"text","text":""}}`},
		"negative index":          {messageStartFixture, `{"type":"content_block_start","index":-1,"content_block":{"type":"text","text":""}}`},
		"missing block":           {messageStartFixture, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lost"}}`},
		"missing finish":          {messageStartFixture, `{"type":"message_stop"}`},
		"open block finish":       {messageStartFixture, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"partial"}}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`},
		"missing signature":       {messageStartFixture, `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"x","signature":""}}`, `{"type":"content_block_stop","index":0}`},
		"wrong delta kind":        {messageStartFixture, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"partial"}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			client, calls := nativeFixture(t, events)
			// Act.
			stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert: no replay, no false terminal success.
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.Error(t, stream.Status().Err)
		})
	}
}

func TestNativeTextAndInterleavedTools(t *testing.T) {
	t.Parallel()
	// Arrange: text and two same-name calls; argument deltas arrive out of order.
	client, calls := nativeFixture(t, []string{
		messageStartFixture,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"early"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"a","name":"lookup","input":{}}}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"b","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"x\":2}"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	})
	// Act.
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if frame.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	status := stream.Status()
	// Assert.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.OutcomeToolCalls, status.Result.Outcome)
	require.Equal(t, 10, status.Result.Usage.TotalTokens)
	require.Len(t, status.Result.Content, 4)
	require.Equal(t, "early", status.Result.Content[1].(prompty.TextPart).Text)
	require.Equal(t, "a", status.Result.Content[2].(prompty.ToolCallPart).ID)
	require.JSONEq(t, `{"x":1}`, status.Result.Content[2].(prompty.ToolCallPart).Args)
	require.Equal(t, "b", status.Result.Content[3].(prompty.ToolCallPart).ID)
	require.JSONEq(t, `{"x":2}`, status.Result.Content[3].(prompty.ToolCallPart).Args)
}

func TestNativeIncompleteAndConsumerBreakRetainPartial(t *testing.T) {
	t.Parallel()
	for _, stop := range []bool{false, true} {
		t.Run(strconv.FormatBool(stop), func(t *testing.T) {
			t.Parallel()
			// Arrange: no message_stop, so EOF is not successful completion.
			client, calls := nativeFixture(t, []string{
				messageStartFixture,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"partial"}}`,
			})
			// Act.
			stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for frame := range stream.Events() {
				if stop && len(frame.Content) > 0 {
					break
				}
			}
			status := stream.Status()
			// Assert.
			require.Equal(t, int32(1), calls.Load())
			require.NotEmpty(t, status.Result.Content)
			require.Equal(t, "partial", status.Result.Content[0].(prompty.TextPart).Text)
			if stop {
				require.Equal(t, prompty.StreamCanceled, status.State)
			} else {
				require.Equal(t, prompty.StreamFailed, status.State)
				require.ErrorIs(t, status.Err, prompty.ErrStreamProtocol)
			}
		})
	}
}
