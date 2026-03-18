package embedregistry

import (
	"context"
	"embed"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

//go:embed testdata/prompts/*.json
var promptsFS embed.FS

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEmbedRegistry_New(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	require.NotNil(t, reg)
}

func TestEmbedRegistry_GetTemplate(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "agent", tpl.Metadata.ID)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Contains(t, tpl.Messages[0].Content[0].Text, "Agent {{ .user_name }}")
}

// TestEmbedRegistry_GetTemplate_BaseId returns base file for id "agent".
func TestEmbedRegistry_GetTemplate_BaseId(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Contains(t, tpl.Messages[0].Content[0].Text, "Agent {{ .user_name }}")
	assert.NotContains(t, tpl.Messages[0].Content[0].Text, "Agent prod")
}

func TestEmbedRegistry_GetTemplate_EnvSpecific(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()), WithEnvironment("prod"))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Contains(t, tpl.Messages[0].Content[0].Text, "Agent prod", "env variant should be preferred")
}

// TestEmbedRegistry_GetTemplate_NotFound ensures missing id returns ErrTemplateNotFound.
func TestEmbedRegistry_GetTemplate_NotFoundId(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "agent/staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestEmbedRegistry_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestEmbedRegistry_List(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	ids, err := reg.List(ctx)
	require.NoError(t, err)
	// List returns base IDs only; agent.prod and agent both yield "agent"
	assert.Contains(t, ids, "agent")
	assert.NotContains(t, ids, "agent.prod")
}

func TestEmbedRegistry_List_BaseIDNormalization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		fsys   fs.FS
		root   string
		wantID string
	}{
		{"base+env json", fstest.MapFS{
			"p/agent.json":      &fstest.MapFile{Data: []byte(`{"id":"agent","messages":[{"role":"system","content":[{"type":"text","text":"Base"}]}]}`)},
			"p/agent.prod.json": &fstest.MapFile{Data: []byte(`{"id":"agent","messages":[{"role":"system","content":[{"type":"text","text":"Prod"}]}]}`)},
		}, "p", "agent"},
		{"nested slash path json", fstest.MapFS{
			"q/internal/router.prod.json": &fstest.MapFile{Data: []byte(`{"id":"internal/router","messages":[{"role":"system","content":[{"type":"text","text":"Router"}]}]}`)},
		}, "q", "internal/router"},
		{"extensions yaml yml", fstest.MapFS{
			"r/foo.yaml": &fstest.MapFile{Data: []byte(`{"id":"foo","messages":[{"role":"system","content":[{"type":"text","text":"Y"}]}]}`)},
			"r/bar.yml":  &fstest.MapFile{Data: []byte(`{"id":"bar","messages":[{"role":"system","content":[{"type":"text","text":"Y"}]}]}`)},
		}, "r", "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg, err := New(tt.fsys, tt.root, WithParser(manifest.NewJSONParser()))
			require.NoError(t, err)
			ids, err := reg.List(context.Background())
			require.NoError(t, err)
			assert.Contains(t, ids, tt.wantID)
			// List returns base IDs only; env suffix must not appear
			assert.NotContains(t, ids, tt.wantID+".prod")
		})
	}
}

func TestEmbedRegistry_List_ExcludesPartials(t *testing.T) {
	t.Parallel()
	mapFS := fstest.MapFS{
		"prompts/main.json":                &fstest.MapFile{Data: []byte(`{"id":"main","messages":[{"role":"system","content":[{"type":"text","text":"Main"}]}]}`)},
		"prompts/partials/dummy.tmpl":      &fstest.MapFile{Data: []byte(`{{ define "dummy" }}x{{ end }}`)},
		"prompts/partials/accidental.yaml": &fstest.MapFile{Data: []byte(`{"id":"accidental","messages":[{"role":"system","content":[{"type":"text","text":"Should not appear"}]}]}`)},
	}
	reg, err := New(mapFS, "prompts", WithPartials("partials/*.tmpl"), WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ids, err := reg.List(context.Background())
	require.NoError(t, err)
	assert.Contains(t, ids, "main", "manifest in root should appear")
	assert.NotContains(t, ids, "partials/accidental", "manifests under partials dir must be excluded")
}

func TestEmbedRegistry_Stat(t *testing.T) {
	t.Parallel()
	reg, err := New(promptsFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	info, err := reg.Stat(ctx, "agent")
	require.NoError(t, err)
	assert.Equal(t, "agent", info.ID)
	_, err = reg.Stat(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestEmbedRegistry_Stat_EnvFallback(t *testing.T) {
	t.Parallel()
	// Only env variant exists; Stat should find it via same candidate lookup as GetTemplate.
	mapFS := fstest.MapFS{
		"p/agent.prod.json": &fstest.MapFile{Data: []byte(`{"id":"agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Prod only"}]}]}`)},
	}
	reg, err := New(mapFS, "p", WithParser(manifest.NewJSONParser()), WithEnvironment("prod"))
	require.NoError(t, err)
	ctx := context.Background()
	info, err := reg.Stat(ctx, "agent")
	require.NoError(t, err)
	assert.Equal(t, "agent", info.ID)
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Contains(t, tpl.Messages[0].Content[0].Text, "Prod only")
}

func TestEmbedRegistry_WithVersion(t *testing.T) {
	t.Parallel()
	mapFS := fstest.MapFS{"v/agent.json": &fstest.MapFile{Data: []byte(`{"id":"agent","version":"","messages":[{"role":"system","content":[{"type":"text","text":"Hi"}]}]}`)}}
	reg, err := New(mapFS, "v", WithVersion("abc123"), WithParser(manifest.NewJSONParser()))
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
		"prompts/doctor.json": &fstest.MapFile{
			Data: []byte(`{"id":"doctor","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"You are a doctor assistant.\n{{ template \"safety\" }}"}]},{"role":"user","content":[{"type":"text","text":"Hi"}]}]}`),
		},
		"prompts/partials/safety.tmpl": &fstest.MapFile{
			Data: []byte(`{{ define "safety" }}Never give medical diagnoses.{{ end }}`),
		},
	}
	reg, err := New(mapFS, "prompts", WithPartials("partials/*.tmpl"), WithParser(manifest.NewJSONParser()))
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
