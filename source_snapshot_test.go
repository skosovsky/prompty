package prompty_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/embedregistry"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/internal/compiled"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/remoteregistry"
)

const oldSource = `{"id":"agent","messages":[{"role":"user","content":[{"type":"text","text":"OLD"}]}]}`
const newSource = `{"id":"agent","messages":[{"role":"user","content":[{"type":"text","text":"NEW"}]}]}`

func TestSourceSnapshot_FileCacheCannotAcquireNewDescriptor(t *testing.T) {
	t.Parallel()
	// Arrange: an old generation in cache and a new descriptor from another registry.
	ctx := context.Background()
	dir := t.TempDir()
	name := filepath.Join(dir, "agent.json")
	require.NoError(t, os.WriteFile(name, []byte(oldSource), 0600))
	registry, err := fileregistry.New(dir, fileregistry.WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	oldPlan, err := registry.Plan(ctx, "agent", prompty.RegistryPlanInput{})
	require.NoError(t, err)
	oldDescriptor, ok := oldPlan.SourceDescriptor()
	require.True(t, ok)
	require.NoError(t, os.WriteFile(name, []byte(newSource), 0600))
	fresh, err := fileregistry.New(dir, fileregistry.WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	newDescriptor, err := fresh.RecommendManifestDescriptor(ctx, "agent")
	require.NoError(t, err)
	recipe, err := prompty.NewPromptRecipeNoLate(newDescriptor, struct{}{})
	require.NoError(t, err)
	// Act/Assert: old cache rejects new identity; Reload permits the matching new generation.
	_, err = recipe.Execute(ctx, registry)
	require.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
	frozen, err := compiled.FromRenderPlan(ctx, oldPlan)
	require.NoError(t, err)
	require.Equal(t, oldDescriptor.Digest, frozen.ManifestDigest())
	require.Equal(t, "OLD", frozen.PromptExecution().Messages[0].Content[0].(prompty.TextPart).Text)
	registry.Reload()
	execution, err := recipe.Execute(ctx, registry)
	require.NoError(t, err)
	require.Equal(t, "NEW", execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestSourceSnapshot_PartialsAndSourceDisappearance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "partials"), 0700))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "agent.json"),
			[]byte(
				`{"id":"agent","messages":[{"role":"user","content":[{"type":"text","text":"{{template \"rule\"}}"}]}]}`,
			),
			0600,
		),
	)
	partial := filepath.Join(dir, "partials", "rules.tmpl")
	require.NoError(t, os.WriteFile(partial, []byte(`{{define "rule"}}OLD{{end}}`), 0600))
	registry, err := fileregistry.New(
		dir,
		fileregistry.WithParser(manifest.NewJSONParser()),
		fileregistry.WithPartials("partials/*.tmpl"),
	)
	require.NoError(t, err)
	snapshot, err := registry.CaptureManifest(ctx, "agent")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(partial, []byte(`{{define "rule"}}NEW{{end}}`), 0600))
	registry.Reload()
	updated, err := registry.CaptureManifest(ctx, "agent")
	require.NoError(t, err)
	require.NotEqual(t, snapshot.Descriptor(), updated.Descriptor())
	require.NoError(t, os.RemoveAll(dir))
	plan, err := snapshot.Plan(ctx, prompty.RegistryPlanInput{})
	require.NoError(t, err)
	execution, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Equal(t, "OLD", execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

type changingSource struct{ calls int }

func (s *changingSource) Fetch(context.Context, string) ([]byte, error) {
	s.calls++
	if s.calls == 1 {
		return []byte(oldSource), nil
	}
	return []byte(newSource), nil
}

func TestSourceSnapshot_RemoteReadsOnceAndCacheRetainsIdentity(t *testing.T) {
	t.Parallel()
	for _, cached := range []bool{false, true} {
		source := &changingSource{}
		registry, err := remoteregistry.New(source, remoteregistry.WithParser(manifest.NewJSONParser()))
		require.NoError(t, err)
		var selected prompty.ManifestCheckpointRegistry = registry
		if cached {
			selected = remoteregistry.WithCache(registry, time.Hour)
		}
		plan, err := selected.Plan(context.Background(), "agent", prompty.RegistryPlanInput{})
		require.NoError(t, err)
		frozen, err := compiled.FromRenderPlan(context.Background(), plan)
		require.NoError(t, err)
		require.Equal(t, 1, source.calls, "compile must not re-read source after Plan")
		require.Equal(t, "OLD", frozen.PromptExecution().Messages[0].Content[0].(prompty.TextPart).Text)
		descriptor, err := selected.RecommendManifestDescriptor(context.Background(), "agent")
		require.NoError(t, err)
		if cached {
			require.Equal(t, frozen.ManifestDigest(), descriptor.Digest)
			require.Equal(t, 1, source.calls)
		} else {
			require.NotEqual(t, frozen.ManifestDigest(), descriptor.Digest)
			require.Equal(t, 2, source.calls)
		}
	}
}

func TestSourceSnapshot_EmbedSealsCallerFSAtConstruction(t *testing.T) {
	t.Parallel()
	source := fstest.MapFS{"agent.json": &fstest.MapFile{Data: []byte(oldSource)}}
	registry, err := embedregistry.New(source, ".", embedregistry.WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	source["agent.json"].Data = []byte(newSource)
	plan, err := registry.Plan(context.Background(), "agent", prompty.RegistryPlanInput{})
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, "OLD", execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestSourceSnapshot_ConcurrentReloadPlanIsolation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent.json"), []byte(oldSource), 0600))
	registry, err := fileregistry.New(dir, fileregistry.WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	descriptor, err := registry.RecommendManifestDescriptor(context.Background(), "agent")
	require.NoError(t, err)
	var workers sync.WaitGroup
	for range 12 {
		workers.Go(func() {
			for range 12 {
				registry.Reload()
				plan, err := prompty.VerifiedManifestPlan(
					context.Background(),
					registry,
					descriptor,
					prompty.RegistryPlanInput{},
				)
				if err != nil {
					t.Error(err)
					return
				}
				frozen, err := compiled.FromRenderPlan(context.Background(), plan)
				if err != nil {
					t.Error(err)
					return
				}
				if frozen.ManifestDigest() != descriptor.Digest ||
					frozen.PromptExecution().Messages[0].Content[0].(prompty.TextPart).Text != "OLD" {
					t.Error("snapshot lost source isolation")
					return
				}
			}
		})
	}
	workers.Wait()
}

type separatePlanRegistry struct {
	snapshot                       *prompty.ManifestSnapshot
	captures, plans, verifications int
}

func (r *separatePlanRegistry) CaptureManifest(context.Context, string) (*prompty.ManifestSnapshot, error) {
	r.captures++
	return r.snapshot, nil
}
func (r *separatePlanRegistry) Plan(context.Context, string, prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	r.plans++
	tpl, err := prompty.NewChatPromptTemplate(
		[]prompty.MessageTemplate{{Role: prompty.RoleUser, Content: prompty.TextContent("NEW")}},
	)
	if err != nil {
		return nil, err
	}
	return prompty.NewRenderPlan(tpl), nil
}

func (r *separatePlanRegistry) RecommendManifestDescriptor(
	context.Context,
	string,
) (prompty.ManifestDescriptor, error) {
	return r.snapshot.Descriptor(), nil
}
func (r *separatePlanRegistry) VerifyManifestDescriptor(context.Context, prompty.ManifestDescriptor) error {
	r.verifications++
	return nil
}

func TestSourceSnapshot_CheckedRecipesDoNotCallUnrelatedPlan(t *testing.T) {
	t.Parallel()
	// Arrange: capture carries OLD while the independently exposed Plan would return NEW.
	snapshot, err := prompty.NewManifestSnapshot(
		"agent",
		prompty.SourceBundle{Sources: map[string][]byte{"main": []byte("OLD")}},
		func(_ context.Context, bundle prompty.SourceBundle, _ prompty.RegistryPlanInput) (*prompty.ChatPromptTemplate, error) {
			return prompty.NewChatPromptTemplate(
				[]prompty.MessageTemplate{
					{Role: prompty.RoleUser, Content: prompty.TextContent(string(bundle.Sources["main"]))},
				},
			)
		},
	)
	require.NoError(t, err)
	registry := &separatePlanRegistry{snapshot: snapshot}
	recipe, err := prompty.NewPromptRecipeNoLate(snapshot.Descriptor(), struct{}{})
	require.NoError(t, err)
	// Act: both typed recipe and raw checkpoint bind through one captured-source port.
	typed, err := recipe.Execute(context.Background(), registry)
	require.NoError(t, err)
	plan, err := prompty.BindRuntime(
		context.Background(),
		registry,
		prompty.RuntimeRecipeCheckpoint{Descriptor: snapshot.Descriptor(), State: prompty.RuntimeRecipeStateNoLate},
		prompty.RuntimeOverlay{},
		prompty.ToolScope{},
	)
	require.NoError(t, err)
	raw, err := plan.Execute(context.Background())
	// Assert: neither an independent verifier nor a storage-backed Plan participates.
	require.NoError(t, err)
	require.Equal(t, "OLD", typed.Messages[0].Content[0].(prompty.TextPart).Text)
	require.Equal(t, "OLD", raw.Messages[0].Content[0].(prompty.TextPart).Text)
	require.Equal(t, 2, registry.captures)
	require.Zero(t, registry.plans)
	require.Zero(t, registry.verifications)
	descriptor, ok := plan.SourceDescriptor()
	require.True(t, ok)
	require.Equal(t, snapshot.Descriptor(), descriptor)
}

type retainedSourcePort struct{ snapshot *prompty.ManifestSnapshot }

func (r retainedSourcePort) CaptureManifest(context.Context, string) (*prompty.ManifestSnapshot, error) {
	return r.snapshot, nil
}

func TestSourceSnapshot_MinimalCustomPortCanRestoreRecipes(t *testing.T) {
	t.Parallel()
	snapshot, err := prompty.NewManifestSnapshot(
		"agent",
		prompty.SourceBundle{Sources: map[string][]byte{"main": []byte("OLD")}},
		func(_ context.Context, bundle prompty.SourceBundle, _ prompty.RegistryPlanInput) (*prompty.ChatPromptTemplate, error) {
			return prompty.NewChatPromptTemplate(
				[]prompty.MessageTemplate{
					{Role: prompty.RoleUser, Content: prompty.TextContent(string(bundle.Sources["main"]))},
				},
			)
		},
	)
	require.NoError(t, err)
	recipe, err := prompty.NewPromptRecipeNoLate(snapshot.Descriptor(), struct{}{})
	require.NoError(t, err)
	execution, err := recipe.Execute(context.Background(), retainedSourcePort{snapshot: snapshot})
	require.NoError(t, err)
	require.Equal(t, "OLD", execution.Messages[0].Content[0].(prompty.TextPart).Text)
}
