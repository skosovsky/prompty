package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPlan_AppendToLayer(t *testing.T) {
	t.Parallel()
	base, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerID: "policy", Content: TextContent("base")},
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	})
	require.NoError(t, err)

	appendTpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("extra")},
	})
	require.NoError(t, err)

	plan, err := NewRenderPlan(base, map[string]any{"q": "go"}).
		AppendToLayer("policy", NewRenderPlan(appendTpl, nil))
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "base", TextFromParts(exec.Messages[0].Content))
	assert.Equal(t, "extra", TextFromParts(exec.Messages[1].Content))
	assert.Equal(t, "go", TextFromParts(exec.Messages[2].Content))
}

func TestRenderPlan_WithResponseFormat_RuntimeOverride(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	})
	require.NoError(t, err)

	type Out struct {
		Answer string `json:"answer"`
	}

	plan, err := NewRenderPlan(tpl, map[string]any{"q": "x"}).
		WithResponseFormat(Out{})
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.NotNil(t, exec.ResponseFormat)
	require.NotNil(t, exec.ResponseFormat.Schema)
}

func TestRenderPlan_WithResponseFormat_RuntimeOverridesManifest(t *testing.T) {
	t.Parallel()
	manifestSchema := &SchemaDefinition{
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from_manifest": map[string]any{"type": "string"},
			},
		},
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithResponseFormat(manifestSchema))
	require.NoError(t, err)

	type RuntimeOut struct {
		Answer string `json:"answer"`
	}
	plan, err := NewRenderPlan(tpl, map[string]any{"q": "x"}).WithResponseFormat(RuntimeOut{})
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	props, _ := exec.ResponseFormat.Schema["properties"].(map[string]any)
	_, hasManifest := props["from_manifest"]
	_, hasRuntime := props["answer"]
	assert.False(t, hasManifest)
	assert.True(t, hasRuntime)
}

func TestRenderPlan_AppendToLayer_Provenance(t *testing.T) {
	t.Parallel()
	base, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerID: "policy", Content: TextContent("base")},
	}, WithMetadata(PromptMetadata{ID: "base-manifest"}))
	require.NoError(t, err)

	appendTpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("extra")},
	}, WithMetadata(PromptMetadata{ID: "append-manifest"}))
	require.NoError(t, err)

	plan, err := NewRenderPlan(base, nil).AppendToLayer("policy", NewRenderPlan(appendTpl, nil))
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(exec.Messages), 2)
	assert.Equal(t, "base-manifest", exec.Messages[0].ManifestID)
	assert.Equal(t, LayerRef{LayerID: "policy", ManifestID: "base-manifest"}, exec.Messages[0].LayerRef)
	assert.Equal(t, "append-manifest", exec.Messages[1].ManifestID)
}

func TestRenderPlan_WithResponseFormat_Errors(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	})
	require.NoError(t, err)
	base := NewRenderPlan(tpl, map[string]any{"q": "x"})

	_, err = (*RenderPlan)(nil).WithResponseFormat(struct{}{})
	require.ErrorIs(t, err, ErrNilRenderPlan)

	_, err = base.WithResponseFormat(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema is required")

	_, err = base.WithResponseFormat(make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema type")
}

func TestRenderPlan_ReplaceLayer_Errors(t *testing.T) {
	t.Parallel()
	base, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerID: "policy", LayerKind: LayerKind("policy"), Content: TextContent("x")},
	})
	require.NoError(t, err)
	override, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerKind: LayerKind("other"), Content: TextContent("y")},
	})
	require.NoError(t, err)

	_, err = NewRenderPlan(base, nil).ReplaceLayer("", NewRenderPlan(override, nil))
	require.Error(t, err)

	_, err = NewRenderPlan(base, nil).ReplaceLayer("missing", NewRenderPlan(override, nil))
	require.Error(t, err)

	_, err = NewRenderPlan(base, nil).ReplaceLayer("policy", NewRenderPlan(override, nil))
	require.Error(t, err)
}
