package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestNativeEventFailureRetainsKnownParts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, prefix string
		count        int
	}{
		{"text", `{"text":"known"}`, 1},
		{"signed reasoning", `{"text":"known","thought":true,"thoughtSignature":"c2lnbmF0dXJl"}`, 1},
		{"complete call", `{"text":"known"},{"functionCall":{"id":"a","name":"lookup","args":{"n":"large-id"}}}`, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: supported parts and an unsupported native result share one SSE event.
			client, calls := geminiNativeFixture(
				t,
				`{"candidates":[{"content":{"parts":[`+test.prefix+`,{"functionResponse":{"name":"lookup","response":{"output":1}}}]},"finishReason":"STOP"}]}`,
			)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			status := stream.Status()
			// Assert: retained prefix with valid associations, no successful finish or replay.
			require.Error(t, status.Err)
			require.Equal(t, prompty.StreamFailed, status.State)
			require.Len(t, status.Result.Content, test.count)
			require.True(t, status.Result.ContinuationUnavailable)
			require.Empty(t, status.Result.Outcome)
			require.Empty(t, status.Result.FinishReason)
			message, err := status.Result.AssistantMessage()
			require.NoError(t, err)
			_, err = client.Execute(t.Context(), prompty.SimplePrompt("next").AddMessage(message))
			require.Error(t, err)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestNativeSourceFailureRevokesEarlierExchange(t *testing.T) {
	t.Parallel()
	for _, final := range []string{"", `{"candidates":[null]}`} {
		t.Run(final, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			client, calls := geminiNativeFixture(
				t,
				`{"candidates":[{"content":{"parts":[{"text":"known"},{"functionCall":{"id":"a","name":"lookup","args":{}}}]}}]}`,
				final,
			)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			if final != "" {
				// SDK-GENAI-STREAM: malformed converter shapes panic before DTO return.
				require.Panics(t, func() {
					for range stream.Events() {
					}
				})
				require.EqualValues(t, 1, calls.Load())
				return
			}
			for range stream.Events() {
			}
			status := stream.Status()
			// Assert.
			require.Error(t, status.Err)
			require.Len(t, status.Result.Content, 2)
			require.True(t, status.Result.ContinuationUnavailable)
			message, err := status.Result.AssistantMessage()
			require.NoError(t, err)
			_, err = client.Execute(t.Context(), prompty.SimplePrompt("next").AddMessage(message))
			require.Error(t, err)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
