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
		`{"id":"nil_optionals","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"flag={{if eq .Input.flag nil}}null{{else}}{{.Input.flag}}{{end}} tags={{if eq .Input.tags nil}}null{{else}}{{len .Input.tags}}{{end}}"}]}]}`,
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
	assert.Equal(t, "flag=null tags=null", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}
