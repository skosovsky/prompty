package prompts

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type registryStub struct{}

func (registryStub) Plan(context.Context, string, prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	return nil, nil
}

func (registryStub) DescribePrompt(_ context.Context, id string) (prompty.TemplateDescriptor, error) {
	return prompty.TemplateDescriptor{
		Metadata:      prompty.PromptMetadata{ID: id},
		RequiredTools: []string{"lookup"},
	}, nil
}

func (registryStub) ReadManifestBytes(context.Context, string) ([]byte, error) {
	return []byte("manifest"), nil
}

func (registryStub) RecommendManifestDescriptor(_ context.Context, id string) (prompty.ManifestDescriptor, error) {
	return prompty.ManifestDescriptor{ID: id, Digest: "sha256:test"}, nil
}

func (registryStub) VerifyManifestDescriptor(context.Context, prompty.ManifestDescriptor) error {
	return nil
}

var _ prompty.PromptCatalogRegistry = registryStub{}

func TestPromptIndex_NewRecipeFromJSON_RawInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	index := NewPromptIndex(registryStub{})
	payload, err := json.Marshal(SupportAgentInput{UserQuery: "hello"})
	require.NoError(t, err)

	recipe, ok, err := index.NewRecipeFromJSON(ctx, SupportAgent, payload)

	require.NoError(t, err)
	require.True(t, ok)
	require.IsType(t, SupportAgentRecipe{}, recipe)
	assert.Equal(t, SupportAgent, recipe.PromptID())
}

func TestPromptIndex_NewRecipeFromJSON_ComposePayloadRequired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	index := NewPromptIndex(registryStub{})
	payload, err := json.Marshal(ComposedConditionalMainRecipePayload{
		Input: ComposedConditionalMainInput{Query: "hello"},
	})
	require.NoError(t, err)

	_, ok, err := index.NewRecipeFromJSON(ctx, ComposedConditionalMain, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compose context is required")
}

func TestPromptIndex_NewRecipeFromJSON_ComposePayloadAndCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	index := NewPromptIndex(registryStub{})
	compose := NewComposedConditionalMainComposeContext(false)
	payload, err := json.Marshal(ComposedConditionalMainRecipePayload{
		Input:   ComposedConditionalMainInput{Query: "hello"},
		Compose: &compose,
	})
	require.NoError(t, err)

	recipe, ok, err := index.NewRecipeFromJSON(ctx, ComposedConditionalMain, payload)

	require.NoError(t, err)
	require.True(t, ok)
	checkpointPayload, err := recipe.CheckpointJSON()
	require.NoError(t, err)

	restored, ok, err := index.DecodeRecipeCheckpoint(ComposedConditionalMain, checkpointPayload)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, ComposedConditionalMain, restored.PromptID())
}

