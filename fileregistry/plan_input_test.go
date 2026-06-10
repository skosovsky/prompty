package fileregistry

import (
	"context"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileregistry_PlanInputFrom_NilOptionals(t *testing.T) {
	t.Parallel()
	reg, err := New("testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)
	ctx := context.Background()
	input, err := prompty.PlanInputFrom(struct {
		Flag *bool     `json:"flag,omitempty" prompt:"flag"`
		Tags *[]string `json:"tags,omitempty" prompt:"tags"`
	}{})
	require.NoError(t, err)
	plan, err := reg.Plan(ctx, "nil_optionals", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "flag=false tags=0", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestPlanInputFrom_ChatHistory_FormatMessages(t *testing.T) {
	t.Parallel()
	reg, err := New("testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)
	ctx := context.Background()
	history := []prompty.ChatMessage{
		{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "prior"}}},
	}
	input, err := prompty.PlanInputFrom(struct {
		Query       string                `prompt:"query"`
		ChatHistory []prompty.ChatMessage `prompt:"chat_history"`
	}{Query: "now", ChatHistory: history})
	require.NoError(t, err)
	plan, err := reg.Plan(ctx, "chat_history_messages", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "prior", exec.Messages[1].Content[0].(prompty.TextPart).Text)
	assert.Equal(t, "now", exec.Messages[2].Content[0].(prompty.TextPart).Text)
}

func TestPlanInputFrom_ChatHistory_FormatMessages_PreservesProvenance(t *testing.T) {
	t.Parallel()
	reg, err := New("testdata/prompts", WithParser(yaml.New()))
	require.NoError(t, err)
	ctx := context.Background()
	history := []prompty.ChatMessage{
		{
			Role:       prompty.RoleUser,
			Content:    []prompty.ContentPart{prompty.TextPart{Text: "prior"}},
			Provenance: &prompty.MessageProvenance{ManifestID: "history-src", LayerID: "turn_1"},
		},
	}
	input, err := prompty.PlanInputFrom(struct {
		Query       string                `prompt:"query"`
		ChatHistory []prompty.ChatMessage `prompt:"chat_history"`
	}{Query: "now", ChatHistory: history})
	require.NoError(t, err)
	plan, err := reg.Plan(ctx, "chat_history_messages", input)
	require.NoError(t, err)
	exec, err := plan.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	require.NotNil(t, exec.Messages[1].Provenance)
	assert.Equal(t, "history-src", exec.Messages[1].Provenance.ManifestID)
	assert.Equal(t, "turn_1", exec.Messages[1].Provenance.LayerID)
	require.NotNil(t, exec.Messages[0].Provenance)
	assert.Equal(t, "chat_history_messages", exec.Messages[0].Provenance.ManifestID)
}
