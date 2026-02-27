package embedregistry

import (
	"context"
	"embed"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

//go:embed testdata/prompts/*.yaml
var promptsFS embed.FS

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEmbedRegistry_New(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	require.NotNil(t, reg)
}

func TestEmbedRegistry_GetTemplate(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent", "")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "agent", tpl.Metadata.ID)
	assert.Contains(t, tpl.Messages[0].Content, "Agent {{ .user_name }}")
}

// TestEmbedRegistry_GetTemplate_BaseFallback ensures env="" returns base file (agent.yaml), not env-specific.
func TestEmbedRegistry_GetTemplate_BaseFallback(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent", "")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	// Base file has "Agent {{ .user_name }}"; agent.prod.yaml has "Agent prod"
	assert.Contains(t, tpl.Messages[0].Content, "Agent {{ .user_name }}")
	assert.NotContains(t, tpl.Messages[0].Content, "Agent prod")
}

func TestEmbedRegistry_GetTemplate_EnvSpecific(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent", "prod")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Contains(t, tpl.Messages[0].Content, "Agent prod")
}

func TestEmbedRegistry_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "nonexistent", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

