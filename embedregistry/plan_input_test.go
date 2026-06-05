package embedregistry

import (
	"context"
	"embed"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/prompts/*.json
var planInputTestFS embed.FS

func TestEmbedregistry_PlanInputFrom_NilOptionals(t *testing.T) {
	t.Parallel()
	reg, err := New(planInputTestFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	input, err := prompty.PlanInputFrom(struct {
		Flag *bool     `json:"flag,omitempty" prompt:"flag"`
		Tags *[]string `json:"tags,omitempty" prompt:"tags"`
	}{})
	require.NoError(t, err)
	plan, err := reg.Plan(ctx, "nil_optionals", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "flag=false tags=0", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}
