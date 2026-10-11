package ollama

import (
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestDerivedUsageOverflowPreservesResponse(t *testing.T) {
	t.Parallel()
	// Arrange.
	maximum := int(^uint(0) >> 1)
	raw := &api.ChatResponse{Message: api.Message{Content: "answer"}, Done: true, DoneReason: "stop",
		PromptEvalCount: maximum, EvalCount: 1}
	// Act.
	response, err := New().ParseResponse(raw)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrUsage)
	require.NotNil(t, response)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "answer"}}, response.Content)
	require.Equal(t, maximum, response.Usage.PromptTokens)
	require.Equal(t, 1, response.Usage.CompletionTokens)
	require.Zero(t, response.Usage.TotalTokens)
	require.Equal(t, []prompty.UsageCounter{prompty.UsagePrompt, prompty.UsageCompletion}, response.Usage.Known)
}

func TestResponseUsageFinishAndReasoningMetadata(t *testing.T) {
	t.Parallel()
	// Arrange.
	raw := &api.ChatResponse{Message: api.Message{Thinking: "exposed summary", Content: "answer"},
		Done: true, DoneReason: "stop", PromptEvalCount: 3, EvalCount: 2}
	// Act.
	response, err := New().ParseResponse(raw)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	require.Equal(t, "stop", response.FinishReason)
	require.Equal(
		t,
		[]prompty.ContentPart{prompty.ReasoningPart{Text: "exposed summary"}, prompty.TextPart{Text: "answer"}},
		response.Content,
	)
	require.Equal(t, prompty.UsageKnown, response.Usage.Availability())
	require.Equal(t, 5, response.Usage.TotalTokens)
}

func TestIncompleteEmptyResponseRetainsKnownUsage(t *testing.T) {
	t.Parallel()
	// Arrange: zeros lack presence in SDK, so cannot be advertised as known zero.
	raw := &api.ChatResponse{Done: true, DoneReason: "length", PromptEvalCount: 3}
	// Act.
	response, err := New().ParseResponse(raw)
	// Assert.
	require.NoError(t, err)
	require.Empty(t, response.Content)
	require.Equal(t, prompty.OutcomeIncomplete, response.Outcome)
	require.Equal(t, []prompty.UsageCounter{prompty.UsagePrompt}, response.Usage.Known)
	require.Equal(t, prompty.UsagePartial, response.Usage.Availability())
	require.Zero(t, response.Usage.TotalTokens)
}