func TestPromptIndex_NewRecipeFromJSON_RejectsMissingComposeField(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	index := NewPromptIndex(registryStub{})
	payload := []byte(`{"input":{"query":"hello"},"compose":{}}`)

	_, ok, err := index.NewRecipeFromJSON(ctx, ComposedConditionalMain, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compose context requires")
}

func TestPromptIndex_DecodeRecipeCheckpoint_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	index := NewPromptIndex(registryStub{})
	payload := []byte(`{"descriptor":{"id":"support_agent","digest":"sha256:test"},"input":{"user_query":"hello"},"unknown":true}`)

	_, ok, err := index.DecodeRecipeCheckpoint(SupportAgent, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestPromptIndex_DecodeRecipeCheckpoint_RejectsUnknownNestedInputFields(t *testing.T) {
	t.Parallel()
	index := NewPromptIndex(registryStub{})
	payload := []byte(`{"descriptor":{"id":"support_agent","digest":"sha256:test"},"input":{"user_query":"hello","unknown":true}}`)

	_, ok, err := index.DecodeRecipeCheckpoint(SupportAgent, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestPromptIndex_DecodeRecipeCheckpoint_RejectsDescriptorIDMismatch(t *testing.T) {
	t.Parallel()
	index := NewPromptIndex(registryStub{})
	payload := []byte(`{"descriptor":{"id":"composed_main","digest":"sha256:test"},"input":{"user_query":"hello"}}`)

	_, ok, err := index.DecodeRecipeCheckpoint(SupportAgent, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint descriptor id mismatch")
}

func TestNewSupportAgentRecipeFromCheckpoint_RejectsDescriptorIDMismatch(t *testing.T) {
	t.Parallel()
	checkpoint := prompty.PromptRecipeNoLateCheckpoint[SupportAgentInput]{
		Descriptor: prompty.ManifestDescriptor{ID: string(ComposedMain), Digest: "sha256:test"},
		Input:      SupportAgentInput{UserQuery: "hello"},
	}

	_, err := NewSupportAgentRecipeFromCheckpoint(checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint descriptor id mismatch")
}

func TestNewSupportAgentRecipeFromCheckpoint_ValidatesInput(t *testing.T) {
	t.Parallel()
	checkpoint := prompty.PromptRecipeNoLateCheckpoint[SupportAgentInput]{
		Descriptor: prompty.ManifestDescriptor{ID: string(SupportAgent), Digest: "sha256:test"},
		Input:      SupportAgentInput{},
	}

	_, err := NewSupportAgentRecipeFromCheckpoint(checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate input")
}

func TestNewSupportAgentRecipeFromCheckpoint_AppliesDefaults(t *testing.T) {
	t.Parallel()
	checkpoint := prompty.PromptRecipeNoLateCheckpoint[SupportAgentInput]{
		Descriptor: prompty.ManifestDescriptor{ID: string(SupportAgent), Digest: "sha256:test"},
		Input:      SupportAgentInput{UserQuery: "hello"},
	}

	recipe, err := NewSupportAgentRecipeFromCheckpoint(checkpoint)
	require.NoError(t, err)
	restored, err := recipe.Checkpoint()
	require.NoError(t, err)

	require.NotNil(t, restored.Input.BotName)
	assert.Equal(t, "SupportBot", *restored.Input.BotName)
}

func TestNewComposedConditionalMainRecipeFromCheckpoint_RejectsEmptyComposeValues(t *testing.T) {
	t.Parallel()
	checkpoint := prompty.PromptRecipeNoLateCheckpoint[ComposedConditionalMainInput]{
		Descriptor: prompty.ManifestDescriptor{ID: string(ComposedConditionalMain), Digest: "sha256:test"},
		Input:      ComposedConditionalMainInput{Query: "hello"},
		RuntimeOptions: prompty.PromptRuntimeOptions{
			ComposeValues: prompty.MustJSONDocumentFromMap(map[string]interface{}{}),
		},
	}

	_, err := NewComposedConditionalMainRecipeFromCheckpoint(checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint compose context requires")
}

func TestNewLateRequiredAgentRecipeFromCheckpoint_RejectsInvalidLateInput(t *testing.T) {
	t.Parallel()
	checkpoint := prompty.PromptRecipeCheckpoint[LateRequiredAgentInput, LateRequiredAgentLateInput]{
		Descriptor: prompty.ManifestDescriptor{ID: string(LateRequiredAgent), Digest: "sha256:test"},
		Input:      LateRequiredAgentInput{UserQuery: "hello"},
		LateInput:  &LateRequiredAgentLateInput{},
		LateBound:  true,
	}

	_, err := NewLateRequiredAgentRecipeFromCheckpoint(checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input")
}

func TestPromptIndex_DecodeRecipeCheckpoint_RejectsInvalidLateInput(t *testing.T) {
	t.Parallel()
	index := NewPromptIndex(registryStub{})
	payload := []byte(`{
		"descriptor":{"id":"late_required_agent","digest":"sha256:test"},
		"input":{"user_query":"hello"},
		"late_input":{},
		"late_bound":true
	}`)

	_, ok, err := index.DecodeRecipeCheckpoint(LateRequiredAgent, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input")
}

func TestPromptEntry_MetadataRequiredToolsAndDescriptor(t *testing.T) {
	t.Parallel()
	index := NewPromptIndex(nil)
	entry, ok := index.Lookup(SupportAgent)
	require.True(t, ok)

	metadata := entry.Metadata()
	tools := entry.RequiredTools()
	desc := entry.Descriptor()

	assert.Equal(t, string(SupportAgent), metadata.ID)
	assert.Empty(t, tools)
	assert.Equal(t, string(SupportAgent), desc.ID)
	assert.NotEmpty(t, desc.Digest)
}

func TestPromptEntry_NewRecipeFromJSON_RequiresRegistry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	index := NewPromptIndex(nil)
	payload, err := json.Marshal(SupportAgentInput{UserQuery: "hello"})
	require.NoError(t, err)

	_, ok, err := index.NewRecipeFromJSON(ctx, SupportAgent, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry is required")
}

func TestPromptIndex_NewRecipeFromJSON_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	index := NewPromptIndex(registryStub{})
	payload := []byte(`{"user_query":"hello","unknown":true}`)

	_, ok, err := index.NewRecipeFromJSON(ctx, SupportAgent, payload)

	require.True(t, ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestPromptCatalog_DescriptorRejectsUnknownID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog := NewPromptCatalog(registryStub{})

	_, err := catalog.Descriptor(ctx, PromptID("unknown"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown prompt id")
}

func TestPromptCatalog_NewRecipeRequiresRegistry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog := NewPromptCatalog(nil)

	_, err := catalog.NewSupportAgentRecipe(ctx, SupportAgentInput{UserQuery: "hello"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipe requires registry")
}
