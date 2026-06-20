package remoteregistry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
)

type workspaceComposeContext struct {
	enabled bool
}

func (c workspaceComposeContext) ComposeValues() prompty.ComposeValues {
	return prompty.NewComposeValuesFromPairs(
		prompty.ComposeBool("capabilities.workspace_enabled", c.enabled),
	)
}

func readComposeFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "fileregistry", "testdata", "prompts", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func newRemoteComposeRegistry(t *testing.T) (*Registry, *mockFetcher) {
	t.Helper()
	m := &mockFetcher{data: map[string][]byte{
		"composed_main":             readComposeFixture(t, "composed_main.yaml"),
		"composed_child":            readComposeFixture(t, "composed_child.yaml"),
		"composed_conditional_main": readComposeFixture(t, "composed_conditional_main.yaml"),
	}}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	return reg, m
}

func TestRegistry_ComposedManifest_PlanExecute(t *testing.T) {
	t.Parallel()
	reg, m := newRemoteComposeRegistry(t)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "remote"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "composed_main", input)
	require.NoError(t, err)

	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.GreaterOrEqual(t, m.called, 1)
}

func TestRegistry_ConditionalCompose_PlanMissingComposeValuesRequiresContext(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "missing-context"})
	require.NoError(t, err)

	_, err = reg.Plan(ctx, "composed_conditional_main", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires compose values")
}

func TestRegistry_ConditionalCompose_ResolveManifestWithCapabilities(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	desc, err := reg.ResolveManifest(
		ctx,
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", false),
			),
		),
	)
	require.NoError(t, err)
	assert.NotContains(t, desc.LayerIDs, "child_rules")
}

func TestRegistry_ConditionalCompose_ResolveManifestRuntimeOn(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	desc, err := reg.ResolveManifest(
		ctx,
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", true),
			),
		),
	)
	require.NoError(t, err)
	assert.Contains(t, desc.LayerIDs, "child_rules")
}

func TestRegistry_ConditionalCompose_PlanMissingComposeValuesDiffersFromResolveManifest(
	t *testing.T,
) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	conservative, err := reg.ResolveManifest(ctx, "composed_conditional_main")
	require.NoError(t, err)
	assert.Contains(t, conservative.LayerIDs, "child_rules")

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "missing-context"})
	require.NoError(t, err)

	_, err = reg.Plan(ctx, "composed_conditional_main", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires compose values")
}

func TestRegistry_Checkpoint_VerifyTransitiveDetectsChildTamper(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	desc, err := reg.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)

	tamperReader := &remoteComposeTamperReader{base: reg, overrides: map[string][]byte{
		"composed_child": tamperedComposedChildManifestBytes(),
	}}
	err = tamperReader.VerifyManifestDescriptor(ctx, desc)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

type remoteComposeTamperReader struct {
	base      prompty.ManifestBytesReader
	overrides map[string][]byte
}

func (r *remoteComposeTamperReader) ReadManifestBytes(
	ctx context.Context,
	id string,
) ([]byte, error) {
	if b, ok := r.overrides[id]; ok {
		return b, nil
	}
	return r.base.ReadManifestBytes(ctx, id)
}

func (r *remoteComposeTamperReader) VerifyManifestDescriptor(
	ctx context.Context,
	desc prompty.ManifestDescriptor,
) error {
	return manifest.CheckpointVerify(ctx, desc, r, yaml.New())
}

func TestCachedRegistry_ConditionalCompose_MissingContextThenStrict(t *testing.T) {
	t.Parallel()
	base, _ := newRemoteComposeRegistry(t)
	reg := WithCache(base, time.Minute)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "first"})
	require.NoError(t, err)

	_, err = reg.Plan(ctx, "composed_conditional_main", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires compose values")

	input2, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "second"})
	require.NoError(t, err)
	input2 = prompty.PlanInputWithComposeContext(
		input2,
		workspaceComposeContext{enabled: false},
	)
	plan2, err := reg.Plan(ctx, "composed_conditional_main", input2)
	require.NoError(t, err)
	exec2, err := plan2.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec2.Messages, 2)
	assert.Equal(t, "second", exec2.Messages[1].Content[0].(prompty.TextPart).Text)
}

func TestRegistry_ComposedManifest_FetchCount(t *testing.T) {
	t.Parallel()
	reg, m := newRemoteComposeRegistry(t)
	ctx := context.Background()
	m.called = 0

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "fetch-count"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "composed_main", input)
	require.NoError(t, err)
	_, err = plan.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(
		t,
		3,
		m.called,
		"composed_main: main fetch plus child loads for cycle-check and resolve",
	)
}

