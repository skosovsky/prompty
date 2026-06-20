package prompty_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/embedregistry"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/internal/compiled"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
	"github.com/skosovsky/prompty/remoteregistry"
)

type crossWorkspaceComposeContext struct {
	enabled bool
}

func (c crossWorkspaceComposeContext) ComposeValues() prompty.ComposeValues {
	return prompty.NewComposeValuesFromPairs(
		prompty.ComposeBool("capabilities.workspace_enabled", c.enabled),
	)
}

const crossRegistryManifestYAML = `
id: cross_agent
version: "3"
description: Cross registry smoke
messages:
  - role: system
    layer_id: policy
    content: "rules"
  - role: user
    content: "{{ .Input.q }}"
`

type staticManifestFetcher struct {
	data map[string][]byte
}

func (f *staticManifestFetcher) Fetch(_ context.Context, id string) ([]byte, error) {
	if d, ok := f.data[id]; ok {
		return d, nil
	}
	return nil, remoteregistry.ErrNotFound
}

func TestCrossRegistry_ResolveManifest_IdenticalDescriptor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manifestBytes := []byte(crossRegistryManifestYAML)

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "cross_agent.yaml")
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0600))

	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/cross_agent.yaml": &fstest.MapFile{Data: manifestBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{"cross_agent": manifestBytes}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)
	cachedReg := remoteregistry.WithCache(remoteReg, time.Hour)

	want, err := fileReg.ResolveManifest(ctx, "cross_agent")
	require.NoError(t, err)

	resolvers := []struct {
		name string
		reg  prompty.ManifestResolver
	}{
		{"file", fileReg},
		{"embed", embedReg},
		{"remote", remoteReg},
		{"cached", cachedReg},
	}
	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, resolveErr := tc.reg.ResolveManifest(ctx, "cross_agent")
			require.NoError(t, resolveErr)
			assert.Equal(t, want.Metadata, got.Metadata)
			assert.Equal(t, want.LayerIDs, got.LayerIDs)
			assert.Equal(t, want.RequiredTools, got.RequiredTools)
		})
	}
}

func readFileregistryComposeFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("fileregistry", "testdata", "prompts", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return data
}

type crossComposeRegistries struct {
	file   *fileregistry.Registry
	embed  *embedregistry.Registry
	remote *remoteregistry.Registry
}

func newCrossComposeRegistries(t *testing.T) crossComposeRegistries {
	t.Helper()
	ctx := context.Background()
	_ = ctx

	mainBytes := readFileregistryComposeFixture(t, "composed_conditional_main.yaml")
	childBytes := readFileregistryComposeFixture(t, "composed_child.yaml")
	composedMainBytes := readFileregistryComposeFixture(t, "composed_main.yaml")
	cachePolicyBytes := readFileregistryComposeFixture(t, "composed_cache_policy.yaml")

	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"composed_conditional_main.yaml": mainBytes,
		"composed_child.yaml":            childBytes,
		"composed_main.yaml":             composedMainBytes,
		"composed_cache_policy.yaml":     cachePolicyBytes,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600))
	}
	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/composed_conditional_main.yaml": &fstest.MapFile{Data: mainBytes},
		"prompts/composed_child.yaml":            &fstest.MapFile{Data: childBytes},
		"prompts/composed_main.yaml":             &fstest.MapFile{Data: composedMainBytes},
		"prompts/composed_cache_policy.yaml":     &fstest.MapFile{Data: cachePolicyBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{
			"composed_conditional_main": mainBytes,
			"composed_child":            childBytes,
			"composed_main":             composedMainBytes,
			"composed_cache_policy":     cachePolicyBytes,
		}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)

	return crossComposeRegistries{file: fileReg, embed: embedReg, remote: remoteReg}
}

