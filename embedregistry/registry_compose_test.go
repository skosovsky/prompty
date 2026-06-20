package embedregistry

import (
	"context"
	"embed"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
)

//go:embed testdata/prompts/composed_*.yaml
var composedFixtures embed.FS

type workspaceComposeContext struct {
	enabled bool
}

func (c workspaceComposeContext) ComposeValues() prompty.ComposeValues {
	return prompty.NewComposeValuesFromPairs(
		prompty.ComposeBool("capabilities.workspace_enabled", c.enabled),
	)
}

func TestRegistry_ManifestUsesComposeE_ComposedMain(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)
	uses, err := reg.ManifestUsesComposeE(context.Background(), "composed_main")
	require.NoError(t, err)
	assert.True(t, uses)
}

func TestRegistry_ComposedManifest_PlanExecute(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "composed_main", input)
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "Base assistant.", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestRegistry_Stat_ComposedManifest(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	info, err := reg.Stat(context.Background(), "composed_main")
	require.NoError(t, err)
	assert.Equal(t, "composed_main", info.ID)
}

func TestRegistry_ConditionalCompose_PlanMissingComposeValuesRequiresContext(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "missing-context"})
	require.NoError(t, err)

	_, err = reg.Plan(context.Background(), "composed_conditional_main", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires compose values")
}

func TestRegistry_ComposedManifest_ResolveManifestLayerIDs(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.ResolveManifest(context.Background(), "composed_main")
	require.NoError(t, err)
	assert.Contains(t, desc.LayerIDs, "base_system")
	assert.Contains(t, desc.LayerIDs, "child_rules")
}

func TestRegistry_ConditionalCompose_ResolveManifestWithCapabilities(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	conservative, err := reg.ResolveManifest(context.Background(), "composed_conditional_main")
	require.NoError(t, err)
	assert.Contains(t, conservative.LayerIDs, "child_rules")

	runtimeOff, err := reg.ResolveManifest(
		context.Background(),
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", false),
			),
		),
	)
	require.NoError(t, err)
	assert.Contains(t, runtimeOff.LayerIDs, "base_system")
	assert.NotContains(t, runtimeOff.LayerIDs, "child_rules")

	runtimeOn, err := reg.ResolveManifest(
		context.Background(),
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", true),
			),
		),
	)
	require.NoError(t, err)
	assert.Contains(t, runtimeOn.LayerIDs, "child_rules")
}

func TestRegistry_ConditionalCompose_PlanMissingComposeValuesDiffersFromResolveManifest(
	t *testing.T,
) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	conservative, err := reg.ResolveManifest(context.Background(), "composed_conditional_main")
	require.NoError(t, err)
	assert.Contains(t, conservative.LayerIDs, "child_rules")

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "missing-context"})
	require.NoError(t, err)

	_, err = reg.Plan(context.Background(), "composed_conditional_main", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires compose values")
}

func TestRegistry_Checkpoint_VerifyDigest(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.RecommendManifestDescriptor(context.Background(), "composed_main")
	require.NoError(t, err)
	require.NoError(t, reg.VerifyManifestDescriptor(context.Background(), desc))

	desc.Digest = "deadbeef"
	err = reg.VerifyManifestDescriptor(context.Background(), desc)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

func TestRegistry_Checkpoint_VerifyTransitiveDetectsChildTamper(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.RecommendManifestDescriptor(context.Background(), "composed_main")
	require.NoError(t, err)

	tamperReader := &embedComposeTamperReader{base: reg, overrides: map[string][]byte{
		"composed_child": tamperedComposedChildManifestBytes(),
	}}
	err = tamperReader.VerifyManifestDescriptor(context.Background(), desc)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

type embedComposeTamperReader struct {
	base      prompty.ManifestBytesReader
	overrides map[string][]byte
}

func (r *embedComposeTamperReader) ReadManifestBytes(
	ctx context.Context,
	id string,
) ([]byte, error) {
	if b, ok := r.overrides[id]; ok {
		return b, nil
	}
	return r.base.ReadManifestBytes(ctx, id)
}

func (r *embedComposeTamperReader) VerifyManifestDescriptor(
	ctx context.Context,
	desc prompty.ManifestDescriptor,
) error {
	return manifest.CheckpointVerify(ctx, desc, r, yaml.New())
}

func TestRegistry_ConditionalCompose_PlanExecute(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "x"})
	require.NoError(t, err)
	input = prompty.PlanInputWithComposeContext(
		input,
		workspaceComposeContext{enabled: false},
	)

	plan, err := reg.Plan(context.Background(), "composed_conditional_main", input)
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "x", exec.Messages[1].Content[0].(prompty.TextPart).Text)
}