func TestRegistry_ManifestUsesComposeE_ReflectsRemoteChange(t *testing.T) {
	t.Parallel()
	flatYAML := []byte(`id: composed_main
inputs:
  query:
    type: string
messages:
  - role: user
    content: "flat"`)
	composeMain := readComposeFixture(t, "composed_main.yaml")
	call := 0
	m := &mockFetcher{
		fetch: func(_ context.Context, id string) ([]byte, error) {
			if id != "composed_main" {
				return nil, ErrNotFound
			}
			call++
			if call == 1 {
				return flatYAML, nil
			}
			return composeMain, nil
		},
	}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	ctx := context.Background()

	uses1, err := reg.ManifestUsesComposeE(ctx, "composed_main")
	require.NoError(t, err)
	assert.False(t, uses1)

	uses2, err := reg.ManifestUsesComposeE(ctx, "composed_main")
	require.NoError(t, err)
	assert.True(t, uses2)
}

func TestCachedRegistry_FlatToComposeTransition_BypassesStaleCache(t *testing.T) {
	t.Parallel()
	flatYAML := []byte(`id: composed_main
inputs:
  query:
    type: string
messages:
  - role: user
    content: "flat"`)
	composeMain := readComposeFixture(t, "composed_main.yaml")
	composeChild := readComposeFixture(t, "composed_child.yaml")
	call := 0
	m := &mockFetcher{
		data: map[string][]byte{"composed_child": composeChild},
		fetch: func(_ context.Context, id string) ([]byte, error) {
			switch id {
			case "composed_main":
				call++
				if call <= 2 {
					return flatYAML, nil
				}
				return composeMain, nil
			case "composed_child":
				return composeChild, nil
			default:
				return nil, ErrNotFound
			}
		},
	}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	cached := WithCache(reg, time.Minute)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "first"})
	require.NoError(t, err)
	plan1, err := cached.Plan(ctx, "composed_main", input)
	require.NoError(t, err)
	exec1, err := plan1.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec1.Messages, 1)
	assert.Equal(t, "flat", exec1.Messages[0].Content[0].(prompty.TextPart).Text)

	input2, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "second"})
	require.NoError(t, err)
	plan2, err := cached.Plan(ctx, "composed_main", input2)
	require.NoError(t, err)
	exec2, err := plan2.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec2.Messages, 3)
	assert.Equal(t, "Base assistant.", exec2.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestRegistry_ManifestUsesComposeE_CorruptReturnsError(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{
		"bad": []byte("id: bad\nlayers: [unclosed"),
	}}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	_, err = reg.ManifestUsesComposeE(context.Background(), "bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestRegistry_RecommendManifestDescriptor_MissingImport(t *testing.T) {
	t.Parallel()
	mainBytes := readComposeFixture(t, "composed_main_missing_child.yaml")
	m := &mockFetcher{data: map[string][]byte{
		"composed_main_missing_child": mainBytes,
	}}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	_, err = reg.RecommendManifestDescriptor(context.Background(), "composed_main_missing_child")
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrTemplateNotFound)
	require.Contains(t, err.Error(), "read import")
}

func TestRegistry_RecommendManifestDescriptor_Corrupt(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{
		"bad": []byte("id: bad\nlayers: [unclosed"),
	}}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	_, err = reg.RecommendManifestDescriptor(context.Background(), "bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestRegistry_Plan_ComposePeekInvalid(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{
		"compose_bad": []byte("id: compose_bad\nimports:\n  - id: child\nlayers: [unclosed"),
	}}
	reg, err := New(m, WithParser(yaml.New()))
	require.NoError(t, err)
	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "x"})
	require.NoError(t, err)
	_, err = reg.Plan(context.Background(), "compose_bad", input)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestRegistry_RecordComposeFlag_KnownManifestUsesCompose(t *testing.T) {
	t.Parallel()
	reg, m := newRemoteComposeRegistry(t)
	ctx := context.Background()

	_, err := reg.ResolveManifest(ctx, "composed_main")
	require.NoError(t, err)
	known, ok := reg.KnownManifestUsesCompose("composed_main")
	require.True(t, ok)
	assert.True(t, known)

	m.called = 0
	cached := WithCache(reg, time.Minute)
	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "no-redundant-probe"})
	require.NoError(t, err)
	plan, err := cached.Plan(ctx, "composed_main", input)
	require.NoError(t, err)
	_, err = plan.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(
		t,
		3,
		m.called,
		"CachedRegistry.Plan must not redundant-probe after ResolveManifest",
	)
}

