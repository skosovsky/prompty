package compiled_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/compiled"
	"github.com/skosovsky/prompty/manifest"
)

func capturedTestPrompt(t *testing.T, raw map[string]any) (*compiled.Prompt, prompty.ManifestDescriptor) {
	t.Helper()
	data, err := json.Marshal(raw)
	require.NoError(t, err)
	snapshot, err := prompty.NewManifestSnapshot(
		"agent",
		prompty.SourceBundle{Sources: map[string][]byte{"main": data}},
		func(ctx context.Context, bundle prompty.SourceBundle, _ prompty.RegistryPlanInput) (*prompty.ChatPromptTemplate, error) {
			return manifest.Parse(
				bundle.Sources["main"],
				manifest.NewJSONParser(),
				manifest.WithCompose(
					manifest.ComposeContext{
						Ctx:                         ctx,
						Values:                      prompty.ComposeValues{},
						Loader:                      &manifest.MemoryLoader{ByID: map[string]*manifest.RawManifest{}},
						AllowMissingConditionValues: false,
					},
				),
			)
		},
	)
	require.NoError(t, err)
	plan, err := snapshot.Plan(context.Background(), prompty.RegistryPlanInput{})
	require.NoError(t, err)
	prompt, err := compiled.FromRenderPlan(context.Background(), plan)
	require.NoError(t, err)
	return prompt, snapshot.Descriptor()
}

func plainSource() map[string]any {
	return map[string]any{
		"id": "agent",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}}},
		},
	}
}

func TestFromRenderPlan_RequiresCapturedSource(t *testing.T) {
	t.Parallel()
	_, err := compiled.FromRenderPlan(context.Background(), nil)
	require.Error(t, err)
	template, err := prompty.NewChatPromptTemplate(
		[]prompty.MessageTemplate{{Role: prompty.RoleUser, Content: prompty.TextContent("hi")}},
	)
	require.NoError(t, err)
	_, err = compiled.FromRenderPlan(context.Background(), prompty.NewRenderPlan(template))
	require.Error(t, err)
}

func TestPrompt_RoundTrip(t *testing.T) {
	t.Parallel()
	prompt, descriptor := capturedTestPrompt(t, plainSource())
	data, err := json.Marshal(prompt)
	require.NoError(t, err)
	var restored compiled.Prompt
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.Equal(t, "agent", restored.ManifestID())
	assert.Equal(t, descriptor.Digest, restored.ManifestDigest())
	assert.Equal(t, compiled.DigestSourceCapturedSources, restored.DigestSource())
}

func TestPrompt_UnmarshalRejectsLegacyFormats(t *testing.T) {
	t.Parallel()
	for _, wire := range []string{
		`{"format_version":1,"manifest_id":"a","manifest_digest":"d","digest_source":"manifest_bytes","execution":{"messages":[]}}`,
		`{"format_version":2,"manifest_id":"a","manifest_digest":"d","digest_source":"canonical_snapshot","execution":{"messages":[]}}`,
	} {
		err := json.Unmarshal([]byte(wire), new(compiled.Prompt))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "format_version")
	}
}

func TestPrompt_RoundTripProvenanceAndCachePolicy(t *testing.T) {
	t.Parallel()
	cache := &prompty.CachePolicy{Mechanism: prompty.CacheBlockMarker}
	content := []any{map[string]any{"type": "text", "text": "hi", "cache_policy": cache}}
	layer := map[string]any{"id": "rules", "role": "system", "cache_policy": cache, "content": content}
	prompt, _ := capturedTestPrompt(t, map[string]any{"id": "agent", "layers": []any{layer}})
	data, err := json.Marshal(prompt)
	require.NoError(t, err)
	var restored compiled.Prompt
	require.NoError(t, json.Unmarshal(data, &restored))
	execution := restored.PromptExecution()
	require.Len(t, execution.Messages, 1)
	require.NotNil(t, execution.Messages[0].Provenance)
	assert.Equal(t, "rules", execution.Messages[0].Provenance.LayerID)
	require.NotNil(t, execution.Messages[0].CachePolicy)
	assert.Equal(t, prompty.CacheBlockMarker, execution.Messages[0].CachePolicy.Mechanism)
}

func TestPrompt_ExecutionCloneIsImmutable(t *testing.T) {
	t.Parallel()
	prompt, _ := capturedTestPrompt(t, plainSource())
	cloned := prompt.PromptExecution()
	cloned.Messages[0].Content = []prompty.ContentPart{prompty.TextPart{Text: "mutated"}}
	assert.Equal(t, "hi", prompt.PromptExecution().Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestPrompt_MarshalIncludesFormatVersion(t *testing.T) {
	t.Parallel()
	prompt, _ := capturedTestPrompt(t, plainSource())
	data, err := json.Marshal(prompt)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	assert.InDelta(t, 3, wire["format_version"], 0)
}
