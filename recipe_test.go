package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptRecipe_Execute_RebuildsPlanAndBindsLateInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipe[recipeInput, recipeLateInput](
		reg.desc,
		recipeInput{Query: "hello"},
	)
	require.NoError(t, err)
	recipe, err = recipe.BindLate(recipeLateInput{Suffix: "world"})
	require.NoError(t, err)

	exec, err := recipe.Execute(ctx, reg)

	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "hello world", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, reg.desc.ID, reg.plannedID)
}

func TestPromptRecipe_Checkpoint_RestoresJSONSafeDTO(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipe[recipeInput, recipeLateInput](
		reg.desc,
		recipeInput{Query: "hello"},
	)
	require.NoError(t, err)
	recipe, err = recipe.BindLate(recipeLateInput{Suffix: "world"})
	require.NoError(t, err)
	cp, err := recipe.Checkpoint()
	require.NoError(t, err)
	data, err := json.Marshal(
		cp, //nolint:musttag // checkpoint DTO fields are JSON-tagged; generic instantiation is opaque to musttag.
	)
	require.NoError(t, err)
	var restoredCP PromptRecipeCheckpoint[recipeInput, recipeLateInput]
	require.NoError(t, json.Unmarshal(data, &restoredCP))

	restored, err := PromptRecipeFromCheckpoint(restoredCP)

	require.NoError(t, err)
	assert.True(t, restored.LateBound)
	require.NotNil(t, restored.LateInput)
	assert.Equal(t, "world", restored.LateInput.Suffix)
	assert.Equal(t, reg.desc, restored.Descriptor)
}

