package prompty_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/parser/yaml"
)

func TestExamples_Task33Compose_PlanExecute(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("examples", "task33_compose")
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query      string `prompt:"query"`
		ClinicName string `prompt:"clinic_name"`
	}{Query: "hello", ClinicName: "demo"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "Base layer.", exec.Messages[0].Content[0].(prompty.TextPart).Text)
	assert.Contains(t, exec.Messages[1].Content[0].(prompty.TextPart).Text, "Child workspace rules")
	assert.Contains(t, exec.Messages[1].Content[0].(prompty.TextPart).Text, "demo")
	assert.Equal(t, "hello", exec.Messages[2].Content[0].(prompty.TextPart).Text)
}

func TestExamples_Task33Compose_Provenance(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("examples", "task33_compose")
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query      string `prompt:"query"`
		ClinicName string `prompt:"clinic_name"`
	}{Query: "hi", ClinicName: "clinic"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)

	assert.Equal(t, "composed_main", exec.Messages[0].Provenance.ManifestID)
	assert.Equal(t, "base_system", exec.Messages[0].Provenance.LayerID)
	assert.Equal(t, "child_rules", exec.Messages[1].Provenance.LayerID)
	assert.Equal(t, "user_turn", exec.Messages[2].Provenance.LayerID)
}

func TestExamples_Task33Compose_Checkpoint(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("examples", "task33_compose")
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	ctx := context.Background()
	desc, err := reg.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)
	assert.Equal(t, "composed_main", desc.ID)
	assert.NotEmpty(t, desc.Digest)
	require.NoError(t, reg.VerifyManifestDescriptor(ctx, desc))
}
