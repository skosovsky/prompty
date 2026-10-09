//go:build e2e

package openai_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	provider "github.com/skosovsky/prompty/adapter/openai"
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

func TestE2EComposeCachePolicy_OpenAI(t *testing.T) {
	t.Parallel()
	exec := composedCachePolicyExecution(t)
	want := exec.Messages[0].CachePolicy
	_, err := provider.New().Translate(exec)
	require.NoError(t, err)
	assert.Equal(t, want, exec.Messages[0].CachePolicy)
}