func TestPromptRecipeFromCheckpoint_RejectsLateInputWhenNotMarkedBound(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	late := recipeLateInput{Suffix: "world"}
	checkpoint := PromptRecipeCheckpoint[recipeInput, recipeLateInput]{
		Descriptor: reg.desc,
		Input:      recipeInput{Query: "hello"},
		LateInput:  &late,
		LateBound:  false,
	}

	_, err := PromptRecipeFromCheckpoint(checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input is present but not marked bound")
}

func TestPromptRecipeFromCheckpoint_RejectsInvalidBoundLateInput(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	checkpoint := PromptRecipeCheckpoint[recipeInput, recipeRequiredLateInput]{
		Descriptor: reg.desc,
		Input:      recipeInput{Query: "hello"},
		LateInput:  &recipeRequiredLateInput{},
		LateBound:  true,
	}

	_, err := PromptRecipeFromCheckpoint(checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input")
}

func TestPromptRecipeCheckpoint_UnmarshalRejectsLateInputWhenNotMarkedBound(t *testing.T) {
	t.Parallel()
	var checkpoint PromptRecipeCheckpoint[recipeInput, recipeLateInput]
	payload := []byte(`{
		"descriptor":{"id":"agent","digest":"digest"},
		"input":{"query":"hello"},
		"late_input":{"suffix":"world"},
		"late_bound":false
	}`)

	err := json.Unmarshal(payload, &checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input is present but not marked bound")
}

func TestPromptRecipeCheckpoint_UnmarshalRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	var checkpoint PromptRecipeCheckpoint[recipeInput, recipeLateInput]
	payload := []byte(`{
		"descriptor":{"id":"agent","digest":"digest"},
		"input":{"query":"hello","unknown":true}
	}`)

	err := json.Unmarshal(payload, &checkpoint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestPromptRecipe_ExecuteRejectsMutatedLateInputWhenNotMarkedBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipe[recipeInput, recipeLateInput](
		reg.desc,
		recipeInput{Query: "hello"},
	)
	require.NoError(t, err)
	late := recipeLateInput{Suffix: "world"}
	recipe.LateInput = &late

	_, err = recipe.Execute(ctx, reg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input is present but not marked bound")
}

func TestPromptRecipe_CheckpointRejectsMutatedLateInputWhenNotMarkedBound(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipe[recipeInput, recipeLateInput](
		reg.desc,
		recipeInput{Query: "hello"},
	)
	require.NoError(t, err)
	late := recipeLateInput{Suffix: "world"}
	recipe.LateInput = &late

	_, err = recipe.Checkpoint()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input is present but not marked bound")
}

func TestPromptRecipe_CheckpointRejectsMutatedInvalidBoundLateInput(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipe[recipeInput, recipeRequiredLateInput](
		reg.desc,
		recipeInput{Query: "hello"},
	)
	require.NoError(t, err)
	recipe.LateInput = &recipeRequiredLateInput{}
	recipe.LateBound = true

	_, err = recipe.Checkpoint()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input")
}

func TestPromptRecipe_Execute_VerifiesDigestBeforePlanning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := newRecipeTestRegistry(t)
	badDesc := ManifestDescriptor{ID: reg.desc.ID, Digest: "stale"}
	recipe, err := NewPromptRecipeNoLate[recipeInput](badDesc, recipeInput{Query: "hello"})
	require.NoError(t, err)

	_, err = recipe.Execute(ctx, reg)

	require.ErrorIs(t, err, ErrManifestDigestMismatch)
	assert.False(t, reg.planCalled)
}

func TestPromptRecipe_WithComposeContext_StoresRuntimeOptions(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipeNoLate[recipeInput](reg.desc, recipeInput{Query: "hello"})
	require.NoError(t, err)

	recipe, err = recipe.WithComposeContext(recipeComposeContext{workspaceEnabled: true})

	require.NoError(t, err)
	values, err := recipe.RuntimeOptions.composeValues()
	require.NoError(t, err)
	got, ok := values.Lookup("capabilities.workspace_enabled")
	require.True(t, ok)
	assert.True(t, got.(bool))
}

func TestPromptRuntimeOptionsFromComposeContext_ValidatesContext(t *testing.T) {
	t.Parallel()

	_, err := PromptRuntimeOptionsFromComposeContext(invalidRecipeComposeContext{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "compose context requires")
}

func TestPromptRecipe_Checkpoint_RoundTripsComposeRuntimeOptions(t *testing.T) {
	t.Parallel()
	reg := newRecipeTestRegistry(t)
	recipe, err := NewPromptRecipeNoLate[recipeInput](reg.desc, recipeInput{Query: "hello"})
	require.NoError(t, err)
	recipe, err = recipe.WithComposeContext(recipeComposeContext{workspaceEnabled: true})
	require.NoError(t, err)
	checkpoint, err := recipe.Checkpoint()
	require.NoError(t, err)
	data, err := json.Marshal(
		checkpoint, //nolint:musttag // checkpoint DTO fields are JSON-tagged; generic instantiation is opaque to musttag.
	)
	require.NoError(t, err)
	var restoredCP PromptRecipeNoLateCheckpoint[recipeInput]
	require.NoError(t, json.Unmarshal(data, &restoredCP))

	restored, err := PromptRecipeNoLateFromCheckpoint(restoredCP)

	require.NoError(t, err)
	values, err := restored.RuntimeOptions.composeValues()
	require.NoError(t, err)
	got, ok := values.Lookup("capabilities.workspace_enabled")
	require.True(t, ok)
	assert.True(t, got.(bool))
}

type recipeInput struct {
	Query string `prompt:"query"`
}

type recipeLateInput struct {
	Suffix string `prompt:"suffix"`
}

type recipeRequiredLateInput struct {
	Suffix *string `prompt:"suffix" validate:"required"`
}

type recipeComposeContext struct {
	workspaceEnabled bool
}

func (c recipeComposeContext) ComposeValues() ComposeValues {
	return NewComposeValuesFromPairs(
		ComposeBool("capabilities.workspace_enabled", c.workspaceEnabled),
	)
}

type invalidRecipeComposeContext struct{}

func (invalidRecipeComposeContext) ComposeValues() ComposeValues {
	return NewComposeValuesFromPairs()
}

func (invalidRecipeComposeContext) ValidateComposeContext() error {
	return errors.New("compose context requires capabilities.workspace_enabled")
}

type recipeTestRegistry struct {
	t          *testing.T
	desc       ManifestDescriptor
	plannedID  string
	planCalled bool
}

func newRecipeTestRegistry(t *testing.T) *recipeTestRegistry {
	t.Helper()
	return &recipeTestRegistry{
		t:    t,
		desc: ManifestDescriptor{ID: "agent", Digest: "digest"},
	}
}

func (r *recipeTestRegistry) Plan(
	_ context.Context,
	id string,
	input RegistryPlanInput,
) (*RenderPlan, error) {
	r.planCalled = true
	r.plannedID = id
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":  map[string]any{"type": "string"},
				"suffix": map[string]any{"type": "string", "x-prompty-late": true},
			},
		}),
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.query }} {{ .LateVars.suffix }}")},
	}, WithMetadata(PromptMetadata{ID: id}), WithInputSchema(schema))
	require.NoError(r.t, err)
	return NewRenderPlanFromPlanInput(tpl, input)
}

func (r *recipeTestRegistry) ReadManifestBytes(context.Context, string) ([]byte, error) {
	return []byte("manifest"), nil
}

func (r *recipeTestRegistry) RecommendManifestDescriptor(
	context.Context,
	string,
) (ManifestDescriptor, error) {
	return r.desc, nil
}

func (r *recipeTestRegistry) VerifyManifestDescriptor(
	_ context.Context,
	desc ManifestDescriptor,
) error {
	if desc != r.desc {
		return ErrManifestDigestMismatch
	}
	return nil
}
