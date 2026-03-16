package fileregistry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"

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
	dest := filepath.Join(dir, "support_agent.json")
	data := []byte(`{"id":"support_agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Hello {{ .user_name }}"}]}]}`)
	require.NoError(t, os.WriteFile(dest, data, 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "support_agent", tpl.Metadata.ID)
}

func TestFileRegistry_GetTemplate_ById(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	basePath := filepath.Join(dir, "support_agent.json")
	require.NoError(t, os.WriteFile(basePath, []byte(`{"id":"support_agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Base {{ .user_name }}"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "text", tpl.Messages[0].Content[0].Type)
	assert.Contains(t, tpl.Messages[0].Content[0].Text, "Base")
}

func TestFileRegistry_GetTemplate_IdWithDot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "support_agent.json"), []byte(`{"id":"support_agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Base"}]}]}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "support_agent.production.json"), []byte(`{"id":"support_agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Production"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent.production")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "Production", tpl.Messages[0].Content[0].Text)
}

func TestFileRegistry_GetTemplate_EnvSpecificInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(`{"id":"p","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Base"}]}]}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.prod.json"), []byte(`{"id":"p","messages":[unclosed`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "p.prod")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestFileRegistry_GetTemplate_JsonExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "agent.json")
	data := []byte(`{"id":"agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"From .json file"}]}]}`)
	require.NoError(t, os.WriteFile(dest, data, 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "agent", tpl.Metadata.ID)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "From .json file", tpl.Messages[0].Content[0].Text)
}

func TestFileRegistry_GetTemplate_CacheSafety(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "safe.json"), []byte(`{"id":"safe","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Original"}]}],"tools":[{"name":"only_tool","description":"Only","parameters":{}}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl1, err := reg.GetTemplate(ctx, "safe")
	require.NoError(t, err)
	require.NotNil(t, tpl1)
	tpl1.Messages[0].Content = []prompty.TemplatePart{{Type: "text", Text: "Mutated"}}
	tpl1.Tools = append(tpl1.Tools, prompty.ToolDefinition{Name: "extra", Description: "Extra"})
	tpl2, err := reg.GetTemplate(ctx, "safe")
	require.NoError(t, err)
	require.NotNil(t, tpl2)
	require.Len(t, tpl2.Messages[0].Content, 1)
	assert.Equal(t, "Original", tpl2.Messages[0].Content[0].Text, "cache must return unchanged template after caller mutated previous copy")
	assert.Len(t, tpl2.Tools, 1)
	assert.Equal(t, "only_tool", tpl2.Tools[0].Name)
}

func TestFileRegistry_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestFileRegistry_List(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"id":"a","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"id":"b","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"y"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
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
	path := filepath.Join(dir, "stat_test.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"id":"stat_test","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"z"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	info, err := reg.Stat(ctx, "stat_test")
	require.NoError(t, err)
	assert.Equal(t, "stat_test", info.ID)
	assert.False(t, info.UpdatedAt.IsZero())
}

func TestFileRegistry_Reload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(`{"id":"p","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"v1"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "p")
	require.NoError(t, err)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "v1", tpl.Messages[0].Content[0].Text)
	reg.Reload()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(`{"id":"p","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"v2"}]}]}`), 0600))
	tpl2, err := reg.GetTemplate(ctx, "p")
	require.NoError(t, err)
	require.Len(t, tpl2.Messages[0].Content, 1)
	assert.Equal(t, "v2", tpl2.Messages[0].Content[0].Text)
}

func TestFileRegistry_Concurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.json"), []byte(`{"id":"p","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
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
		require.Len(t, r.tpl.Messages[0].Content, 1)
		assert.Equal(t, "x", r.tpl.Messages[0].Content[0].Text)
	}
}

func TestFileRegistry_GetTemplate_WithPartials(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "partials"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "partials", "safety.tmpl"), []byte(`{{ define "safety" }}Never give medical diagnoses.{{ end }}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doctor.json"), []byte(`{"id":"doctor","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"You are a doctor assistant.\n{{ template \"safety\" }}"}]},{"role":"user","content":[{"type":"text","text":"Hi"}]}]}`), 0600))
	reg, err := New(dir, WithPartials("partials/*.tmpl"), WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.json"), []byte(`{"id":"q","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"q"}]}]}`), 0600))
	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
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