func TestCrossRegistry_ConditionalCompose_ResolveManifestParity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossComposeRegistries(t)

	wantConservative, err := regs.file.ResolveManifest(ctx, "composed_conditional_main")
	require.NoError(t, err)
	assert.Contains(t, wantConservative.LayerIDs, "child_rules")

	wantRuntimeOff, err := regs.file.ResolveManifest(
		ctx,
		"composed_conditional_main",
		prompty.WithResolveComposeValues(
			prompty.NewComposeValuesFromPairs(
				prompty.ComposeBool("capabilities.workspace_enabled", false),
			),
		),
	)
	require.NoError(t, err)
	assert.NotContains(t, wantRuntimeOff.LayerIDs, "child_rules")

	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	resolvers := []struct {
		name string
		reg  prompty.ManifestResolver
	}{
		{"file", regs.file},
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	}
	for _, tc := range resolvers {
		t.Run(tc.name+"/conservative", func(t *testing.T) {
			t.Parallel()
			got, resolveErr := tc.reg.ResolveManifest(ctx, "composed_conditional_main")
			require.NoError(t, resolveErr)
			assert.Equal(t, wantConservative.LayerIDs, got.LayerIDs)
		})
		t.Run(tc.name+"/runtime_off", func(t *testing.T) {
			t.Parallel()
			got, resolveErr := tc.reg.ResolveManifest(
				ctx,
				"composed_conditional_main",
				prompty.WithResolveComposeValues(
					prompty.NewComposeValuesFromPairs(
						prompty.ComposeBool("capabilities.workspace_enabled", false),
					),
				),
			)
			require.NoError(t, resolveErr)
			assert.Equal(t, wantRuntimeOff.LayerIDs, got.LayerIDs)
		})
	}
}

func TestCrossRegistry_ComposeCheckpointDigestParity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossComposeRegistries(t)
	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	fileDesc, err := regs.file.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)

	embedDesc, err := regs.embed.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)

	remoteDesc, err := regs.remote.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)

	cachedDesc, err := cachedRemote.RecommendManifestDescriptor(ctx, "composed_main")
	require.NoError(t, err)

	assert.Equal(t, fileDesc.Digest, embedDesc.Digest)
	assert.Equal(t, fileDesc.Digest, remoteDesc.Digest)
	assert.Equal(t, fileDesc.Digest, cachedDesc.Digest)
}

type crossComposeRegistry struct {
	name     string
	plan     prompty.Registry
	resolver prompty.ManifestResolver
	reader   prompty.ManifestBytesReader
}

func crossComposeRegistryMatrix(t *testing.T) []crossComposeRegistry {
	t.Helper()
	regs := newCrossComposeRegistries(t)
	cached := remoteregistry.WithCache(regs.remote, time.Hour)
	return []crossComposeRegistry{
		{name: "file", plan: regs.file, resolver: regs.file, reader: regs.file},
		{name: "embed", plan: regs.embed, resolver: regs.embed, reader: regs.embed},
		{name: "remote", plan: regs.remote, resolver: regs.remote, reader: regs.remote},
		{name: "cached_remote", plan: cached, resolver: cached, reader: cached},
	}
}

