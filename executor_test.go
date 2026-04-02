package prompty

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStructuredExecutor_ValidationErrorAppendsAssistantAndUser(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	callNum := 0
	var seen []*PromptExecution
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			seen = append(seen, clonePromptExecution(exec))
			if callNum == 1 {
				return NewResponse([]ContentPart{TextPart{Text: `{invalid`}}), nil
			}
			return NewResponse([]ContentPart{TextPart{Text: `{"name":"ok"}`}}), nil
		},
	}

	step := NewStructuredExecutor[result](invoker, SimplePrompt("hi"))

	ptr, err := step(context.Background())
	require.Error(t, err)
	require.Nil(t, ptr)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)

	ptr, err = step(context.Background())
	require.NoError(t, err)
	require.NotNil(t, ptr)
	assert.Equal(t, "ok", ptr.Name)

	require.Equal(t, 2, callNum)
	require.Len(t, seen, 2)
	require.Len(t, seen[1].Messages, 3)
	assert.Equal(t, RoleAssistant, seen[1].Messages[1].Role)
	assert.Equal(t, `{invalid`, TextFromParts(seen[1].Messages[1].Content))
	assert.Equal(t, RoleUser, seen[1].Messages[2].Role)
	assert.Contains(t, TextFromParts(seen[1].Messages[2].Content), "invalid character")
}

func TestNewStructuredExecutor_ToolCallErrorAppendsAssistantAndTool(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	callNum := 0
	var seen []*PromptExecution
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			seen = append(seen, clonePromptExecution(exec))
			if callNum == 1 {
				msg := newAssistantMessageWithContent([]ContentPart{
					TextPart{Text: "Calling tool"},
					ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
					ToolCallPart{ID: "tool-2", Name: "weather", Args: `{}`},
				})
				return nil, &ToolCallError{
					RawAssistantMessage: &msg,
					ToolResults: []ContentPart{
						newToolResultPart("tool-1", "lookup", "lookup invalid", true),
						newToolResultPart("tool-2", "weather", "weather invalid", true),
					},
					Err: errors.New("invalid tool batch"),
				}
			}
			return NewResponse([]ContentPart{TextPart{Text: `{"name":"ok"}`}}), nil
		},
	}

	step := NewStructuredExecutor[result](invoker, SimplePrompt("hi"))

	ptr, err := step(context.Background())
	require.Error(t, err)
	require.Nil(t, ptr)
	var toolErr *ToolCallError
	require.ErrorAs(t, err, &toolErr)

	ptr, err = step(context.Background())
	require.NoError(t, err)
	require.NotNil(t, ptr)
	assert.Equal(t, "ok", ptr.Name)

	require.Len(t, seen, 2)
	require.Len(t, seen[1].Messages, 3)
	assert.Equal(t, RoleAssistant, seen[1].Messages[1].Role)
	assert.Equal(t, RoleTool, seen[1].Messages[2].Role)
	require.Len(t, seen[1].Messages[2].Content, 2)
	assert.Equal(t, "lookup invalid", seen[1].Messages[2].Content[0].(ToolResultPart).Content[0].(TextPart).Text)
	assert.Equal(t, "weather invalid", seen[1].Messages[2].Content[1].(ToolResultPart).Content[0].(TextPart).Text)
}

func TestNewStructuredExecutor_ContextCanceledBeforeCall(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	step := NewStructuredExecutor[result](&scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) {
			t.Fatal("Execute must not run when ctx is already canceled")
			return nil, errors.New("unreachable: invoker should not run")
		},
	}, SimplePrompt("hi"))

	ptr, err := step(ctx)
	require.Error(t, err)
	assert.Nil(t, ptr)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewStructuredExecutor_NilExecution(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}
	step := NewStructuredExecutor[result](&scriptedInvoker{}, nil)
	_, err := step(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execution is nil")
}

func TestNewStructuredExecutor_NilInvoker(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}
	step := NewStructuredExecutor[result](nil, SimplePrompt("hi"))
	_, err := step(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invoker is nil")
}
