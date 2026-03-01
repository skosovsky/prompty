package embedregistry

import (
	"context"
	"embed"
	"testing"
	"testing/fstest"

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
	assert.Empty(t, tpl.Metadata.Environment)
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
	assert.Empty(t, tpl.Metadata.Environment)
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
	assert.Equal(t, "prod", tpl.Metadata.Environment)
}

// TestEmbedRegistry_GetTemplate_EnvFallback ensures that when env-specific file is missing,
// fallback to base file still sets Metadata.Environment to the requested env.
func TestEmbedRegistry_GetTemplate_EnvFallback(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	// No agent.staging.yaml exists; fallback to agent.yaml
	tpl, err := reg.GetTemplate(ctx, "agent", "staging")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Contains(t, tpl.Messages[0].Content, "Agent {{ .user_name }}", "content from base file")
	assert.Equal(t, "staging", tpl.Metadata.Environment, "Environment must be set to requested env on fallback")
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

func TestEmbedRegistry_GetTemplate_WithPartials(t *testing.T) {
	t.Parallel()
	// Use MapFS to simulate an embed with a manifest and partials; no real embed needed.
	mapFS := fstest.MapFS{
		"prompts/doctor.yaml": &fstest.MapFile{
			Data: []byte(`
id: doctor
version: "1"
messages:
  - role: system
    content: |
      You are a doctor assistant.
      {{ template "safety" }}
  - role: user
    content: "Hi"
`),
		},
		"prompts/partials/safety.tmpl": &fstest.MapFile{
			Data: []byte(`{{ define "safety" }}Never give medical diagnoses.{{ end }}`),
		},
	}
	reg, err := New(mapFS, "prompts", WithPartials("partials/*.tmpl"))
	require.NoError(t, err)
	require.NotNil(t, reg)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "doctor", "")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	exec, err := tpl.FormatStruct(ctx, &struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.Len(t, exec.Messages, 2)
	require.Len(t, exec.Messages[0].Content, 1)
	textPart, ok := exec.Messages[0].Content[0].(prompty.TextPart)
	require.True(t, ok)
	assert.Contains(t, textPart.Text, "Never give medical diagnoses.", "partial 'safety' must be rendered into message")
	assert.Contains(t, textPart.Text, "You are a doctor assistant.")
}
