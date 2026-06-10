package compiled_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/internal/compiled"
	"github.com/skosovsky/prompty/parser/yaml"
)

func TestFromRenderPlanRegistry_ComposeDigestMatchesCheckpoint(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "fileregistry", "testdata", "prompts")
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	ctx := context.Background()
	desc, err := reg.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "digest"})
	require.NoError(t, err)
	plan, err := reg.Plan(ctx, "composed_main", input)
	require.NoError(t, err)

	cp, err := compiled.FromRenderPlanRegistry(ctx, plan, reg, "composed_main")
	require.NoError(t, err)
	assert.Equal(t, desc.Digest, cp.ManifestDigest())
	mainBytes := mustReadManifest(t, dir, "composed_main.yaml")
	assert.NotEqual(t, prompty.ManifestDigestSHA256(mainBytes), cp.ManifestDigest())
}

type bytesOnlyReader struct {
	bytes map[string][]byte
}

func (r *bytesOnlyReader) ReadManifestBytes(_ context.Context, id string) ([]byte, error) {
	b, ok := r.bytes[id]
	if !ok {
		return nil, prompty.ErrTemplateNotFound
	}
	return b, nil
}

func TestFromRenderPlanRegistry_ComposeRequiresCheckpointRegistry(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "fileregistry", "testdata", "prompts")
	mainBytes := mustReadManifest(t, dir, "composed_main.yaml")
	reg := &bytesOnlyReader{bytes: map[string][]byte{"composed_main": mainBytes}}

	ctx := context.Background()
	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "x"})
	require.NoError(t, err)
	tpl, err := prompty.NewChatPromptTemplate([]prompty.MessageTemplate{
		{Role: prompty.RoleUser, Content: prompty.TextContent("{{ .Input.query }}")},
	})
	require.NoError(t, err)
	plan, err := prompty.NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)

	_, err = compiled.FromRenderPlanRegistry(ctx, plan, reg, "composed_main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ManifestCheckpointRegistry")
}

func mustReadManifest(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return data
}
