package embedregistry

import (
	"context"
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"
)

//go:embed testdata/prompts/late_required_agent.yaml
var lateFixtures embed.FS

func TestLateBinding_RequiredLateWithLateInput(t *testing.T) {
	t.Parallel()
	reg, err := New(lateFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		UserQuery string `prompt:"user_query"`
	}{UserQuery: "hello"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "late_required_agent", input)
	require.NoError(t, err)

	plan, err = plan.WithLateInput(struct {
		PatientDossier string `prompt:"patient_dossier"`
	}{PatientDossier: "chart-42"})
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Contains(t, exec.Messages[0].Content[0].(prompty.TextPart).Text, "chart-42")
}