func crossSchemaPropertyNames(t *testing.T, schema *prompty.SchemaDefinition) map[string]bool {
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

type crossComposeTamperReader struct {
	base      prompty.ManifestBytesReader
	overrides map[string][]byte
}

func (r *crossComposeTamperReader) ReadManifestBytes(
	ctx context.Context,
	id string,
) ([]byte, error) {
	if b, ok := r.overrides[id]; ok {
		return b, nil
	}
	return r.base.ReadManifestBytes(ctx, id)
}

func TestCrossRegistry_ConditionalCompose_Matrix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	parser := yaml.New()
	manifestID := "composed_conditional_main"

	for _, tc := range crossComposeRegistryMatrix(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("runtime_on_layer_ids", func(t *testing.T) {
				t.Parallel()
				desc, err := tc.resolver.ResolveManifest(
					ctx,
					manifestID,
					prompty.WithResolveComposeValues(
						prompty.NewComposeValuesFromPairs(
							prompty.ComposeBool("capabilities.workspace_enabled", true),
						),
					),
				)
				require.NoError(t, err)
				assert.Contains(t, desc.LayerIDs, "child_rules")
			})

			t.Run("empty_caps_strict", func(t *testing.T) {
				t.Parallel()
				desc, err := tc.resolver.ResolveManifest(
					ctx,
					manifestID,
					prompty.WithResolveComposeValues(prompty.NewComposeValuesFromPairs()),
				)
				require.NoError(t, err)
				assert.Contains(t, desc.LayerIDs, "base_system")
				assert.NotContains(t, desc.LayerIDs, "child_rules")
			})

			t.Run("input_schema_conservative_vs_runtime_off", func(t *testing.T) {
				t.Parallel()
				conservative, err := tc.resolver.ResolveManifest(ctx, manifestID)
				require.NoError(t, err)
				conservativeProps := crossSchemaPropertyNames(t, conservative.InputSchema)
				assert.Contains(t, conservativeProps, "clinic_name")

				runtimeOff, err := tc.resolver.ResolveManifest(
					ctx,
					manifestID,
					prompty.WithResolveComposeValues(
						prompty.NewComposeValuesFromPairs(
							prompty.ComposeBool("capabilities.workspace_enabled", false),
						),
					),
				)
				require.NoError(t, err)
				runtimeProps := crossSchemaPropertyNames(t, runtimeOff.InputSchema)
				assert.Contains(t, runtimeProps, "query")
				assert.NotContains(t, runtimeProps, "clinic_name")
			})

			t.Run("describe_prompt_matches_resolve_conservative", func(t *testing.T) {
				t.Parallel()
				describer, ok := tc.plan.(prompty.PromptDescriber)
				require.True(t, ok, "registry must implement PromptDescriber")
				described, err := describer.DescribePrompt(ctx, manifestID)
				require.NoError(t, err)
				resolved, err := tc.resolver.ResolveManifest(ctx, manifestID)
				require.NoError(t, err)
				assert.Equal(t, resolved.LayerIDs, described.LayerIDs)
				assert.Equal(
					t,
					crossSchemaPropertyNames(t, resolved.InputSchema),
					crossSchemaPropertyNames(t, described.InputSchema),
				)
			})

			t.Run("plan_missing_context_errors_and_runtime_off_executes", func(t *testing.T) {
				t.Parallel()
				missingContextInput, err := prompty.PlanInputFrom(struct {
					Query string `prompt:"query"`
				}{Query: "missing-context-" + tc.name})
				require.NoError(t, err)
				_, err = tc.plan.Plan(ctx, manifestID, missingContextInput)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "requires compose values")

				runtimeOffInput, err := prompty.PlanInputFrom(struct {
					Query string `prompt:"query"`
				}{Query: "runtime-off-" + tc.name})
				require.NoError(t, err)
				runtimeOffInput = prompty.PlanInputWithComposeContext(
					runtimeOffInput,
					crossWorkspaceComposeContext{enabled: false},
				)
				runtimeOffPlan, err := tc.plan.Plan(ctx, manifestID, runtimeOffInput)
				require.NoError(t, err)
				runtimeOffExec, err := runtimeOffPlan.Execute(ctx)
				require.NoError(t, err)
				require.Len(t, runtimeOffExec.Messages, 2)
			})

			t.Run("plan_provenance_parity", func(t *testing.T) {
				t.Parallel()
				input, err := prompty.PlanInputFrom(struct {
					Query string `prompt:"query"`
				}{Query: "prov-" + tc.name})
				require.NoError(t, err)
				plan, err := tc.plan.Plan(ctx, "composed_main", input)
				require.NoError(t, err)
				exec, err := plan.Execute(ctx)
				require.NoError(t, err)
				require.Len(t, exec.Messages, 3)
				require.NotNil(t, exec.Messages[0].Provenance, tc.name)
				assert.Equal(t, "composed_main", exec.Messages[0].Provenance.ManifestID)
				assert.Equal(t, "base_system", exec.Messages[0].Provenance.LayerID)
				require.NotNil(t, exec.Messages[1].Provenance, tc.name)
				assert.Equal(t, "child_rules", exec.Messages[1].Provenance.LayerID)
				require.NotNil(t, exec.Messages[2].Provenance, tc.name)
				assert.Equal(t, "user_turn", exec.Messages[2].Provenance.LayerID)
			})

			t.Run("composed_plan_context_canceled", func(t *testing.T) {
				t.Parallel()
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel()
				input, err := prompty.PlanInputFrom(struct {
					Query string `prompt:"query"`
				}{Query: "cancel-" + tc.name})
				require.NoError(t, err)
				_, err = tc.plan.Plan(canceledCtx, "composed_main", input)
				require.Error(t, err)
				assert.ErrorIs(t, err, context.Canceled)
			})

			t.Run("verify_manifest_descriptor_child_tamper_fails", func(t *testing.T) {
				t.Parallel()
				checkpoint, ok := tc.reader.(prompty.ManifestCheckpointRegistry)
				require.True(t, ok, "registry must implement ManifestCheckpointRegistry")
				desc, err := checkpoint.RecommendManifestDescriptor(ctx, "composed_main")
				require.NoError(t, err)
				require.NoError(t, checkpoint.VerifyManifestDescriptor(ctx, desc))

				tamperReader := &crossComposeTamperReader{
					base: tc.reader,
					overrides: map[string][]byte{
						"composed_child": tamperedComposedChildManifestBytes(),
					},
				}
				err = manifest.CheckpointVerify(ctx, desc, tamperReader, parser)
				require.Error(t, err)
				assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
			})

			t.Run("plan_cache_policy_ephemeral", func(t *testing.T) {
				t.Parallel()
				input, err := prompty.PlanInputFrom(struct {
					Query string `prompt:"query"`
				}{Query: "cache-" + tc.name})
				require.NoError(t, err)
				plan, err := tc.plan.Plan(ctx, "composed_cache_policy", input)
				require.NoError(t, err)
				exec, err := plan.Execute(ctx)
				require.NoError(t, err)
				require.Len(t, exec.Messages, 2)
				require.NotNil(t, exec.Messages[0].CachePolicy)
				assert.Equal(t, "ephemeral", exec.Messages[0].CachePolicy.Type)
			})
		})
	}
}

