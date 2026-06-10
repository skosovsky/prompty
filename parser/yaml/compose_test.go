package yaml

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/manifest"
)

func TestUnmarshal_ComposedManifestImportsAndLayers(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "manifest", "testdata", "composed_main.yaml"))
	require.NoError(t, err)

	var raw manifest.RawManifest
	require.NoError(t, New().Unmarshal(data, &raw))
	require.Len(t, raw.Imports, 1)
	assert.Equal(t, "composed_child", raw.Imports[0].ID)
	require.Len(t, raw.Layers, 3)
	assert.Equal(t, "composed_child", raw.Layers[1].ImportRef)
}
