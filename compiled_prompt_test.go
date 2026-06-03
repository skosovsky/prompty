package prompty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCompiledPrompt_RequiresManifestBytes(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	_, err := NewCompiledPrompt(exec, "agent", nil)
	require.ErrorIs(t, err, ErrManifestBytesRequired)
}

func TestCompiledPrompt_RoundTrip(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.Metadata.ID = "agent"
	raw := []byte("id: agent\nmessages:\n  - role: user\n    content: hi\n")

	cp, err := NewCompiledPrompt(exec, "agent", raw)
	require.NoError(t, err)
	assert.Equal(t, DigestSourceManifestBytes, cp.DigestSource())
	assert.NotEmpty(t, cp.ManifestDigest())

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var restored CompiledPrompt
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.Equal(t, cp.ManifestDigest(), restored.ManifestDigest())
	require.NotNil(t, restored.PromptExecution())
	assert.Equal(t, "hi", mustTextFromParts(t, restored.PromptExecution().Messages[0].Content))
}

func TestRenderPlan_Compile(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "t"}))
	require.NoError(t, err)

	plan, err := newRenderPlanFromMap(tpl, map[string]any{"q": "x"})
	require.NoError(t, err)
	cp, err := plan.Compile(context.Background(), "t", []byte("id: t"))
	require.NoError(t, err)
	assert.Equal(t, "x", mustTextFromParts(t, cp.PromptExecution().Messages[0].Content))
}
