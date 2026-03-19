package prompty

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithToolValidation_InvalidToolCallReturnsToolCallError(t *testing.T) {
	t.Parallel()

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			require.Len(t, exec.Messages, 1)
			return NewResponse([]ContentPart{
				TextPart{Text: "Calling tool"},
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{"city":1}`},
			}), nil
		},
	}

	exec := SimplePrompt("hi")
	result, err := ExecuteWithToolValidation(context.Background(), invoker, exec, toolValidatorFunc(func(name string, argsJSON string) error {
		assert.Equal(t, "lookup", name)
		assert.JSONEq(t, `{"city":1}`, argsJSON)
		return errors.New("city must be a string")
	}))
	require.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, callNum)
	assert.Len(t, result.Messages, 1)
	assert.Len(t, exec.Messages, 1)

	var toolErr *ToolCallError
	require.ErrorAs(t, err, &toolErr)
	require.NotNil(t, toolErr.RawAssistantMessage)
	assert.Equal(t, RoleAssistant, toolErr.RawAssistantMessage.Role)
	require.Len(t, toolErr.ToolResults, 1)
	part := toolErr.ToolResults[0].(ToolResultPart)
	assert.Equal(t, "tool-1", part.ToolCallID)
	assert.Equal(t, "lookup", part.Name)
	assert.True(t, part.IsError)
	assert.Equal(t, "city must be a string", part.Content[0].(TextPart).Text)
}

func TestExecuteWithToolValidation_MultipleInvalidToolCallsReturnAllResults(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
				ToolCallPart{ID: "tool-2", Name: "weather", Args: `{}`},
			}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), toolValidatorFunc(func(name string, _ string) error {
		return errors.New(name + " invalid")
	}))
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 1)

	var toolErr *ToolCallError
	require.ErrorAs(t, err, &toolErr)
	require.Len(t, toolErr.ToolResults, 2)
	assert.Equal(t, "lookup invalid", toolErr.ToolResults[0].(ToolResultPart).Content[0].(TextPart).Text)
	assert.Equal(t, "weather invalid", toolErr.ToolResults[1].(ToolResultPart).Content[0].(TextPart).Text)
}

func TestExecuteWithToolValidation_ValidToolCallReturnsImmediately(t *testing.T) {
	t.Parallel()

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			callNum++
			return NewResponse([]ContentPart{
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
			}), nil
		},
	}

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), toolValidatorFunc(func(string, string) error {
		return nil
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, callNum)
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
	}))
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

	result, err := ExecuteWithToolValidation(context.Background(), invoker, SimplePrompt("hi"), nil)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 1)
	assert.Contains(t, err.Error(), "validator is nil")
}
