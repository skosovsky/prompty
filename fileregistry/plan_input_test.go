package fileregistry

import (
	"context"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileregistry_PlanInputFrom_NilOptionals(t *testing.T) {
	t.Parallel()
	reg, err := New("testdata/prompts", WithParser(yaml.New()))
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
