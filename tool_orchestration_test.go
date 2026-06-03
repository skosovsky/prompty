package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithTypedTools_InvokesHandler(t *testing.T) {
	t.Parallel()

	var invoked bool
	tool, err := NewTypedTool("greet", func(a greetArgs) (string, error) {
		invoked = true
		return "hi " + a.Name, nil
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

	result, err := ExecuteWithTypedTools(context.Background(), invoker, SimplePrompt("hi"), reg)
	require.NoError(t, err)
	require.True(t, invoked)
	require.Len(t, result.Messages, 3)
	assert.Equal(t, RoleAssistant, result.Messages[1].Role)
	assert.Equal(t, RoleTool, result.Messages[2].Role)
	tr, ok := result.Messages[2].Content[0].(ToolResultPart)
	require.True(t, ok)
	assert.Equal(t, "hi Ada", mustTextFromParts(t, tr.Content))
}

func TestExecuteWithTypedTools_ValidationErrorDoesNotInvoke(t *testing.T) {
	t.Parallel()

	var invoked bool
	tool, err := NewTypedTool("greet", func(_ greetArgs) (string, error) {
		invoked = true
		return "hi", nil
	})
	require.NoError(t, err)

	reg := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(reg, tool))

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "greet", Args: `{"name":"Ada","extra":1}`},
			}), nil
		},
	}

	_, err = ExecuteWithTypedTools(context.Background(), invoker, SimplePrompt("hi"), reg)
	require.Error(t, err)
	assert.False(t, invoked)
}
