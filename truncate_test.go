package prompty

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type truncateCounter struct {
	count   func(string) (int, error)
	penalty int
}

func (c *truncateCounter) Count(text string) (int, error) {
	if c.count != nil {
		return c.count(text)
	}
	return len(text), nil
}

func (c *truncateCounter) MediaTokenPenalty() int {
	if c.penalty > 0 {
		return c.penalty
	}
	return defaultMediaTokenPenalty
}

func TestPromptExecution_Truncate_NoOpWhenUnderBudget(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			NewUserMessage("hi"),
		},
	}

	err := exec.Truncate(100, &truncateCounter{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
}

func TestPromptExecution_Truncate_KeepsProtectedPrefixAndRemovesOrphanToolBlock(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			{Role: RoleDeveloper, Content: []ContentPart{TextPart{Text: "dev"}}},
			{
				Role: RoleAssistant,
				Content: []ContentPart{
					ToolCallPart{ID: "call-1", Name: "lookup", Args: `{"x":"12345"}`},
				},
			},
			newToolResultMessage("call-1", "lookup", "result", false),
			NewUserMessage("latest"),
		},
	}

	counter := &truncateCounter{count: func(s string) (int, error) {
		switch s {
		case "sys", "dev", "latest":
			return 1, nil
		case `{"x":"12345"}`:
			return 5, nil
		case "result":
			return 5, nil
		default:
			return len(s), nil
		}
	}}

	err := exec.Truncate(3, counter)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, RoleDeveloper, exec.Messages[1].Role)
	assert.Equal(t, RoleUser, exec.Messages[2].Role)
	for _, msg := range exec.Messages {
		assert.NotEqual(t, RoleTool, msg.Role)
	}
}

func TestPromptExecution_Truncate_CountsToolArgsReasoningAndMedia(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			{
				Role: RoleUser,
				Content: []ContentPart{
					TextPart{Text: "user"},
					MediaPart{MediaType: "image", MIMEType: "image/png"},
				},
			},
			{
				Role: RoleAssistant,
				Content: []ContentPart{
					ReasoningPart{Text: "chain"},
					ToolCallPart{ID: "call-1", Name: "lookup", Args: `{"x":"long"}`},
				},
			},
		},
	}

	counter := &truncateCounter{
		count: func(s string) (int, error) {
			switch s {
			case "sys":
				return 1, nil
			case "user":
				return 1, nil
			case "chain":
				return 5, nil
			case `{"x":"long"}`:
				return 5, nil
			default:
				return len(s), nil
			}
		},
		penalty: 10,
	}

	err := exec.Truncate(2, counter)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
}

func TestPromptExecution_Truncate_PreservesMidHistorySystemMessages(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys-1"),
			NewUserMessage("old-user"),
			{Role: RoleAssistant, Content: []ContentPart{TextPart{Text: "old-answer"}}},
			NewSystemMessage("sys-2"),
			NewUserMessage("latest"),
		},
	}

	counter := &truncateCounter{count: func(s string) (int, error) {
		switch s {
		case "sys-1", "sys-2", "latest":
			return 1, nil
		case "old-user", "old-answer":
			return 5, nil
		default:
			return len(s), nil
		}
	}}

	err := exec.Truncate(3, counter)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, "sys-1", exec.Messages[0].Content[0].(TextPart).Text)
	assert.Equal(t, RoleSystem, exec.Messages[1].Role)
	assert.Equal(t, "sys-2", exec.Messages[1].Content[0].(TextPart).Text)
	assert.Equal(t, RoleUser, exec.Messages[2].Role)
}

func TestPromptExecution_Truncate_CountsNestedToolResultMedia(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			NewUserMessage("turn-1"),
			{
				Role: RoleAssistant,
				Content: []ContentPart{
					ToolCallPart{ID: "call-1", Name: "lookup", Args: `{}`},
				},
			},
			{
				Role: RoleTool,
				Content: []ContentPart{
					ToolResultPart{
						ToolCallID: "call-1",
						Name:       "lookup",
						Content: []ContentPart{
							MediaPart{MediaType: "image", MIMEType: "image/png"},
						},
					},
				},
			},
			NewUserMessage("latest"),
		},
	}

	counter := &truncateCounter{
		count: func(s string) (int, error) {
			switch s {
			case "sys", "turn-1", "latest", `{}`:
				return 1, nil
			default:
				return len(s), nil
			}
		},
		penalty: 10,
	}

	err := exec.Truncate(4, counter)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, RoleUser, exec.Messages[1].Role)
	assert.Equal(t, "latest", exec.Messages[1].Content[0].(TextPart).Text)
}

func TestPromptExecution_Truncate_PropagatesCounterError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("count failed")
	exec := &PromptExecution{
		Messages: []ChatMessage{NewUserMessage("hi")},
	}

	err := exec.Truncate(1, &truncateCounter{count: func(string) (int, error) {
		return 0, wantErr
	}})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}
