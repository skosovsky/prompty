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
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "agent", tpl.Metadata.ID)
	assert.Contains(t, tpl.Messages[0].Content, "Agent {{ .user_name }}")
}

// TestEmbedRegistry_GetTemplate_BaseId returns base file for id "agent".
func TestEmbedRegistry_GetTemplate_BaseId(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Contains(t, tpl.Messages[0].Content, "Agent {{ .user_name }}")
	assert.NotContains(t, tpl.Messages[0].Content, "Agent prod")
}

func TestEmbedRegistry_GetTemplate_EnvSpecific(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent.prod")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Contains(t, tpl.Messages[0].Content, "Agent prod")
}

// TestEmbedRegistry_GetTemplate_NotFound ensures missing id returns ErrTemplateNotFound.
func TestEmbedRegistry_GetTemplate_NotFoundId(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "agent.staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestEmbedRegistry_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestEmbedRegistry_List(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	ids, err := reg.List(ctx)
	require.NoError(t, err)
	assert.Contains(t, ids, "agent")
	assert.Contains(t, ids, "agent.prod")
}

func TestEmbedRegistry_Stat(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts")
	require.NoError(t, err)
	ctx := context.Background()
	info, err := reg.Stat(ctx, "agent")
	require.NoError(t, err)
	assert.Equal(t, "agent", info.ID)
	_, err = reg.Stat(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestEmbedRegistry_WithVersion(t *testing.T) {
	t.Parallel()
	mapFS := fstest.MapFS{"v/agent.yaml": &fstest.MapFile{Data: []byte("id: agent\nversion: \"\"\nmessages:\n  - role: system\n    content: Hi\n")}}
	reg, err := New(mapFS, "v", WithVersion("abc123"))
	require.NoError(t, err)
	ctx := context.Background()
	info, err := reg.Stat(ctx, "agent")
	require.NoError(t, err)
	assert.Equal(t, "abc123", info.Version)
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "abc123", tpl.Metadata.Version)
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
	tpl, err := reg.GetTemplate(ctx, "doctor")
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
