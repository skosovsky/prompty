package ollama

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestThinkingWireContinuationActualHTTP(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int64
			incoming := make(chan []byte, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read", http.StatusInternalServerError)
					return
				}
				incoming <- data
				calls.Add(1)
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(
					w,
					`{"message":{"role":"assistant","thinking":"private reasoning","content":"public answer"},"done":true,"done_reason":"stop"}`+"\n",
				)
			}))
			t.Cleanup(server.Close)
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := api.NewClient(endpoint, server.Client())
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local actual HTTP conformance",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream: prompty.CapabilitySupported,
				},
			}
			client := adapter.NewClient(
				New(WithModel("fixture"), WithClient(transport), WithProfiles(server.URL, profile)),
			)
			invoke := func(execution *prompty.PromptExecution) (*prompty.Response, error) {
				if mode == "" {
					return client.Execute(context.Background(), execution)
				}
				stream := client.ExecuteStream(context.Background(), execution, mode)
				for range stream.Events() {
				}
				return stream.Status().Result, stream.Status().Err
			}
			// Act: actual provider IO -> canonical -> response wire -> next actual request.
			response, err := invoke(prompty.SimplePrompt("fixture"))
			require.NoError(t, err)
			wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			restored, err := prompty.UnmarshalResponse(wire)
			require.NoError(t, err)
			message, err := restored.AssistantMessage()
			require.NoError(t, err)
			_, err = invoke(prompty.SimplePrompt("fixture").AddMessage(message))
			// Assert.
			require.NoError(t, err)
			require.Equal(t, int64(2), calls.Load())
			<-incoming
			var native struct {
				Messages []struct {
					Role     string `json:"role"`
					Thinking string `json:"thinking"`
					Content  string `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(<-incoming, &native))
			require.Len(t, native.Messages, 2)
			require.Equal(t, "private reasoning", native.Messages[1].Thinking)
			require.Equal(t, "public answer", native.Messages[1].Content)
		})
	}
}
func TestThinkingInterleavingAndToolErrorAreUnsupported(t *testing.T) {
	t.Parallel()
	for _, message := range []prompty.ChatMessage{
		{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "before"}, prompty.ReasoningPart{Text: "later"}}},
		{Role: prompty.RoleTool, Content: []prompty.ContentPart{prompty.ToolResultPart{Name: "lookup", ToolCallID: "call", IsError: true, Content: []prompty.ContentPart{prompty.TextPart{Text: "failed"}}}}},
		{Role: prompty.RoleTool, Content: []prompty.ContentPart{&prompty.ToolResultPart{Name: "lookup", ToolCallID: "call", IsError: true, Content: []prompty.ContentPart{prompty.TextPart{Text: "failed"}}}}},
	} {
		// Arrange.
		execution := prompty.SimplePrompt("fixture").AddMessage(message)
		// Act.
		request, err := New().Translate(execution)
		// Assert.
		require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
		require.Nil(t, request)
	}
}

type invalidArgumentSDKSource struct{ response api.ChatResponse }

func (s invalidArgumentSDKSource) Chat(
	_ context.Context,
	_ *api.ChatRequest,
	callback api.ChatResponseFunc,
) error {
	return callback(s.response)
}
func TestInvalidValuesPreserveKnownPartsWithoutStreamFinish(t *testing.T) {
	t.Parallel()
	for _, cyclic := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonfinite float", true: "cycle"}[cyclic], func(t *testing.T) {
			t.Parallel()
			// Arrange: a host-supplied SDK client/DTO has no original JSON source.
			arguments := api.NewToolCallFunctionArguments()
			value := map[string]any{"nested": map[string][]float64{"values": {math.Inf(1)}}}
			if cyclic {
				value = map[string]any{}
				value["cycle"] = value
			}
			arguments.Set("value", value)
			raw := api.ChatResponse{
				Message: api.Message{
					Role:     "assistant",
					Thinking: "private reasoning",
					Content:  "known answer",
					ToolCalls: []api.ToolCall{
						{ID: "call", Function: api.ToolCallFunction{Name: "lookup", Arguments: arguments}},
					},
				},
				Done:       true,
				DoneReason: "stop",
			}
			source := New(WithClient(invalidArgumentSDKSource{response: raw}))
			// Act: both direct SDK parse and native callback use the same rejection.
			response, err := source.ParseResponse(&raw)
			var frames []*prompty.ResponseChunk
			var streamError error
			for frame, streamErr := range source.StreamEvents(t.Context(), &api.ChatRequest{}) {
				if frame != nil {
					frames = append(frames, frame)
				}
				if streamErr != nil {
					streamError = streamErr
				}
			}
			// Assert: preserve accepted text/thinking, never claim completion or expose bad args.
			require.ErrorIs(t, err, adapter.ErrMalformedArgs)
			require.NotNil(t, response)
			require.Len(t, response.Content, 2)
			require.True(t, response.ContinuationUnavailable)
			require.Empty(t, response.Outcome)
			require.Empty(t, response.FinishReason)
			require.ErrorIs(t, streamError, adapter.ErrMalformedArgs)
			require.Len(t, frames, 1)
			require.Equal(t, prompty.StreamDelta, frames[0].Kind)
			require.Len(t, frames[0].Content, 2)
			require.Empty(t, frames[0].Outcome)
			require.Empty(t, frames[0].FinishReason)
		})
	}
}
