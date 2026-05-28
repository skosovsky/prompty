package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPlan_Execute_UsesInputAndLateVars(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role:    RoleUser,
			Content: TextContent("hello {{ .Input.name }} {{ .LateVars.suffix }}"),
		},
	})
	require.NoError(t, err)

	plan := NewRenderPlan(tpl, map[string]any{"name": "Alice"}).
		WithLateVariables(map[string]any{"suffix": "!"})

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "hello Alice !", TextFromParts(exec.Messages[0].Content))
}

func TestRenderPlan_ReplaceLayer(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role:      RoleSystem,
			LayerID:   "s1",
			LayerKind: LayerKind("policy"),
			Content:   TextContent("base"),
		},
		{
			Role:    RoleUser,
			Content: TextContent("u"),
		},
	})
	require.NoError(t, err)

	overrideTemplate, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerKind: LayerKind("policy"), Content: TextContent("override")},
	})
	require.NoError(t, err)

	plan, err := NewRenderPlan(tpl, nil).ReplaceLayer("s1", NewRenderPlan(overrideTemplate, nil))
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "override", TextFromParts(exec.Messages[0].Content))
	assert.Equal(t, RoleUser, exec.Messages[1].Role)
}

func TestRenderPlan_Execute_CollapsesConsecutiveSystemMessages(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("A")},
		{Role: RoleDeveloper, Content: TextContent("B")},
		{Role: RoleUser, Content: TextContent("U")},
	})
	require.NoError(t, err)

	exec, err := NewRenderPlan(tpl, nil).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, "A", TextFromParts(exec.Messages[0].Content))
	assert.Equal(t, RoleDeveloper, exec.Messages[1].Role)
	assert.Equal(t, "B", TextFromParts(exec.Messages[1].Content))
}

func TestRenderPlan_Execute_SetsProvenance(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role:      RoleSystem,
			LayerID:   "policy",
			LayerKind: LayerKind("policy"),
			Content:   TextContent("rules"),
		},
	}, WithMetadata(PromptMetadata{ID: "base-manifest"}))
	require.NoError(t, err)

	exec, err := NewRenderPlan(tpl, nil).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, LayerRef{LayerID: "policy", ManifestID: "base-manifest"}, exec.Messages[0].LayerRef)
	assert.Equal(t, "base-manifest", exec.Messages[0].ManifestID)
	assert.Equal(t, "base-manifest", exec.Metadata.ID)
}

func TestRenderPlan_ReplaceLayer_ReplacesContiguousSegmentOnce(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerID: "policy", LayerKind: LayerKind("policy"), Content: TextContent("p1")},
		{Role: RoleSystem, LayerID: "policy", LayerKind: LayerKind("policy"), Content: TextContent("p2")},
		{Role: RoleUser, Content: TextContent("u")},
	})
	require.NoError(t, err)

	overrideTemplate, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, LayerKind: LayerKind("policy"), Content: TextContent("override")},
	})
	require.NoError(t, err)

	plan, err := NewRenderPlan(tpl, nil).ReplaceLayer("policy", NewRenderPlan(overrideTemplate, nil))
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "override", TextFromParts(exec.Messages[0].Content))
	assert.Equal(t, RoleUser, exec.Messages[1].Role)
}
