package prompty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRecipeCheckpoint_JSONSafeDTO(t *testing.T) {
	t.Parallel()
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: ManifestDescriptor{ID: "agent", Digest: "digest"},
		Input:      MustJSONDocumentFromMap(map[string]any{"query": "hello"}),
		LateInput:  MustJSONDocumentFromMap(map[string]any{"suffix": "world"}),
		State:      RuntimeRecipeStateLateBound,
	}

	data, err := json.Marshal(checkpoint)
	require.NoError(t, err)
	var restored RuntimeRecipeCheckpoint
	err = json.Unmarshal(data, &restored)

	require.NoError(t, err)
	assert.Equal(t, checkpoint.Descriptor, restored.Descriptor)
	assert.JSONEq(t, `{"query":"hello"}`, string(restored.Input))
	assert.NotContains(t, string(data), "RenderPlan")
	assert.NotContains(t, string(data), "registry")
}

func TestBindRuntime_AppliesOverlayAndToolScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := newRecipeTestRegistry(t)
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: reg.desc,
		Input:      MustJSONDocumentFromMap(map[string]any{"query": "hello"}),
		State:      RuntimeRecipeStateLateUnbound,
	}
	overlay := RuntimeOverlay{
		LateInput: MustJSONDocumentFromMap(map[string]any{"suffix": "world"}),
	}
	scope := ToolScope{
		Required: []ToolRequirement{{Name: "lookup"}},
		Allowed:  []ToolManifest{{Name: "lookup"}},
	}

	plan, err := BindRuntime(ctx, reg, checkpoint, overlay, scope)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)

	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "hello world", mustTextFromParts(t, exec.Messages[0].Content))
}

func TestBindRuntime_VerifiesDescriptorBeforePlanning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := newRecipeTestRegistry(t)
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: ManifestDescriptor{ID: reg.desc.ID, Digest: "stale"},
		Input:      MustJSONDocumentFromMap(map[string]any{"query": "hello"}),
		State:      RuntimeRecipeStateNoLate,
	}

	_, err := BindRuntime(ctx, reg, checkpoint, RuntimeOverlay{}, ToolScope{})

	require.ErrorIs(t, err, ErrManifestDigestMismatch)
	assert.False(t, reg.planCalled)
}

func TestBindRuntime_RejectsMissingRequiredLateInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := &runtimeRequiredLateRegistry{recipeTestRegistry: *newRecipeTestRegistry(t)}
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: reg.desc,
		Input:      MustJSONDocumentFromMap(map[string]any{"query": "hello"}),
		State:      RuntimeRecipeStateLateUnbound,
	}

	_, err := BindRuntime(ctx, reg, checkpoint, RuntimeOverlay{}, ToolScope{})

	require.ErrorIs(t, err, ErrMissingVariable)
	assert.Contains(t, err.Error(), "suffix")
}

func TestBindRuntime_AttachedToolScopeFailsClosedOnExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := &runtimeRequiredToolRegistry{recipeTestRegistry: *newRecipeTestRegistry(t)}
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: reg.desc,
		Input:      MustJSONDocumentFromMap(map[string]any{"query": "hello"}),
		LateInput:  MustJSONDocumentFromMap(map[string]any{"suffix": "world"}),
		State:      RuntimeRecipeStateLateBound,
	}
	scope := ToolScope{
		Allowed: []ToolManifest{{Name: "search"}},
	}

	plan, err := BindRuntime(ctx, reg, checkpoint, RuntimeOverlay{}, scope)
	require.NoError(t, err)
	_, err = plan.Execute(ctx)

	require.ErrorIs(t, err, ErrMissingRequiredTool)
	assert.Contains(t, err.Error(), "lookup")
}

func TestBindRuntime_EmptyToolScopeFailsClosedForRequiredTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := &runtimeRequiredToolRegistry{recipeTestRegistry: *newRecipeTestRegistry(t)}
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: reg.desc,
		Input:      MustJSONDocumentFromMap(map[string]any{"query": "hello"}),
		LateInput:  MustJSONDocumentFromMap(map[string]any{"suffix": "world"}),
		State:      RuntimeRecipeStateLateBound,
	}

	plan, err := BindRuntime(ctx, reg, checkpoint, RuntimeOverlay{}, ToolScope{})
	require.NoError(t, err)
	_, err = plan.Execute(ctx)

	require.ErrorIs(t, err, ErrMissingRequiredTool)
	assert.Contains(t, err.Error(), "agent@digest")
	assert.Contains(t, err.Error(), "lookup")
}

type runtimeRequiredLateRegistry struct {
	recipeTestRegistry
}

func (r *runtimeRequiredLateRegistry) Plan(
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
			"required": []any{"query", "suffix"},
		}),
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.query }} {{ .LateVars.suffix }}")},
	}, WithMetadata(PromptMetadata{ID: id}), WithInputSchema(schema))
	require.NoError(r.t, err)
	return NewRenderPlanFromPlanInput(tpl, input)
}

type runtimeRequiredToolRegistry struct {
	recipeTestRegistry
}

func (r *runtimeRequiredToolRegistry) Plan(
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
	}, WithMetadata(PromptMetadata{ID: id}), WithInputSchema(schema), WithRequiredTools([]string{"lookup"}))
	require.NoError(r.t, err)
	return NewRenderPlanFromPlanInput(tpl, input)
}
