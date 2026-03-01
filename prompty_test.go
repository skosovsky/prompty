package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentPart_Implementations(t *testing.T) {
	t.Parallel()
	// Compile-time: only our types implement ContentPart
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
		Messages:    []ChatMessage{{Role: "system", Content: []ContentPart{TextPart{Text: "Hi"}}}},
		Tools:       nil,
		ModelConfig: map[string]any{"temperature": 0.7},
		Metadata:    PromptMetadata{ID: "test", Version: "1"},
	}
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "test", exec.Metadata.ID)
}
