package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompiledPrompt_FallbackDigestIndependentOfInput(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "agent"}))
	require.NoError(t, err)

	planA, err := newRenderPlanFromMap(tpl, map[string]any{"q": "alpha"})
	require.NoError(t, err)
	cpA, err := planA.CompileFromRegistryWithCanonicalSnapshot(context.Background(), "agent")
	require.NoError(t, err)

	planB, err := newRenderPlanFromMap(tpl, map[string]any{"q": "beta"})
	require.NoError(t, err)
	cpB, err := planB.CompileFromRegistryWithCanonicalSnapshot(context.Background(), "agent")
	require.NoError(t, err)

	assert.Equal(t, cpA.ManifestDigest(), cpB.ManifestDigest())
	assert.Equal(t, DigestSourceCanonicalSnapshot, cpA.DigestSource())
	execA := cpA.PromptExecution()
	execB := cpB.PromptExecution()
	textA := execA.Messages[0].Content[0].(TextPart).Text
	textB := execB.Messages[0].Content[0].(TextPart).Text
	assert.NotEqual(t, textA, textB)
}