const crossFlatManifestYAML = `
id: cross_flat_agent
version: "1"
messages:
  - role: system
    layer_id: policy
    content: "You are a support agent. User: {{ .Input.user_name }}."
  - role: user
    content: "{{ .Input.q }}"
`

type crossFlatRegistries struct {
	file   *fileregistry.Registry
	embed  *embedregistry.Registry
	remote *remoteregistry.Registry
}

func newCrossFlatRegistries(t *testing.T) crossFlatRegistries {
	t.Helper()
	manifestBytes := []byte(crossFlatManifestYAML)

	dir := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, "cross_flat_agent.yaml"), manifestBytes, 0600),
	)
	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/cross_flat_agent.yaml": &fstest.MapFile{Data: manifestBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{"cross_flat_agent": manifestBytes}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)

	return crossFlatRegistries{file: fileReg, embed: embedReg, remote: remoteReg}
}

func crossFlatRegistryMatrix(t *testing.T) []crossComposeRegistry {
	t.Helper()
	regs := newCrossFlatRegistries(t)
	cached := remoteregistry.WithCache(regs.remote, time.Hour)
	return []crossComposeRegistry{
		{name: "file", plan: regs.file, resolver: regs.file, reader: regs.file},
		{name: "embed", plan: regs.embed, resolver: regs.embed, reader: regs.embed},
		{name: "remote", plan: regs.remote, resolver: regs.remote, reader: regs.remote},
		{name: "cached_remote", plan: cached, resolver: cached, reader: cached},
	}
}

