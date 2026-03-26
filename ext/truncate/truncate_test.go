package truncate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type truncateCounter struct {
	count        func(string) (int, error)
	countMessage func(prompty.ChatMessage) (int, error)
}

func (c *truncateCounter) Count(text string) (int, error) {
	if c.count != nil {
		return c.count(text)
	}
	return len(text), nil
}

func (c *truncateCounter) CountMessage(msg prompty.ChatMessage) (int, error) {
	if c.countMessage != nil {
		return c.countMessage(msg)
	}
	return c.Count(prompty.TextFromParts(msg.Content))
}

func TestDropOldest_UnderBudgetNoChanges(t *testing.T) {
	t.Parallel()

	messages := []prompty.ChatMessage{
		prompty.NewSystemMessage("sys"),
		prompty.NewUserMessage("hi"),
	}
	trimmed, err := DropOldest(messages, 100, &truncateCounter{})
	require.NoError(t, err)
	require.Len(t, trimmed, 2)
	assert.Equal(t, messages[0].Role, trimmed[0].Role)
	assert.Equal(t, messages[1].Role, trimmed[1].Role)
}

func TestDropOldest_RemovesOldUserTurns(t *testing.T) {
	t.Parallel()

	messages := []prompty.ChatMessage{
		prompty.NewSystemMessage("sys"),
		prompty.NewUserMessage("old"),
		prompty.NewUserMessage("latest"),
	}
	counter := &truncateCounter{
		count: func(s string) (int, error) {
			switch s {
			case "sys", "latest":
				return 1, nil
			case "old":
				return 4, nil
			default:
				return len(s), nil
			}
		},
	}
	trimmed, err := DropOldest(messages, 2, counter)
	require.NoError(t, err)
	require.Len(t, trimmed, 2)
	assert.Equal(t, prompty.RoleSystem, trimmed[0].Role)
	assert.Equal(t, prompty.RoleUser, trimmed[1].Role)
	assert.Equal(t, "latest", prompty.TextFromParts(trimmed[1].Content))
}

func TestDropOldest_ProtectsDeveloperOutsidePrefix(t *testing.T) {
	t.Parallel()

	messages := []prompty.ChatMessage{
		prompty.NewSystemMessage("sys"),
		prompty.NewUserMessage("old"),
		{
			Role:    prompty.RoleDeveloper,
			Content: []prompty.ContentPart{prompty.TextPart{Text: "dev-guard"}},
		},
		prompty.NewUserMessage("latest"),
	}
	counter := &truncateCounter{
		count: func(s string) (int, error) {
			switch s {
			case "old":
				return 8, nil
			default:
				return 1, nil
			}
		},
	}
	trimmed, err := DropOldest(messages, 3, counter)
	require.NoError(t, err)
	require.Len(t, trimmed, 3)
	assert.Equal(t, prompty.RoleSystem, trimmed[0].Role)
	assert.Equal(t, prompty.RoleDeveloper, trimmed[1].Role)
	assert.Equal(t, prompty.RoleUser, trimmed[2].Role)
	assert.Equal(t, "latest", prompty.TextFromParts(trimmed[2].Content))
}
