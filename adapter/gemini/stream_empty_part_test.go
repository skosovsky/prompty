package gemini

import (
	"context"
	"io"
	"iter"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func TestSDKStreamSkipsEmptyParts(t *testing.T) {
	t.Parallel()
	// Arrange: the official SDK decodes the issue's SSE sequence and a mixed-part frame.
	const frames = `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_probe","name":"lookup","args":{"key":"example"}},"thoughtSignature":"AAI/"}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{}]}}],"usageMetadata":{"promptTokenCount":100,"totalTokenCount":110}}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":"before"},{},{"thoughtSignature":"AQI="},{"text":"after"}]}}]}

data: {"candidates":[{"finishReason":"STOP"}]}

`
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(frames)),
			}, nil
		})},
	})
	require.NoError(t, err)
	provider := New(WithClient(sdk), WithProfiles("https://fixture.invalid"))

	// Act.
	stream := prompty.NewStream(
		t.Context(),
		prompty.StreamNative,
		func(ctx context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return provider.StreamEvents(ctx, &Request{Model: "fixture"})
		},
	)
	for _, streamErr := range stream.Events() {
		require.NoError(t, streamErr)
	}

	// Assert: metadata, canonical output and protected source parts survive.
	status := stream.Status()
	require.Equal(t, prompty.StreamCompleted, status.State)
	result := status.Result
	require.Equal(t, 100, result.Usage.PromptTokens)
	require.Equal(t, 110, result.Usage.TotalTokens)
	require.Equal(t, []prompty.ContentPart{
		prompty.ToolCallPart{ID: "call_probe", Name: "lookup", Args: `{"key":"example"}`},
		prompty.TextPart{Text: "before"},
		prompty.ReasoningPart{},
		prompty.TextPart{Text: "after"},
	}, result.Content)

	parts, err := orderedPartialMessage(prompty.ChatMessage{
		Role:          prompty.RoleAssistant,
		Content:       result.Content,
		ProviderState: result.ProviderState,
		Annotations:   result.Annotations,
	})
	require.NoError(t, err)
	require.Len(t, parts, 4)
	require.Equal(t, []byte{0, 2, 63}, parts[0].ThoughtSignature)
	require.Equal(t, "before", parts[1].Text)
	require.Equal(t, []byte{1, 2}, parts[2].ThoughtSignature)
	require.Equal(t, "after", parts[3].Text)
}
