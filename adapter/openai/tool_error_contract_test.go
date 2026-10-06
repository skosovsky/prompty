package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestChatToolErrorCannotBecomeSuccess(t *testing.T) {
	t.Parallel()
	for _, part := range []prompty.ContentPart{
		prompty.ToolResultPart{ToolCallID: "call", Name: "lookup", IsError: true, Content: []prompty.ContentPart{prompty.TextPart{Text: "failed"}}},
		&prompty.ToolResultPart{ToolCallID: "call", Name: "lookup", IsError: true, Content: []prompty.ContentPart{prompty.TextPart{Text: "failed"}}},
	} {
		// Arrange.
		execution := prompty.SimplePrompt("fixture").
			AddMessage(prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{part}})
		// Act.
		request, err := New().Translate(execution)
		// Assert.
		require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
		require.Nil(t, request)
	}
}
