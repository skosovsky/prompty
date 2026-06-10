package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPlan_Execute_HistoryPreservesProvenance(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("current {{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "current-agent"}))
	require.NoError(t, err)

	input, err := PlanInputFrom(struct {
		Q           string        `prompt:"q"`
		ChatHistory []ChatMessage `prompt:"chat_history"`
	}{
		Q: "now",
		ChatHistory: []ChatMessage{
			{
				Role:    RoleUser,
				Content: []ContentPart{TextPart{Text: "past"}},
				Provenance: &MessageProvenance{
					ManifestID: "prior-session",
					LayerID:    "user_turn",
				},
			},
		},
	})
	require.NoError(t, err)

	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	require.NotNil(t, exec.Messages[0].Provenance)
	assert.Equal(t, "prior-session", exec.Messages[0].Provenance.ManifestID)
	assert.Equal(t, "user_turn", exec.Messages[0].Provenance.LayerID)
	require.NotNil(t, exec.Messages[1].Provenance)
	assert.Equal(t, "current-agent", exec.Messages[1].Provenance.ManifestID)
}

func TestRenderPlan_Execute_HistoryWithoutProvenanceStaysNil(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("current")},
	}, WithMetadata(PromptMetadata{ID: "current-agent"}))
	require.NoError(t, err)

	input, err := PlanInputFrom(struct {
		ChatHistory []ChatMessage `prompt:"chat_history"`
	}{
		ChatHistory: []ChatMessage{{Role: RoleUser, Content: []ContentPart{TextPart{Text: "past"}}}},
	})
	require.NoError(t, err)

	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Nil(t, exec.Messages[0].Provenance)
	require.NotNil(t, exec.Messages[1].Provenance)
	assert.Equal(t, "current-agent", exec.Messages[1].Provenance.ManifestID)
}

func TestRenderPlan_Execute_RenderedGetsManifestProvenance(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role:    RoleSystem,
			LayerID: "policy",
			Content: TextContent("rules"),
		},
	}, WithMetadata(PromptMetadata{ID: "agent"}))
	require.NoError(t, err)

	exec, err := NewRenderPlan(tpl).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	require.NotNil(t, exec.Messages[0].Provenance)
	assert.Equal(t, "agent", exec.Messages[0].Provenance.ManifestID)
	assert.Equal(t, "policy", exec.Messages[0].Provenance.LayerID)
}
