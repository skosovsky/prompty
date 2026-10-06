package openai

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestUsageCounterConversionChecksCanonicalRange(t *testing.T) {
	t.Parallel()
	maximum := int64(int(^uint(0) >> 1))
	for _, value := range []int64{-1, 0, maximum, 4294967297} {
		t.Run(strconv.FormatInt(value, 10), func(t *testing.T) {
			t.Parallel()
			// Arrange: check every SDK counter, not just provider total.
			var raw openai.CompletionUsage
			body := fmt.Sprintf(
				`{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d},"completion_tokens_details":{"reasoning_tokens":%d}}`,
				value,
				value,
				value,
				value,
				value,
			)
			require.NoError(t, raw.UnmarshalJSON([]byte(body)))
			// Act.
			usage, err := usageFromOpenAI(raw)
			// Assert.
			if value < 0 || value > maximum {
				require.ErrorIs(t, err, prompty.ErrUsage)
				require.Empty(t, usage.Known)
				require.Zero(t, usage.PromptTokens)
				require.Zero(t, usage.CompletionTokens)
				require.Zero(t, usage.TotalTokens)
				require.Zero(t, usage.PromptTokensCached)
				require.Zero(t, usage.CompletionTokensReasoning)
			} else {
				require.NoError(t, err)
				require.Len(t, usage.Known, 5)
				require.Equal(t, int(value), usage.TotalTokens)
			}
			require.NoError(t, usage.Validate())
		})
	}
}

func TestInvalidUsageSyncRetainsRepresentableCountersAndCaptureMetadata(t *testing.T) {
	t.Parallel()
	// Arrange: actual SDK JSON presence, one invalid counter and one observed valid zero.
	var raw openai.ChatCompletion
	require.NoError(
		t,
		raw.UnmarshalJSON(
			[]byte(
				`{"id":"observed","choices":[{"index":0,"finish_reason":"stop","message":{"content":"answer"}}],"usage":{"prompt_tokens":-1,"completion_tokens":3,"total_tokens":0}}`,
			),
		),
	)
	a := New()
	// Act.
	response, err := a.ParseResponse(&raw)
	metadata := a.CaptureResponseMetadata(&raw)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrUsage)
	require.NotNil(t, response)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "answer"}}, response.Content)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageCompletion, prompty.UsageTotal}, response.Usage.Known)
	require.Equal(t, 3, response.Usage.CompletionTokens)
	require.Zero(t, response.Usage.PromptTokens)
	require.NoError(t, response.Usage.Validate())
	require.Equal(t, response.Usage, *metadata.Usage)
	require.Equal(t, "observed", metadata.ProviderResponseID)
}

func TestInvalidNativeUsageRetainsFrameAndClosesTransportWithoutReplay(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, invalidNativeUsageConformance)
}

func invalidNativeUsageConformance(t *testing.T, model string) {
	t.Helper()
	// Arrange: content and invalid usage arrive together, before unread transport data.
	const wire = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":-1,\"completion_tokens\":3}}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"total_tokens\":10}}\n\n" + "data: [DONE]\n\n"
	client, transport := cleanupFixtureClientModel(strings.NewReader(wire), model)
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	require.Zero(t, transport.calls.Load())
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, prompty.ErrUsage)
		} else {
			require.NoError(t, err)
		}
	}
	// Assert: accepted content and valid counts survive, later unread usage does not.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.Equal(t, prompty.OutcomeCompleted, status.Result.Outcome)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "answer"}}, status.Result.Content)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageCompletion}, status.Result.Usage.Known)
	require.Equal(t, 3, status.Result.Usage.CompletionTokens)
	require.Zero(t, status.Result.Usage.TotalTokens)
	require.Equal(t, 1, terminals)
	require.Equal(t, int32(1), transport.calls.Load())
	require.Equal(t, int32(1), transport.body.closed.Load())
}
