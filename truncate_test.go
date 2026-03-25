package prompty

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type truncateCounter struct {
	count        func(string) (int, error)
	countMessage func(ChatMessage) (int, error)
	penalty      int
}

type aliasingStrategy struct{}

func (aliasingStrategy) Truncate(messages []ChatMessage, _ int, _ TokenCounter) ([]ChatMessage, error) {
	return messages, nil
}

func (c *truncateCounter) Count(text string) (int, error) {
	if c.count != nil {
		return c.count(text)
	}
	return len(text), nil
}

func (c *truncateCounter) CountMessage(msg ChatMessage) (int, error) {
	if c.countMessage != nil {
		return c.countMessage(msg)
	}
	penalty := c.penalty
	if penalty <= 0 {
		penalty = defaultMediaTokenPenalty
	}
	return countContentPartsWithTextCounter(msg.Content, c, penalty)
}

func TestPromptExecution_Truncated_NoOpWhenUnderBudget(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			NewUserMessage("hi"),
		},
	}

	trimmed, err := exec.Truncated(100, &truncateCounter{}, DropOldestStrategy{})
	require.NoError(t, err)
	require.NotSame(t, exec, trimmed)
	require.Len(t, exec.Messages, 2)
	require.Len(t, trimmed.Messages, 2)
	trimmed.Messages[0].Metadata = map[string]any{"mutated": true}
	assert.Nil(t, exec.Messages[0].Metadata)
}

func TestPromptExecution_Truncated_TypedNilStrategyFallsBackToDefault(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			NewUserMessage("older"),
			NewUserMessage("latest"),
		},
	}
	counter := &truncateCounter{
		count: func(s string) (int, error) {
			switch s {
			case "sys", "latest":
				return 1, nil
			case "older":
				return 4, nil
			default:
				return len(s), nil
			}
		},
	}

	expected, err := exec.Truncated(2, counter, DropOldestStrategy{})
	require.NoError(t, err)

	var strategy *DropOldestStrategy
	require.NotPanics(t, func() {
		trimmed, err := exec.Truncated(2, counter, strategy)
		require.NoError(t, err)
		assert.Equal(t, expected.Messages, trimmed.Messages)
	})
}

func TestPromptExecution_Truncated_KeepsProtectedPrefixAndRemovesOrphanToolBlock(t *testing.T) {
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

	trimmed, err := exec.Truncated(3, counter, DropOldestStrategy{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 5)
	require.Len(t, trimmed.Messages, 3)
	assert.Equal(t, RoleSystem, trimmed.Messages[0].Role)
	assert.Equal(t, RoleDeveloper, trimmed.Messages[1].Role)
	assert.Equal(t, RoleUser, trimmed.Messages[2].Role)
	for _, msg := range trimmed.Messages {
		assert.NotEqual(t, RoleTool, msg.Role)
	}
}

func TestPromptExecution_Truncated_CountsToolArgsReasoningAndMedia(t *testing.T) {
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

	trimmed, err := exec.Truncated(2, counter, DropOldestStrategy{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	require.Len(t, trimmed.Messages, 1)
	assert.Equal(t, RoleSystem, trimmed.Messages[0].Role)
}

func TestPromptExecution_Truncated_PreservesMidHistorySystemMessages(t *testing.T) {
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

	trimmed, err := exec.Truncated(3, counter, DropOldestStrategy{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 5)
	require.Len(t, trimmed.Messages, 3)
	assert.Equal(t, RoleSystem, trimmed.Messages[0].Role)
	assert.Equal(t, "sys-1", trimmed.Messages[0].Content[0].(TextPart).Text)
	assert.Equal(t, RoleSystem, trimmed.Messages[1].Role)
	assert.Equal(t, "sys-2", trimmed.Messages[1].Content[0].(TextPart).Text)
	assert.Equal(t, RoleUser, trimmed.Messages[2].Role)
}

func TestPromptExecution_Truncated_CountsNestedToolResultMedia(t *testing.T) {
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

	trimmed, err := exec.Truncated(4, counter, DropOldestStrategy{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 5)
	require.Len(t, trimmed.Messages, 2)
	assert.Equal(t, RoleSystem, trimmed.Messages[0].Role)
	assert.Equal(t, RoleUser, trimmed.Messages[1].Role)
	assert.Equal(t, "latest", trimmed.Messages[1].Content[0].(TextPart).Text)
}

func TestPromptExecution_Truncated_PropagatesCounterError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("count failed")
	exec := &PromptExecution{
		Messages: []ChatMessage{NewUserMessage("hi")},
	}

	_, err := exec.Truncated(1, &truncateCounter{count: func(string) (int, error) {
		return 0, wantErr
	}}, DropOldestStrategy{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestPromptExecution_Truncated_DoesNotMutateOriginalExecution(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			NewSystemMessage("sys"),
			NewUserMessage("turn-1"),
			NewAssistantMessage("answer-1"),
			NewUserMessage("latest"),
		},
		Metadata: PromptMetadata{
			Extras: map[string]any{"trace": map[string]any{"env": "dev"}},
		},
	}

	counter := &truncateCounter{count: func(s string) (int, error) {
		switch s {
		case "sys", "latest":
			return 1, nil
		default:
			return 5, nil
		}
	}}

	trimmed, err := exec.Truncated(3, counter, DropOldestStrategy{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 4)
	require.Len(t, trimmed.Messages, 2)

	trimmed.Metadata.Extras["trace"].(map[string]any)["env"] = "prod"
	trimmed.Messages[0].Content[0] = TextPart{Text: "mutated"}

	assert.Equal(t, "dev", exec.Metadata.Extras["trace"].(map[string]any)["env"])
	assert.Equal(t, "sys", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestPromptExecution_Truncated_ClonesExternalStrategyResult(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			{
				Role:     RoleSystem,
				Content:  []ContentPart{TextPart{Text: "sys"}},
				Metadata: map[string]any{"trace": map[string]any{"id": "original"}},
			},
			NewUserMessage("latest"),
		},
	}

	trimmed, err := exec.Truncated(100, &truncateCounter{}, aliasingStrategy{})
	require.NoError(t, err)
	require.Len(t, trimmed.Messages, 2)

	trimmed.Messages[0].Content[0] = TextPart{Text: "mutated"}
	trimmed.Messages[0].Metadata["trace"].(map[string]any)["id"] = "mutated"

	assert.Equal(t, "sys", exec.Messages[0].Content[0].(TextPart).Text)
	assert.Equal(t, "original", exec.Messages[0].Metadata["trace"].(map[string]any)["id"])
}
