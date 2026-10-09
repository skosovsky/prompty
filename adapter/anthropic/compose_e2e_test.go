//go:build e2e

package anthropic_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	provider "github.com/skosovsky/prompty/adapter/anthropic"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/parser/yaml"
)

func composedCachePolicyExecution(t *testing.T) *prompty.PromptExecution {
	t.Helper()
	dir := filepath.Join("..", "..", "fileregistry", "testdata", "prompts")
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "cache-policy"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "composed_cache_policy", input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	require.NotNil(t, exec.Messages[0].CachePolicy)
	assert.Equal(t, "ephemeral", exec.Messages[0].CachePolicy.Type)
	return exec
}

func TestE2EComposeCachePolicy_Anthropic(t *testing.T) {
	t.Parallel()
	exec := composedCachePolicyExecution(t)
	params, err := provider.New().Translate(exec)
	require.NoError(t, err)
	require.NotEmpty(t, params.System)
	require.NotNil(t, params.System[0].CacheControl)
	assert.Equal(t, "ephemeral", string(params.System[0].CacheControl.Type))
}