func TestCrossRegistry_FlatCheckpoint_Parity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossFlatRegistries(t)
	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	fileDesc, err := regs.file.RecommendManifestDescriptor(ctx, "cross_flat_agent")
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		reg  prompty.ManifestCheckpointRegistry
	}{
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	} {
		t.Run(tc.name+"/digest_parity", func(t *testing.T) {
			t.Parallel()
			got, recommendErr := tc.reg.RecommendManifestDescriptor(ctx, "cross_flat_agent")
			require.NoError(t, recommendErr)
			assert.Equal(t, fileDesc.Digest, got.Digest)
			require.NoError(t, tc.reg.VerifyManifestDescriptor(ctx, got))
		})
	}

	for _, tc := range crossFlatRegistryMatrix(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("flat_manifest_tamper_fails", func(t *testing.T) {
				t.Parallel()
				checkpoint, ok := tc.reader.(prompty.ManifestCheckpointRegistry)
				require.True(t, ok)
				desc, descErr := checkpoint.RecommendManifestDescriptor(ctx, "cross_flat_agent")
				require.NoError(t, descErr)
				require.NoError(t, checkpoint.VerifyManifestDescriptor(ctx, desc))

				tamperReader := &crossComposeTamperReader{
					base: tc.reader,
					overrides: map[string][]byte{
						"cross_flat_agent": []byte(
							`{"id":"cross_flat_agent","messages":[{"role":"system","content":"TAMPERED"}]}`,
						),
					},
				}
				err := manifest.CheckpointVerify(ctx, desc, tamperReader, yaml.New())
				require.Error(t, err)
				assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
			})
		})
	}
}

type crossLateRegistries struct {
	file   *fileregistry.Registry
	embed  *embedregistry.Registry
	remote *remoteregistry.Registry
}

func newCrossLateRegistries(t *testing.T) crossLateRegistries {
	t.Helper()
	lateBytes := readFileregistryComposeFixture(t, "late_required_agent.yaml")

	dir := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, "late_required_agent.yaml"), lateBytes, 0600),
	)
	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/late_required_agent.yaml": &fstest.MapFile{Data: lateBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{"late_required_agent": lateBytes}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)

	return crossLateRegistries{file: fileReg, embed: embedReg, remote: remoteReg}
}

func TestCrossRegistry_LateBinding_Parity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossLateRegistries(t)
	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	resolvers := []struct {
		name string
		reg  prompty.Registry
	}{
		{"file", regs.file},
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	}

	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input, err := prompty.PlanInputFrom(struct {
				UserQuery string `prompt:"user_query"`
			}{UserQuery: "hello-" + tc.name})
			require.NoError(t, err)

			plan, err := tc.reg.Plan(ctx, "late_required_agent", input)
			require.NoError(t, err)
			plan, err = plan.WithLateInput(struct {
				PatientDossier string `prompt:"patient_dossier"`
			}{PatientDossier: "chart-" + tc.name})
			require.NoError(t, err)

			exec, err := plan.Execute(ctx)
			require.NoError(t, err)
			require.Len(t, exec.Messages, 1)
			text := exec.Messages[0].Content[0].(prompty.TextPart).Text
			assert.Contains(t, text, "hello-"+tc.name)
			assert.Contains(t, text, "chart-"+tc.name)
		})
	}
}

