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

	plan := newRenderPlanFromMap(tpl, map[string]any{"name": "Alice"})
	plan, err = plan.WithLateInput(struct {
		Suffix string `prompt:"suffix"`
	}{Suffix: "!"})
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "hello Alice !", mustTextFromParts(t, exec.Messages[0].Content))
}

func TestRenderPlan_Execute_DoesNotCollapseConsecutiveSystemMessages(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("A")},
		{Role: RoleDeveloper, Content: TextContent("B")},
		{Role: RoleUser, Content: TextContent("U")},
	})
	require.NoError(t, err)

	exec, err := NewRenderPlan(tpl).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, "A", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, RoleDeveloper, exec.Messages[1].Role)
	assert.Equal(t, "B", mustTextFromParts(t, exec.Messages[1].Content))
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

	exec, err := NewRenderPlan(tpl).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	require.NotNil(t, exec.Messages[0].Provenance)
	assert.Equal(t, "policy", exec.Messages[0].Provenance.LayerID)
	assert.Equal(t, "base-manifest", exec.Messages[0].Provenance.ManifestID)
	assert.Equal(t, "base-manifest", exec.Metadata.ID)
}

func TestRenderPlan_WithLateInput_RejectsEarlyField(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_query": map[string]any{"type": "string"},
				"extra_ctx":  map[string]any{"type": "string"},
			},
		}),
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.user_query }} {{ .LateVars.extra_ctx }}")},
	}, WithInputSchema(schema))
	require.NoError(t, err)

	plan := newRenderPlanFromMap(tpl, map[string]any{"user_query": "hi"})
	_, err = plan.WithLateInput(struct {
		ExtraCtx string `prompt:"extra_ctx"`
	}{ExtraCtx: "late"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "early-bound")
}

func TestRenderPlan_WithLateInput_AcceptsLateField(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_query":      map[string]any{"type": "string"},
				"patient_dossier": map[string]any{"type": "string", "x-prompty-late": true},
			},
		}),
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.user_query }} {{ .LateVars.patient_dossier }}")},
	}, WithInputSchema(schema))
	require.NoError(t, err)

	plan := newRenderPlanFromMap(tpl, map[string]any{"user_query": "hi"})
	plan, err = plan.WithLateInput(struct {
		PatientDossier string `prompt:"patient_dossier"`
	}{PatientDossier: "chart"})
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "hi chart", mustTextFromParts(t, exec.Messages[0].Content))
}

func TestRenderPlan_WithLateInput_RejectsValidateTags(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_query": map[string]any{"type": "string"},
				"patient_dossier": map[string]any{
					"type":           "string",
					"x-prompty-late": true,
				},
			},
		}),
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.user_query }} {{ .LateVars.patient_dossier }}")},
	}, WithInputSchema(schema))
	require.NoError(t, err)

	plan := newRenderPlanFromMap(tpl, map[string]any{"user_query": "hi"})
	_, err = plan.WithLateInput(struct {
		PatientDossier string `prompt:"patient_dossier" validate:"required"`
	}{PatientDossier: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input")
}

func TestRenderPlan_WithLateInput_RejectsValidateTagsPointer(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_query": map[string]any{"type": "string"},
				"patient_dossier": map[string]any{
					"type":           "string",
					"x-prompty-late": true,
				},
			},
		}),
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.user_query }} {{ .LateVars.patient_dossier }}")},
	}, WithInputSchema(schema))
	require.NoError(t, err)

	plan := newRenderPlanFromMap(tpl, map[string]any{"user_query": "hi"})
	type latePayload struct {
		PatientDossier string `prompt:"patient_dossier" validate:"required"`
	}
	payload := latePayload{PatientDossier: ""}
	_, err = plan.WithLateInput(&payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "late input")
}

func TestRenderPlan_WithLateInput_RejectsChatHistoryInLatePayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	})
	require.NoError(t, err)

	plan := newRenderPlanFromMap(tpl, map[string]any{"q": "hi"})
	_, err = plan.WithLateInput(struct {
		ChatHistory []ChatMessage `prompt:"chat_history"`
	}{
		ChatHistory: []ChatMessage{{
			Role:    RoleUser,
			Content: []ContentPart{TextPart{Text: "prior"}},
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat history must not be bound as late variables")
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

	plan := newRenderPlanFromMap(tpl, map[string]any{"q": "x"})
	plan, err = WithResponseFormatFromStruct[Out](plan)
	require.NoError(t, err)

	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.NotNil(t, exec.ResponseFormat)
	require.NotNil(t, exec.ResponseFormat.Schema)
}
