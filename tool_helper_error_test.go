package prompty

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOneShotOrdinaryErrorContinuesAdmittedBatch(t *testing.T) {
	t.Parallel()
	// Arrange: a typed error, no cancellation or lifecycle policy.
	cause := errors.New("handler failed")
	failure := &fs.PathError{Op: "lookup", Path: "caller-owned", Err: cause}
	var effects []string
	first, err := NewTypedTool("first", func(_ context.Context, _ struct{}) (string, error) {
		effects = append(effects, "first")
		return "partial", failure
	})
	require.NoError(t, err)
	second, err := NewTypedTool("second", func(_ context.Context, _ struct{}) (string, error) {
		effects = append(effects, "second")
		return "done", nil
	})
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, first))
	require.NoError(t, RegisterTool(registry, second))
	execution := SimplePrompt("lookup")
	execution.Tools = registry.Definitions()
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		return NewResponse([]ContentPart{ToolCallPart{ID: "one", Name: "first", Args: `{}`},
			ToolCallPart{ID: "two", Name: "second", Args: `{}`}}), nil
	}}
	// Act.
	updated, err := ExecuteWithTypedTools(t.Context(), invoker, execution, registry)
	// Assert.
	require.ErrorIs(t, err, cause)
	var typed *fs.PathError
	require.ErrorAs(t, err, &typed)
	require.Same(t, failure, typed)
	require.Equal(t, []string{"first", "second"}, effects)
	require.Len(t, updated.Messages, 4)
	require.True(t, updated.Messages[2].Content[0].(ToolResultPart).IsError)
	require.False(t, updated.Messages[3].Content[0].(ToolResultPart).IsError)
	require.Equal(t, "two", updated.Messages[3].Content[0].(ToolResultPart).ToolCallID)
}