func TestCrossRegistry_ComposeProvenance_WireRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossComposeRegistries(t)
	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	checkpointRegs := []struct {
		name string
		reg  prompty.ManifestCheckpointRegistry
	}{
		{"file", regs.file},
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	}

	for _, tc := range checkpointRegs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input, err := prompty.PlanInputFrom(struct {
				Query string `prompt:"query"`
			}{Query: "wire-" + tc.name})
			require.NoError(t, err)
			plan, err := tc.reg.(prompty.Registry).Plan(ctx, "composed_main", input)
			require.NoError(t, err)

			cp, err := compiled.FromRenderPlanRegistry(ctx, plan, tc.reg, "composed_main")
			require.NoError(t, err)

			data, err := json.Marshal(cp)
			require.NoError(t, err)

			var wire map[string]any
			require.NoError(t, json.Unmarshal(data, &wire))
			assert.InDelta(t, 2, wire["format_version"], 0)

			var restored compiled.Prompt
			require.NoError(t, json.Unmarshal(data, &restored))
			exec := restored.PromptExecution()
			require.Len(t, exec.Messages, 3)
			require.NotNil(t, exec.Messages[0].Provenance)
			assert.Equal(t, "base_system", exec.Messages[0].Provenance.LayerID)
			require.NotNil(t, exec.Messages[1].Provenance)
			assert.Equal(t, "child_rules", exec.Messages[1].Provenance.LayerID)
			require.NotNil(t, exec.Messages[2].Provenance)
			assert.Equal(t, "user_turn", exec.Messages[2].Provenance.LayerID)
		})
	}
}

func newCrossChatHistoryRegistries(t *testing.T) struct {
	file   *fileregistry.Registry
	embed  *embedregistry.Registry
	remote *remoteregistry.Registry
} {
	t.Helper()
	historyBytes := readFileregistryComposeFixture(t, "chat_history_messages.yaml")
	lateBytes := readFileregistryComposeFixture(t, "late_binding_agent.yaml")

	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"chat_history_messages.yaml": historyBytes,
		"late_binding_agent.yaml":    lateBytes,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600))
	}
	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/chat_history_messages.yaml": &fstest.MapFile{Data: historyBytes},
		"prompts/late_binding_agent.yaml":    &fstest.MapFile{Data: lateBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{
			"chat_history_messages": historyBytes,
			"late_binding_agent":    lateBytes,
		}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)

	return struct {
		file   *fileregistry.Registry
		embed  *embedregistry.Registry
		remote *remoteregistry.Registry
	}{file: fileReg, embed: embedReg, remote: remoteReg}
}

func TestCrossRegistry_ChatHistory_FormatMessages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossChatHistoryRegistries(t)
	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	resolvers := []struct {
		name string
		reg  prompty.Registry
	}{
		{"file", regs.file},
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	}

	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			history := []prompty.ChatMessage{
				{
					Role:    prompty.RoleUser,
					Content: []prompty.ContentPart{prompty.TextPart{Text: "prior-" + tc.name}},
				},
			}
			input, err := prompty.PlanInputFrom(struct {
				Query       string                `prompt:"query"`
				ChatHistory []prompty.ChatMessage `prompt:"chat_history"`
			}{Query: "now", ChatHistory: history})
			require.NoError(t, err)
			plan, err := tc.reg.Plan(ctx, "chat_history_messages", input)
			require.NoError(t, err)
			exec, err := plan.Execute(ctx)
			require.NoError(t, err)
			require.Len(t, exec.Messages, 3)
			assert.Equal(t, "prior-"+tc.name, exec.Messages[1].Content[0].(prompty.TextPart).Text)
		})
	}
}

func TestCrossRegistry_ChatHistory_PreservesProvenance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossChatHistoryRegistries(t)

	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)
	resolvers := []struct {
		name string
		reg  prompty.Registry
	}{
		{"file", regs.file},
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	}

	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			history := []prompty.ChatMessage{
				{
					Role:    prompty.RoleUser,
					Content: []prompty.ContentPart{prompty.TextPart{Text: "prior"}},
					Provenance: &prompty.MessageProvenance{
						ManifestID: "history-src",
						LayerID:    "turn_1",
					},
				},
			}
			input, err := prompty.PlanInputFrom(struct {
				Query       string                `prompt:"query"`
				ChatHistory []prompty.ChatMessage `prompt:"chat_history"`
			}{Query: "now", ChatHistory: history})
			require.NoError(t, err)
			plan, err := tc.reg.Plan(ctx, "chat_history_messages", input)
			require.NoError(t, err)
			exec, err := plan.Execute(ctx)
			require.NoError(t, err)
			require.NotNil(t, exec.Messages[1].Provenance)
			assert.Equal(t, "history-src", exec.Messages[1].Provenance.ManifestID)
			assert.Equal(t, "turn_1", exec.Messages[1].Provenance.LayerID)
		})
	}
}