func TestRegistry_ComposedManifest_ReadManifestBytesAndPlan(t *testing.T) {
	t.Parallel()
	root := filepath.Join("testdata", "prompts")
	reg, err := New(composedFixtures, root, WithParser(yaml.New()))
	require.NoError(t, err)

	raw, err := reg.ReadManifestBytes(context.Background(), "composed_main")
	require.NoError(t, err)

	desc, err := reg.RecommendManifestDescriptor(context.Background(), "composed_main")
	require.NoError(t, err)
	assert.Equal(t, "composed_main", desc.ID)
	_ = raw

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "checkpoint"})
	require.NoError(t, err)
	plan, err := reg.Plan(context.Background(), desc.ID, input)
	require.NoError(t, err)
	_, err = plan.Execute(context.Background())
	require.NoError(t, err)
}

func TestRegistry_ConditionalCompose_ResolveManifestEmptyCapsStrict(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	desc, err := reg.ResolveManifest(
		context.Background(),
		"composed_conditional_main",
		prompty.WithResolveComposeValues(prompty.NewComposeValuesFromPairs()),
	)
	require.NoError(t, err)
	assert.Contains(t, desc.LayerIDs, "base_system")
	assert.NotContains(t, desc.LayerIDs, "child_rules")
}

func TestRegistry_ConditionalCompose_ResolveManifestInputSchemaWithCaps(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	conservative, err := reg.ResolveManifest(context.Background(), "composed_conditional_main")
	require.NoError(t, err)
	conservativeProps := composeSchemaPropertyNames(t, conservative.InputSchema)
	assert.Contains(t, conservativeProps, "clinic_name")

	runtimeOff, err := reg.ResolveManifest(
		context.Background(),
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", false),
			),
		),
	)
	require.NoError(t, err)
	runtimeProps := composeSchemaPropertyNames(t, runtimeOff.InputSchema)
	assert.Contains(t, runtimeProps, "query")
	assert.NotContains(t, runtimeProps, "clinic_name")
}

func composeSchemaPropertyNames(t *testing.T, schema *prompty.SchemaDefinition) map[string]bool {
	t.Helper()
	if schema == nil {
		return nil
	}
	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	require.NoError(t, err)
	props, _ := doc["properties"].(map[string]any)
	out := make(map[string]bool, len(props))
	for name := range props {
		out[name] = true
	}
	return out
}

func TestRegistry_ComposedManifest_Provenance(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "prov"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	for _, msg := range exec.Messages {
		require.NotNil(t, msg.Provenance)
		assert.Equal(t, "composed_main", msg.Provenance.ManifestID)
	}
	assert.Equal(t, "base_system", exec.Messages[0].Provenance.LayerID)
	assert.Equal(t, "child_rules", exec.Messages[1].Provenance.LayerID)
}

func TestRegistry_ComposedManifest_WithEnvironment(t *testing.T) {
	t.Parallel()
	prodMain, err := composedFixtures.ReadFile("testdata/prompts/composed_main.yaml")
	require.NoError(t, err)
	prodMain = []byte(strings.Replace(string(prodMain), "Base assistant.", "Prod assistant.", 1))
	child, err := composedFixtures.ReadFile("testdata/prompts/composed_child.yaml")
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/composed_main.prod.yaml":  &fstest.MapFile{Data: prodMain},
		"prompts/composed_child.prod.yaml": &fstest.MapFile{Data: child},
	}
	reg, err := New(embedFS, "prompts", WithParser(yaml.New()), WithEnvironment("prod"))
	require.NoError(t, err)

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "env"})
	require.NoError(t, err)

	plan, err := reg.Plan(context.Background(), "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "Prod assistant.", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestRegistry_ComposedRead_ContextCanceled(t *testing.T) {
	t.Parallel()
	reg, err := New(composedFixtures, "testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "cancel"})
	require.NoError(t, err)
	_, err = reg.Plan(ctx, "composed_main", input)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func tamperedComposedChildManifestBytes() []byte {
	return []byte(`{"id":"composed_child","inputs":{"clinic_name":{"type":"string"}},` +
		`"layers":[{"id":"child_rules","role":"system","content":"TAMPERED"}]}`)
}
