package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPausedOutcomeDoesNotAuthorizeTypedHandlers(t *testing.T) {
	t.Parallel()
	// Arrange: even a valid-looking local call cannot turn a paused operation into tool dispatch.
	handlers := 0
	tool, err := NewTypedTool(
		"greet",
		func(context.Context, greetArgs) (string, error) { handlers++; return "hi", nil },
	)
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, tool))
	exec := SimplePrompt("fixture")
	exec.Tools = registry.Definitions()
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		response := NewResponse([]ContentPart{ToolCallPart{ID: "call", Name: "greet", Args: `{"name":"Ada"}`}})
		response.Outcome, response.FinishReason = OutcomePaused, "pause_turn"
		return response, nil
	}}
	// Act.
	result, err := ExecuteWithTypedTools(t.Context(), invoker, exec, registry)
	// Assert: the response remains available for caller policy, and the execution is unchanged.
	require.ErrorIs(t, err, ErrResponseOutcome)
	var dispatch *ToolDispatchError
	require.ErrorAs(t, err, &dispatch)
	require.Equal(t, OutcomePaused, dispatch.Response.Outcome)
	require.Zero(t, handlers)
	require.Equal(t, exec.Messages, result.Messages)
}
