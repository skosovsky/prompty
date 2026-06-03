package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompiledPrompt_PromptExecutionIsolatedFromExternalMutation(t *testing.T) {
	t.Parallel()
	exec := SimplePrompt("hello")
	cp, err := NewCompiledPrompt(exec, "agent", []byte("id: agent\n"))
	require.NoError(t, err)

	snap := cp.PromptExecution()
	snap.Messages[0].Content[0] = TextPart{Text: "mutated"}

	restored := cp.PromptExecution()
	text := restored.Messages[0].Content[0].(TextPart).Text
	assert.Equal(t, "hello", text)
}
