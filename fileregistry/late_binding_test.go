package fileregistry

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"
)

func TestLateBinding_RequiredLateWithLateInput(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
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

func TestLateBinding_RequiredLateEmptyWithLateInputFails(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		UserQuery string `prompt:"user_query"`
	}{UserQuery: "hello"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "late_required_agent", input)
	require.NoError(t, err)

	plan, err = plan.WithLateInput(struct {
		PatientDossier string `prompt:"patient_dossier"`
	}{PatientDossier: ""})
	require.NoError(t, err)

	_, err = plan.Execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrMissingVariable)
}

func TestLateBinding_PlanInputFromRejectsLateField(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		UserQuery      string `prompt:"user_query"`
		PatientDossier string `prompt:"patient_dossier"`
	}{UserQuery: "hello", PatientDossier: "nope"})
	require.NoError(t, err)

	_, err = reg.Plan(context.Background(), "late_required_agent", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "late-bound")
}

func TestLateBinding_RequiredLate_ExecuteWithoutWithLateInputFails(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		UserQuery string `prompt:"user_query"`
	}{UserQuery: "hello"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "late_required_agent", input)
	require.NoError(t, err)

	_, err = plan.Execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrMissingVariable)
}

func TestLateBinding_OptionalLate_ExecuteWithoutLateInput(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		UserQuery string `prompt:"user_query"`
	}{UserQuery: "hello"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "late_binding_agent", input)
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Contains(t, exec.Messages[0].Content[0].(prompty.TextPart).Text, "hello")
}

func TestRegistry_MissingEarlyRequiredFails(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "prompts")
	reg, err := New(dir, WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		UserQuery string `prompt:"user_query"`
	}{})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "late_required_agent", input)
	require.NoError(t, err)

	_, err = plan.Execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrMissingVariable)
}
