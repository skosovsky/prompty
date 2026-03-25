package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentPart_Implementations(t *testing.T) {
	t.Parallel()
	// Compile-time: only our types implement ContentPart.
	// Note: While pointers satisfy the interface, ProviderAdapters MUST return slices of value-types only.
	var _ ContentPart = (*TextPart)(nil)
	var _ ContentPart = (*MediaPart)(nil)
	var _ ContentPart = (*ToolCallPart)(nil)
	var _ ContentPart = (*ToolResultPart)(nil)
}

func TestContentPart_RuntimeAssertions(t *testing.T) {
	t.Parallel()
	parts := []ContentPart{
		TextPart{Text: "hi"},
		MediaPart{MediaType: "image", URL: "https://example.com/img.png", MIMEType: "image/png"},
		ToolCallPart{ID: "1", Name: "foo", Args: "{}"},
		ToolResultPart{ToolCallID: "1", Name: "foo", Content: []ContentPart{TextPart{Text: "ok"}}, IsError: false},
	}
	for i, p := range parts {
		require.NotNil(t, p, "part %d", i)
		// Type assertions
		switch p.(type) {
		case TextPart:
		case MediaPart:
		case ToolCallPart:
		case ToolResultPart:
		default:
			t.Errorf("unknown ContentPart type %T", p)
		}
	}
}

func TestChatMessage_WithContentParts(t *testing.T) {
	t.Parallel()
	msg := ChatMessage{
		Role: "user",
		Content: []ContentPart{
			TextPart{Text: "What is this?"},
			MediaPart{MediaType: "image", URL: "https://example.com/pic.png", MIMEType: "image/png"},
		},
	}
	assert.Equal(t, RoleUser, msg.Role)
	require.Len(t, msg.Content, 2)
	assert.IsType(t, TextPart{}, msg.Content[0])
	assert.IsType(t, MediaPart{}, msg.Content[1])
}

func TestPromptExecution_ImmutableShape(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{{Role: "system", Content: []ContentPart{TextPart{Text: "Hi"}}}},
		Metadata: PromptMetadata{ID: "test", Version: "1"},
	}
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "test", exec.Metadata.ID)
}

func TestPromptExecution_WithHistory_AddMessage(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{{Role: "system", Content: []ContentPart{TextPart{Text: "Hi"}}}},
		Metadata: PromptMetadata{ID: "x", Version: "1"},
	}
	origLen := len(exec.Messages)
	withHistory := exec.WithHistory([]ChatMessage{{Role: "user", Content: []ContentPart{TextPart{Text: "Hello"}}}})
	require.Len(t, exec.Messages, origLen)
	require.Len(t, withHistory.Messages, origLen+1)
	withExtra := withHistory.AddMessage(ChatMessage{Role: "assistant", Content: []ContentPart{TextPart{Text: "Bye"}}})
	require.Len(t, withHistory.Messages, origLen+1)
	require.Len(t, withExtra.Messages, origLen+2)
}

// TestPromptExecution_WithHistory_AddMessage_DoesNotMutateOriginals verifies the immutable helper contract:
// WithHistory and AddMessage must return new executions and leave the source execution/history untouched.
func TestPromptExecution_WithHistory_AddMessage_DoesNotMutateOriginals(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleSystem, Content: []ContentPart{TextPart{Text: "System"}}},
			{Role: RoleUser, Content: []ContentPart{TextPart{Text: "User1"}}},
		},
		Metadata: PromptMetadata{ID: "immut", Version: "1"},
	}
	origMessages := exec.Messages
	origLen := len(origMessages)
	origSystemText := origMessages[0].Content[0].(TextPart).Text
	origUserText := origMessages[1].Content[0].(TextPart).Text

	history := []ChatMessage{
		{Role: RoleAssistant, Content: []ContentPart{TextPart{Text: "Hist1"}}},
	}
	histLen := len(history)
	histText := history[0].Content[0].(TextPart).Text

	extra := ChatMessage{Role: RoleUser, Content: []ContentPart{TextPart{Text: "Extra"}}}
	withHistory := exec.WithHistory(history)
	withExtra := withHistory.AddMessage(extra)

	require.Len(t, exec.Messages, origLen, "receiver must not be mutated")
	require.Len(t, origMessages, origLen, "original Messages slice must not be mutated")
	assert.Equal(t, origSystemText, origMessages[0].Content[0].(TextPart).Text)
	assert.Equal(t, origUserText, origMessages[1].Content[0].(TextPart).Text)

	require.Len(t, history, histLen)
	assert.Equal(t, histText, history[0].Content[0].(TextPart).Text)

	require.Len(t, withHistory.Messages, origLen+histLen)
	require.Len(t, withExtra.Messages, origLen+histLen+1)
	assert.Equal(t, "System", withExtra.Messages[0].Content[0].(TextPart).Text)
	assert.Equal(t, "User1", withExtra.Messages[1].Content[0].(TextPart).Text)
	assert.Equal(t, "Hist1", withExtra.Messages[2].Content[0].(TextPart).Text)
	assert.Equal(t, "Extra", withExtra.Messages[3].Content[0].(TextPart).Text)
}

func TestNewExecution_DefensiveCopy(t *testing.T) {
	t.Parallel()
	messages := []ChatMessage{
		{
			Role: RoleUser,
			Content: []ContentPart{
				MediaPart{MediaType: "image", URL: "https://example.com/img.png", Data: []byte("abc")},
			},
			Metadata: map[string]any{"nested": map[string]any{"env": "dev"}},
		},
	}
	exec := NewExecution(messages)
	require.NotNil(t, exec)
	require.Len(t, exec.Messages, 1)

	messages[0].Metadata["nested"].(map[string]any)["env"] = "prod"
	part := messages[0].Content[0].(MediaPart)
	part.Data[0] = 'X'
	messages[0].Content[0] = part

	assert.Equal(t, "dev", exec.Messages[0].Metadata["nested"].(map[string]any)["env"])
	assert.Equal(t, byte('a'), exec.Messages[0].Content[0].(MediaPart).Data[0])
}
