package prompty_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/parser/yaml"
)

func TestDescribePrompt_CapabilitiesFromMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `
id: route_agent
metadata:
  tags: [routing]
  capabilities: [text-only, low-latency]
messages:
  - role: user
    content: hi
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "route_agent.yaml"), []byte(body), 0600))

	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.DescribePrompt(context.Background(), "route_agent")
	require.NoError(t, err)
	assert.Equal(t, []string{"routing"}, desc.Tags)
	assert.Equal(t, []string{"text-only", "low-latency"}, desc.Capabilities)
}
