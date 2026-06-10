package fileregistry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
)

func TestRegistry_ResolveManifest_LightweightNoTemplateCache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifestPath := dir + "/agent.yaml"
	require.NoError(t, writeFile(manifestPath, `{
  "id": "agent",
  "messages": [
    {"role": "system", "layer_id": "policy", "content": [{"type": "text", "text": "Hi"}]},
    {"role": "user", "content": [{"type": "text", "text": "{{ .Input.q }}"}]}
  ]
}`))

	reg, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)

	desc, err := reg.ResolveManifest(context.Background(), "agent")
	require.NoError(t, err)
	assert.Equal(t, "agent", desc.Metadata.ID)
	assert.Equal(t, []string{"policy"}, desc.LayerIDs)

	reg.mu.RLock()
	_, cached := reg.cache["agent"]
	reg.mu.RUnlock()
	assert.False(t, cached, "ResolveManifest must not compile or cache ChatPromptTemplate")

	input, err := prompty.PlanInputFrom(struct {
		Q string `prompt:"q"`
	}{Q: "x"})
	require.NoError(t, err)
	_, err = reg.Plan(context.Background(), "agent", input)
	require.NoError(t, err)
	reg.mu.RLock()
	_, cached = reg.cache["agent"]
	reg.mu.RUnlock()
	assert.True(t, cached, "Plan should populate template cache")
}

func TestRegistry_ResolveManifest_MatchesPlanMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(dir+"/support.yaml", `
id: support
version: "2"
description: Support bot
messages:
  - role: system
    content: "You are helpful"
  - role: user
    content: "{{ .Input.q }}"
`))

	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.ResolveManifest(context.Background(), "support")
	require.NoError(t, err)
	assert.Equal(t, "2", desc.Metadata.Version)
	assert.Equal(t, "Support bot", desc.Metadata.Description)

	planInput, err := prompty.PlanInputFrom(struct {
		Q string `prompt:"q"`
	}{Q: "hi"})
	require.NoError(t, err)
	plan, err := reg.Plan(context.Background(), "support", planInput)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, desc.Metadata.ID, exec.Metadata.ID)
}

func TestRegistry_RecommendManifestDescriptor_MissingImport(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)
	_, err = reg.RecommendManifestDescriptor(context.Background(), "composed_main_missing_child")
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrTemplateNotFound)
	require.Contains(t, err.Error(), "read import")
}

func TestRegistry_RecommendManifestDescriptor_Corrupt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "bad.yaml"),
		[]byte("id: bad\nlayers: [unclosed"),
		0600,
	))
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)
	_, err = reg.RecommendManifestDescriptor(context.Background(), "bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestRegistry_Checkpoint_FlatManifest(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	ctx := context.Background()
	desc, err := reg.RecommendManifestDescriptor(ctx, "support_agent")
	require.NoError(t, err)
	assert.Equal(t, "support_agent", desc.ID)
	assert.NotEmpty(t, desc.Digest)
	require.NoError(t, reg.VerifyManifestDescriptor(ctx, desc))
}

func TestRegistry_Checkpoint_FlatManifest_TamperFails(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	ctx := context.Background()
	desc, err := reg.RecommendManifestDescriptor(ctx, "support_agent")
	require.NoError(t, err)

	tamperReader := &flatTamperReader{
		base: reg,
		overrides: map[string][]byte{
			"support_agent": []byte(`{"id":"support_agent","messages":[{"role":"system","content":"TAMPERED"}]}`),
		},
	}
	err = tamperReader.VerifyManifestDescriptor(ctx, desc)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

type flatTamperReader struct {
	base      prompty.ManifestBytesReader
	overrides map[string][]byte
}

func (r *flatTamperReader) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if b, ok := r.overrides[id]; ok {
		return b, nil
	}
	return r.base.ReadManifestBytes(ctx, id)
}

func (r *flatTamperReader) VerifyManifestDescriptor(ctx context.Context, desc prompty.ManifestDescriptor) error {
	return manifest.CheckpointVerify(ctx, desc, r, yaml.New())
}

var _ prompty.ManifestResolver = (*Registry)(nil)

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0600)
}
