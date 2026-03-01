package fileregistry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestFileRegistry_GetTemplate_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "support_agent.yaml")
	data := []byte(`
id: support_agent
version: "1"
messages:
  - role: system
    content: "Hello {{ .user_name }}"
`)
	require.NoError(t, os.WriteFile(dest, data, 0600))
	reg := New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "support_agent", tpl.Metadata.ID)
}

func TestFileRegistry_GetTemplate_ById(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "support_agent.yaml")
	require.NoError(t, os.WriteFile(basePath, []byte(`
id: support_agent
version: "1"
messages:
  - role: system
    content: "Base {{ .user_name }}"
`), 0600))
	reg := New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Contains(t, tpl.Messages[0].Content, "Base")
}

func TestFileRegistry_GetTemplate_IdWithDot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "support_agent.yaml"), []byte(`
id: support_agent
version: "1"
messages:
  - role: system
    content: "Base"
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "support_agent.production.yaml"), []byte(`
id: support_agent
version: "1"
messages:
  - role: system
    content: "Production"
`), 0600))
	reg := New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent.production")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "Production", tpl.Messages[0].Content)
}

func TestFileRegistry_GetTemplate_EnvSpecificInvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(`
id: p
version: "1"
messages:
  - role: system
    content: "Base"
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.prod.yaml"), []byte("id: p\nmessages: [unclosed"), 0600))
	reg := New(dir)
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "p.prod")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestFileRegistry_GetTemplate_YmlExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "agent.yml")
	data := []byte(`
id: agent
version: "1"
messages:
  - role: system
    content: "From .yml file"
`)
	require.NoError(t, os.WriteFile(dest, data, 0600))
	reg := New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "agent", tpl.Metadata.ID)
	assert.Equal(t, "From .yml file", tpl.Messages[0].Content)
}

func TestFileRegistry_GetTemplate_CacheSafety(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "safe.yaml"), []byte(`
id: safe
version: "1"
messages:
  - role: system
    content: "Original"
tools:
  - name: only_tool
    description: "Only"
`), 0600))
	reg := New(dir)
	ctx := context.Background()
	tpl1, err := reg.GetTemplate(ctx, "safe")
	require.NoError(t, err)
	require.NotNil(t, tpl1)
	tpl1.Messages[0].Content = "Mutated"
	tpl1.Tools = append(tpl1.Tools, prompty.ToolDefinition{Name: "extra", Description: "Extra"})
	tpl2, err := reg.GetTemplate(ctx, "safe")
	require.NoError(t, err)
	require.NotNil(t, tpl2)
	assert.Equal(t, "Original", tpl2.Messages[0].Content, "cache must return unchanged template after caller mutated previous copy")
	assert.Len(t, tpl2.Tools, 1)
	assert.Equal(t, "only_tool", tpl2.Tools[0].Name)
}

func TestFileRegistry_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := New(dir)
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestFileRegistry_List(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("id: a\nversion: \"1\"\nmessages:\n  - role: system\n    content: x\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte("id: b\nversion: \"1\"\nmessages:\n  - role: system\n    content: y\n"), 0600))
	reg := New(dir)
	ctx := context.Background()
	ids, err := reg.List(ctx)
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "a")
	assert.Contains(t, ids, "b")
}

func TestFileRegistry_Stat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stat_test.yaml")
	require.NoError(t, os.WriteFile(path, []byte("id: stat_test\nversion: \"1\"\nmessages:\n  - role: system\n    content: z\n"), 0600))
	reg := New(dir)
	ctx := context.Background()
	info, err := reg.Stat(ctx, "stat_test")
	require.NoError(t, err)
	assert.Equal(t, "stat_test", info.ID)
	assert.False(t, info.UpdatedAt.IsZero())
}

func TestFileRegistry_Reload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(`
id: p
version: "1"
messages:
  - role: system
    content: "v1"
`), 0600))
	reg := New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, "v1", tpl.Messages[0].Content)
	reg.Reload()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(`
id: p
version: "1"
messages:
  - role: system
    content: "v2"
`), 0600))
	tpl2, err := reg.GetTemplate(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, "v2", tpl2.Messages[0].Content)
}

func TestFileRegistry_Concurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(`
id: p
version: "1"
messages:
  - role: system
    content: "x"
`), 0600))
	reg := New(dir)
	ctx := context.Background()
	type result struct {
		tpl *prompty.ChatPromptTemplate
		err error
	}
	done := make(chan result, 50)
	for range 50 {
		go func() {
			tpl, err := reg.GetTemplate(ctx, "p")
			done <- result{tpl: tpl, err: err}
		}()
	}
	for range 50 {
		r := <-done
		require.NoError(t, r.err)
		require.NotNil(t, r.tpl)
		assert.Equal(t, "p", r.tpl.Metadata.ID)
		assert.Equal(t, "x", r.tpl.Messages[0].Content)
	}
}

func TestFileRegistry_GetTemplate_WithPartials(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "partials"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "partials", "safety.tmpl"), []byte(`{{ define "safety" }}Never give medical diagnoses.{{ end }}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doctor.yaml"), []byte(`
id: doctor
version: "1"
messages:
  - role: system
    content: |
      You are a doctor assistant.
      {{ template "safety" }}
  - role: user
    content: "Hi"
`), 0600))
	reg := New(dir, WithPartials("partials/*.tmpl"))
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

func TestFileRegistry_ConcurrentReloadAndGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.yaml"), []byte(`
id: q
version: "1"
messages:
  - role: system
    content: "q"
`), 0600))
	reg := New(dir)
	ctx := context.Background()
	done := make(chan struct{})
	for range 30 {
		go func() {
			_, _ = reg.GetTemplate(ctx, "q")
			done <- struct{}{}
		}()
	}
	for range 20 {
		go func() {
			reg.Reload()
			done <- struct{}{}
		}()
	}
	for range 50 {
		<-done
	}
}