func TestCrossRegistry_Checkpoint_MissingImportFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	missingChildBytes := readFileregistryComposeFixture(t, "composed_main_missing_child.yaml")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "composed_main_missing_child.yaml"),
		missingChildBytes,
		0600,
	))
	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{
			"composed_main_missing_child": missingChildBytes,
		}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)

	// embedregistry.New eagerly expands compose imports; undeployable children fail at construction.
	checkpointers := []struct {
		name string
		reg  prompty.ManifestCheckpointRegistry
	}{
		{"file", fileReg},
		{"remote", remoteReg},
	}
	for _, tc := range checkpointers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.reg.RecommendManifestDescriptor(ctx, "composed_main_missing_child")
			require.Error(t, err)
			require.ErrorIs(t, err, prompty.ErrTemplateNotFound)
			require.Contains(t, err.Error(), "read import")
		})
	}
}

func TestCrossRegistry_LateBinding_RequiredWithoutLateInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lateRequiredBytes := readFileregistryComposeFixture(t, "late_required_agent.yaml")

	dir := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, "late_required_agent.yaml"), lateRequiredBytes, 0600),
	)
	fileReg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	embedFS := fstest.MapFS{
		"prompts/late_required_agent.yaml": &fstest.MapFile{Data: lateRequiredBytes},
	}
	embedReg, err := embedregistry.New(embedFS, "prompts", embedregistry.WithParser(yaml.New()))
	require.NoError(t, err)

	remoteReg, err := remoteregistry.New(
		&staticManifestFetcher{data: map[string][]byte{"late_required_agent": lateRequiredBytes}},
		remoteregistry.WithParser(yaml.New()),
	)
	require.NoError(t, err)
	cachedRemote := remoteregistry.WithCache(remoteReg, time.Hour)

	resolvers := []struct {
		name string
		reg  prompty.Registry
	}{
		{"file", fileReg},
		{"embed", embedReg},
		{"remote", remoteReg},
		{"cached_remote", cachedRemote},
	}
	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input, err := prompty.PlanInputFrom(struct {
				UserQuery string `prompt:"user_query"`
			}{UserQuery: "hello-" + tc.name})
			require.NoError(t, err)
			plan, err := tc.reg.Plan(ctx, "late_required_agent", input)
			require.NoError(t, err)
			_, err = plan.Execute(ctx)
			require.Error(t, err)
			assert.ErrorIs(t, err, prompty.ErrMissingVariable)
		})
	}
}

func TestCrossRegistry_LateBinding_OptionalLate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regs := newCrossChatHistoryRegistries(t)
	cachedRemote := remoteregistry.WithCache(regs.remote, time.Hour)

	resolvers := []struct {
		name string
		reg  prompty.Registry
	}{
		{"file", regs.file},
		{"embed", regs.embed},
		{"remote", regs.remote},
		{"cached_remote", cachedRemote},
	}

	for _, tc := range resolvers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input, err := prompty.PlanInputFrom(struct {
				UserQuery string `prompt:"user_query"`
			}{UserQuery: "hello-" + tc.name})
			require.NoError(t, err)
			plan, err := tc.reg.Plan(ctx, "late_binding_agent", input)
			require.NoError(t, err)
			exec, err := plan.Execute(ctx)
			require.NoError(t, err)
			require.Len(t, exec.Messages, 1)
			assert.Contains(
				t,
				exec.Messages[0].Content[0].(prompty.TextPart).Text,
				"hello-"+tc.name,
			)
		})
	}
}

func tamperedComposedChildManifestBytes() []byte {
	return []byte(`{"id":"composed_child","inputs":{"clinic_name":{"type":"string"}},` +
		`"layers":[{"id":"child_rules","role":"system","content":"TAMPERED"}]}`)
}
