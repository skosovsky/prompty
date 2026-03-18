package prompty

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithToolValidation_InvalidToolCallRetries(t *testing.T) {
	t.Parallel()

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			if callNum == 1 {
				return NewResponse([]ContentPart{
					TextPart{Text: "Calling tool"},
					ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{"city":1}`},
				}), nil
			}
			require.Len(t, exec.Messages, 3)
			return NewResponse([]ContentPart{TextPart{Text: "done"}}), nil
		},
	}

	exec := SimplePrompt("hi")
	result, err := ExecuteWithToolValidation(context.Background(), invoker, exec, toolValidatorFunc(func(name string, argsJSON string) error {
		assert.Equal(t, "lookup", name)
		assert.JSONEq(t, `{"city":1}`, argsJSON)
		return errors.New("city must be a string")
	}), 1)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 4)
	assert.Equal(t, RoleAssistant, result.Messages[1].Role)
	assert.Len(t, result.Messages[1].Content, 2)
	assert.Equal(t, RoleTool, result.Messages[2].Role)
	assert.Equal(t, "city must be a string", result.Messages[2].Content[0].(ToolResultPart).Content[0].(TextPart).Text)
	assert.Equal(t, RoleAssistant, result.Messages[3].Role)
	assert.Len(t, exec.Messages, 1, "original exec must remain unchanged")
}

func TestExecuteWithToolValidation_MultipleInvalidToolCalls(t *testing.T) {
	t.Parallel()

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			if callNum == 1 {
				return NewResponse([]ContentPart{
					ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
					ToolCallPart{ID: "tool-2", Name: "weather", Args: `{}`},
				}), nil
			}
			require.Len(t, exec.Messages, 4)
			return NewResponse([]ContentPart{TextPart{Text: "retry ok"}}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), toolValidatorFunc(func(name string, _ string) error {
		return errors.New(name + " invalid")
	}), 1)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 5)
	assert.Equal(t, "lookup invalid", result.Messages[2].Content[0].(ToolResultPart).Content[0].(TextPart).Text)
	assert.Equal(t, "weather invalid", result.Messages[3].Content[0].(ToolResultPart).Content[0].(TextPart).Text)
}

func TestExecuteWithToolValidation_ValidToolCallReturnsImmediately(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
			}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), toolValidatorFunc(func(string, string) error {
		return nil
	}), 2)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 2)
	assert.Equal(t, RoleAssistant, result.Messages[1].Role)
}

func TestExecuteWithToolValidation_PlainTextResponseReturnsImmediately(t *testing.T) {
	t.Parallel()

	validatorCalls := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{TextPart{Text: "plain text"}}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), toolValidatorFunc(func(string, string) error {
		validatorCalls++
		return nil
	}), 2)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 2)
	assert.Equal(t, 0, validatorCalls)
}

func TestExecuteWithToolValidation_NilValidatorWithToolCall(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
			}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), nil, 0)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 2)
	assert.Contains(t, err.Error(), "validator is nil")
}

func TestExecuteWithToolValidation_ReturnsLastExecutionOnRetryLimit(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
			}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), toolValidatorFunc(func(string, string) error {
		return errors.New("bad args")
	}), 0)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 3)
	assert.Equal(t, RoleTool, result.Messages[2].Role)
	assert.Contains(t, err.Error(), "bad args")
}
