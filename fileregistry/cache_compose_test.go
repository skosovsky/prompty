package fileregistry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"
	"github.com/skosovsky/prompty/remoteregistry"
)

func TestCachedRegistry_ConditionalCompose_PlanWithoutExplicitCaps(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	base, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)
	reg := remoteregistry.WithCache(base, time.Minute)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "first"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "composed_conditional_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)

	input2, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "second"})
	require.NoError(t, err)
	input2 = prompty.PlanInputWithCapabilities(input2, map[string]any{
		"capabilities": map[string]any{"workspace_enabled": false},
	})
	plan2, err := reg.Plan(ctx, "composed_conditional_main", input2)
	require.NoError(t, err)
	exec2, err := plan2.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec2.Messages, 2)
	assert.Equal(t, "second", exec2.Messages[1].Content[0].(prompty.TextPart).Text)
}
