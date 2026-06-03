package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithToolValidation_DelegatesToTypedToolsWhenToolInvoker(t *testing.T) {
	t.Parallel()

	var invoked bool
	tool, err := NewTypedTool("greet", func(_ greetArgs) (string, error) {
		invoked = true
		return "ok", nil
	})
	require.NoError(t, err)

	reg := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(reg, tool))

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "greet", Args: `{"name":"Ada"}`},
			}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), reg)
	require.NoError(t, err)
	require.True(t, invoked)
	require.Len(t, result.Messages, 3)
	assert.Equal(t, RoleTool, result.Messages[2].Role)
}
