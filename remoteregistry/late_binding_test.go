package remoteregistry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"
)

func newRemoteLateRegistry(t *testing.T) *Registry {
	t.Helper()
	m := &mockFetcher{data: map[string][]byte{
		"late_required_agent": readComposeFixture(t, "late_required_agent.yaml"),
	}}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	return reg
}

func TestLateBinding_RequiredLateWithLateInput(t *testing.T) {
	t.Parallel()
	reg := newRemoteLateRegistry(t)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		UserQuery string `prompt:"user_query"`
	}{UserQuery: "hello"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "late_required_agent", input)
	require.NoError(t, err)

	plan, err = plan.WithLateInput(struct {
		PatientDossier string `prompt:"patient_dossier"`
	}{PatientDossier: "chart-42"})
	require.NoError(t, err)

	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Contains(t, exec.Messages[0].Content[0].(prompty.TextPart).Text, "chart-42")
}
