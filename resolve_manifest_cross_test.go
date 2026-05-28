package prompty_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/embedregistry"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/parser/yaml"
	"github.com/skosovsky/prompty/remoteregistry"
)

const crossRegistryManifestYAML = `
id: cross_agent
version: "3"
description: Cross registry smoke
messages:
  - role: system
    layer_id: policy
    content: "rules"
  - role: user
    content: "{{ .Input.q }}"
`

type staticManifestFetcher struct {
	data map[string][]byte
}

func (f *staticManifestFetcher) Fetch(_ context.Context, id string) ([]byte, error) {
	if d, ok := f.data[id]; ok {
		return d, nil
	}
	return nil, remoteregistry.ErrNotFound
}

func TestCrossRegistry_ResolveManifest_IdenticalDescriptor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manifestBytes := []byte(crossRegistryManifestYAML)

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "cross_agent.yaml")
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0600))

	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/cross_agent.yaml": &fstest.MapFile{Data: manifestBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{"cross_agent": manifestBytes}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)
	cachedReg := remoteregistry.WithCache(remoteReg, time.Hour)

	want, err := fileReg.ResolveManifest(ctx, "cross_agent")
	require.NoError(t, err)

	resolvers := []struct {
		name string
		reg  prompty.ManifestResolver
	}{
		{"file", fileReg},
		{"embed", embedReg},
		{"remote", remoteReg},
		{"cached", cachedReg},
	}
	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, resolveErr := tc.reg.ResolveManifest(ctx, "cross_agent")
			require.NoError(t, resolveErr)
			assert.Equal(t, want.Metadata, got.Metadata)
			assert.Equal(t, want.LayerIDs, got.LayerIDs)
			assert.Equal(t, want.RequiredTools, got.RequiredTools)
		})
	}
}
