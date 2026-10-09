//go:build integration

package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"
	"github.com/skosovsky/prompty/remoteregistry"
)

func TestIntegrationFetcher_IntegrationWithRegistry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"integ.yaml": `
id: integ
version: "1"
messages:
  - role: system
    content: "Integrated"
`,
	})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	reg, err := remoteregistry.New(g, remoteregistry.WithParser(yaml.New()))
	require.NoError(t, err)
	ctx := context.Background()
	plan, err := reg.Plan(ctx, "integ", prompty.RegistryPlanInput{})
	require.NoError(t, err)
	tpl := plan.Template()
	require.NotNil(t, tpl)
	require.Equal(t, "integ", tpl.Metadata.ID)
}
