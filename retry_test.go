package prompty

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRetry_ValidationErrorAppendsAssistantAndUser(t *testing.T) {
	t.Parallel()

	callNum := 0
	var seen []*PromptExecution

	result, err := WithRetry(context.Background(), SimplePrompt("hi"), 1, func(_ context.Context, exec *PromptExecution) (string, error) {
		callNum++
		seen = append(seen, clonePromptExecution(exec))
		if callNum == 1 {
			msg := NewAssistantMessage(`{invalid`)
			return "", &ValidationError{
				RawAssistantMessage: &msg,
				FeedbackPrompt:      "JSON validation failed: invalid character. Please fix your output.",
				Err:                 errors.New("invalid character"),
			}
		}
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	require.Len(t, seen, 2)
	require.Len(t, seen[1].Messages, 3)
	assert.Equal(t, RoleAssistant, seen[1].Messages[1].Role)
	assert.Equal(t, `{invalid`, TextFromParts(seen[1].Messages[1].Content))
	assert.Equal(t, RoleUser, seen[1].Messages[2].Role)
	assert.Equal(t, "JSON validation failed: invalid character. Please fix your output.", TextFromParts(seen[1].Messages[2].Content))
}

func TestWithRetry_ToolCallErrorAppendsAssistantAndTool(t *testing.T) {
	t.Parallel()

	callNum := 0
	var seen []*PromptExecution

	result, err := WithRetry(context.Background(), SimplePrompt("hi"), 1, func(_ context.Context, exec *PromptExecution) (string, error) {
		callNum++
		seen = append(seen, clonePromptExecution(exec))
		if callNum == 1 {
			msg := newAssistantMessageWithContent([]ContentPart{
				TextPart{Text: "Calling tool"},
				ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
				ToolCallPart{ID: "tool-2", Name: "weather", Args: `{}`},
			})
			return "", &ToolCallError{
				RawAssistantMessage: &msg,
				ToolResults: []ContentPart{
					newToolResultPart("tool-1", "lookup", "lookup invalid", true),
					newToolResultPart("tool-2", "weather", "weather invalid", true),
				},
				Err: errors.New("invalid tool batch"),
			}
		}
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	require.Len(t, seen, 2)
	require.Len(t, seen[1].Messages, 3)
	assert.Equal(t, RoleAssistant, seen[1].Messages[1].Role)
	assert.Equal(t, RoleTool, seen[1].Messages[2].Role)
	require.Len(t, seen[1].Messages[2].Content, 2)
	assert.Equal(t, "lookup invalid", seen[1].Messages[2].Content[0].(ToolResultPart).Content[0].(TextPart).Text)
	assert.Equal(t, "weather invalid", seen[1].Messages[2].Content[1].(ToolResultPart).Content[0].(TextPart).Text)
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	callNum := 0
	start := time.Now()
	result, err := WithRetry(ctx, SimplePrompt("hi"), 5, func(_ context.Context, _ *PromptExecution) (string, error) {
		callNum++
		time.Sleep(30 * time.Millisecond)
		msg := NewAssistantMessage(`{invalid`)
		return "", &ValidationError{
			RawAssistantMessage: &msg,
			FeedbackPrompt:      "fix json",
			Err:                 errors.New("invalid json"),
		}
	})
	require.Error(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 1, callNum)
	assert.Contains(t, err.Error(), "retry aborted due to context cancellation")
	assert.Less(t, time.Since(start), 150*time.Millisecond)
}
