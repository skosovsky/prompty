package embedregistry

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/parser/yaml"
)

func TestRegistry_ResolveManifest_MetadataOnly(t *testing.T) {
	t.Parallel()
	const manifestYAML = `
id: agent
version: "1"
messages:
  - role: system
    layer_id: policy
    content: "rules"
  - role: user
    content: "{{ .Input.q }}"
`
	fsys := fstest.MapFS{
		"prompts/agent.yaml": &fstest.MapFile{Data: []byte(manifestYAML)},
	}
	reg, err := New(fsys, "prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.ResolveManifest(context.Background(), "agent")
	require.NoError(t, err)
	assert.Equal(t, "agent", desc.Metadata.ID)
	assert.Equal(t, []string{"policy"}, desc.LayerIDs)
}
