package manifest

import (
	"context"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest_PlanInputFrom_NilOptionals(t *testing.T) {
	t.Parallel()
	data := []byte(
		`{"id":"nil_optionals","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"flag={{ .Input.flag }} tags={{ len .Input.tags }}"}]}]}`,
	)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	input, err := prompty.PlanInputFrom(struct {
		Flag *bool     `json:"flag,omitempty" prompt:"flag"`
		Tags *[]string `json:"tags,omitempty" prompt:"tags"`
	}{})
	require.NoError(t, err)
	plan, err := prompty.NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "flag=false tags=0", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}