func TestRegistry_ComposedPlan_ContextCanceled(t *testing.T) {
	t.Parallel()
	mainBytes := readComposeFixture(t, "composed_main.yaml")
	childBytes := readComposeFixture(t, "composed_child.yaml")
	data := map[string][]byte{
		"composed_main":  mainBytes,
		"composed_child": childBytes,
	}
	m := &mockFetcher{
		data: data,
		fetch: func(ctx context.Context, id string) ([]byte, error) {
			if id == "composed_child" {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			if d, ok := data[id]; ok {
				return d, nil
			}
			return nil, ErrNotFound
		},
	}
	reg, err := New(m, WithParser(yaml.New()))
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

func TestRegistry_ComposedManifest_Provenance(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "prov"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	for _, msg := range exec.Messages {
		require.NotNil(t, msg.Provenance)
		assert.Equal(t, "composed_main", msg.Provenance.ManifestID)
	}
}

func TestRegistry_Checkpoint_VerifyDigest(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	raw, err := reg.ReadManifestBytes(ctx, "composed_main")
	require.NoError(t, err)

	desc, err := reg.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)
	require.NoError(t, reg.VerifyManifestDescriptor(ctx, desc))
	_ = raw
}

func TestCachedRegistry_ConditionalCompose_PlanExecute(t *testing.T) {
	t.Parallel()
	base, m := newRemoteComposeRegistry(t)
	reg := WithCache(base, time.Minute)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "off"})
	require.NoError(t, err)
	input = prompty.PlanInputWithComposeContext(
		input,
		workspaceComposeContext{enabled: false},
	)

	plan, err := reg.Plan(ctx, "composed_conditional_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)

	plan2, err := reg.Plan(ctx, "composed_conditional_main", input)
	require.NoError(t, err)
	exec2, err := plan2.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec2.Messages, 2)
	assert.GreaterOrEqual(t, m.called, 2, "composed manifests must not use stale template cache")
}

func TestRegistry_ConditionalCompose_ResolveManifestEmptyCapsStrict(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	desc, err := reg.ResolveManifest(
		ctx,
		"composed_conditional_main",
		prompty.WithResolveComposeValues(prompty.NewComposeValuesFromPairs()),
	)
	require.NoError(t, err)
	assert.Contains(t, desc.LayerIDs, "base_system")
	assert.NotContains(t, desc.LayerIDs, "child_rules")
}

func TestRegistry_ConditionalCompose_ResolveManifestInputSchemaWithCaps(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	conservative, err := reg.ResolveManifest(ctx, "composed_conditional_main")
	require.NoError(t, err)
	conservativeProps := remoteComposeSchemaProps(t, conservative.InputSchema)
	assert.Contains(t, conservativeProps, "clinic_name")

	runtimeOff, err := reg.ResolveManifest(
		ctx,
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", false),
			),
		),
	)
	require.NoError(t, err)
	runtimeProps := remoteComposeSchemaProps(t, runtimeOff.InputSchema)
	assert.Contains(t, runtimeProps, "query")
	assert.NotContains(t, runtimeProps, "clinic_name")
}

func remoteComposeSchemaProps(t *testing.T, schema *prompty.SchemaDefinition) map[string]bool {
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

func TestRegistry_ComposedManifest_ProvenanceLayerID(t *testing.T) {
	t.Parallel()
	reg, _ := newRemoteComposeRegistry(t)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "layer"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "base_system", exec.Messages[0].Provenance.LayerID)
	assert.Equal(t, "child_rules", exec.Messages[1].Provenance.LayerID)
}

func TestRegistry_ComposedManifest_WithEnvironment(t *testing.T) {
	t.Parallel()
	prodMain := readComposeFixture(t, "composed_main.prod.yaml")
	childProd := readComposeFixture(t, "composed_child.prod.yaml")
	m := &mockFetcher{data: map[string][]byte{
		"composed_main.prod":  prodMain,
		"composed_child.prod": childProd,
	}}
	reg, err := New(m, WithParser(yaml.New()), WithEnvironment("prod"))
	require.NoError(t, err)
	ctx := context.Background()

	input, err := prompty.PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "env"})
	require.NoError(t, err)

	plan, err := reg.Plan(ctx, "composed_main", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "Prod assistant.", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}

func tamperedComposedChildManifestBytes() []byte {
	return []byte(`{"id":"composed_child","inputs":{"clinic_name":{"type":"string"}},` +
		`"layers":[{"id":"child_rules","role":"system","content":"TAMPERED"}]}`)
}
