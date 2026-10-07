package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const exactArguments = `{"id":9007199254740993,"nested":[{"n":1.234567890123456789,"big":1e1000}]}`

func TestHTTPExactArgsSyncNativeAndContinuation(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: request/response HTTP values are decoded with UseNumber.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				if calls.Load() == 2 {
					var incoming struct {
						Messages []struct {
							ToolCalls []struct {
								Function struct {
									Arguments json.RawMessage `json:"arguments"`
								} `json:"function"`
							} `json:"tool_calls"`
						} `json:"messages"`
					}
					if !assert.NoError(t, json.NewDecoder(req.Body).Decode(&incoming)) {
						return
					}
					var actual, expected any
					d := json.NewDecoder(
						strings.NewReader(string(incoming.Messages[0].ToolCalls[0].Function.Arguments)),
					)
					d.UseNumber()
					if !assert.NoError(t, d.Decode(&actual)) {
						return
					}
					d = json.NewDecoder(strings.NewReader(exactArguments))
					d.UseNumber()
					if !assert.NoError(t, d.Decode(&expected)) {
						return
					}
					assert.Equal(t, expected, actual)
				}
				_, _ = fmt.Fprintf(
					w,
					`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":%s}}]},"done":true,"done_reason":"stop"}`+"\n",
					exactArguments,
				)
			}))
			defer server.Close()
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport, err := NewHTTPChatClient(endpoint, server.Client(), 4096)
			require.NoError(t, err)
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local precision fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream: prompty.CapabilitySupported,
				},
			}
			a := New(WithModel("fixture"), WithClient(transport), WithProfiles(server.URL, profile))
			client := adapter.NewClient(a)
			// Act.
			var response *prompty.Response
			if mode == "" {
				response, err = client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
				require.NoError(t, err)
			} else {
				stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), mode)
				for _, streamErr := range stream.Events() {
					require.NoError(t, streamErr)
				}
				response = stream.Status().Result
			}
			wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			restored, err := prompty.UnmarshalResponse(wire)
			require.NoError(t, err)
			_, err = client.Execute(context.Background(), &prompty.PromptExecution{Messages: []prompty.ChatMessage{
				{Role: prompty.RoleAssistant, Content: restored.Content},
			}})
			// Assert: a second actual HTTP request retains exact nested numeric values.
			require.NoError(t, err)
			require.Equal(t, int32(2), calls.Load())
			require.Contains(t, restored.Content[0].(prompty.ToolCallPart).Args, "9007199254740993")
			require.Contains(t, restored.Content[0].(prompty.ToolCallPart).Args, "1.234567890123456789")
			require.Contains(t, restored.Content[0].(prompty.ToolCallPart).Args, "1e1000")
		})
	}
}

func TestSDKUnverifiedFloatsFailClosed(t *testing.T) {
	t.Parallel()
	// Arrange: genuine SDK response loses precision before the adapter receives it.
	var raw api.ChatResponse
	require.NoError(
		t,
		json.Unmarshal(
			[]byte(
				`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"id":9007199254740993}}}]},"done":true,"done_reason":"stop"}`,
			),
			&raw,
		),
	)
	// Act/Assert: neither sync nor chunk parsing advertises rounded args as exact.
	_, err := New().ParseResponse(&raw)
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
	_, err = New().ParseStreamChunk(&raw)
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
}

func TestSDKPrecisionFailurePreventsToolHandler(t *testing.T) {
	t.Parallel()
	// Arrange: the real SDK converts the received integer into an unverified float.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprintln(
			w,
			`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"id":9007199254740993}}}]},"done":true,"done_reason":"stop"}`,
		)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "SDK precision rejection fixture",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityTools: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(api.NewClient(endpoint, server.Client())),
		WithProfiles(server.URL, profile)))
	registry := prompty.NewTypedToolRegistry()
	invoked := 0
	tool, err := prompty.NewTypedTool("lookup", func(_ context.Context, _ struct {
		ID int64 `json:"id"`
	}) (string, error) {
		invoked++
		return "must not execute", nil
	})
	require.NoError(t, err)
	require.NoError(t, prompty.RegisterTool(registry, tool))
	execution := prompty.SimplePrompt("fixture")
	execution.Tools = []prompty.ToolDefinition{{Name: "lookup",
		Parameters: prompty.JSONDocument(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`),
	}}
	// Act.
	_, err = prompty.ExecuteWithTypedTools(context.Background(), client, execution, registry)
	// Assert.
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
	require.Equal(t, int32(1), calls.Load())
	require.Zero(t, invoked)
}

func TestHTTPFrameBoundsAndSafeErrors(t *testing.T) {
	t.Parallel()
	for name, frame := range map[string]string{
		"too large":      strings.Repeat("x", 33),
		"malformed":      `{"private":"secret"`,
		"provider error": `{"error":"secret"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintln(w, frame)
			}))
			defer server.Close()
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport, err := NewHTTPChatClient(endpoint, server.Client(), 32)
			require.NoError(t, err)
			// Act.
			err = transport.Chat(
				context.Background(),
				&api.ChatRequest{Model: "fixture"},
				func(api.ChatResponse) error {
					t.Error("invalid frame reached callback")
					return nil
				},
			)
			// Assert.
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestHTTPFrameLimitExactAndEOF(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{"", "\n"} {
		// Arrange: framing newline does not consume the positive payload budget.
		payload := `{"done":true}`
		reader := bufio.NewReader(strings.NewReader(payload + suffix))
		// Act.
		frame, err := readChatFrame(reader, len(payload))
		// Assert.
		require.Equal(t, payload, string(frame))
		if suffix == "" {
			require.ErrorIs(t, err, errChatEOF)
		} else {
			require.NoError(t, err)
		}
	}
}
