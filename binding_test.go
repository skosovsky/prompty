package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_bindTemplateVars_MergesPartialsWhenNeeded(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("{{ .Input.bot_name }}: {{ .Input.msg }}")},
	}, MustWithPartialVariablesJSON(MustJSONDocumentFromMap(map[string]any{"bot_name": "Bot", "msg": "default"})))
	require.NoError(t, err)

	type Payload struct {
		Msg string `prompt:"msg"`
	}
	plan, err := NewRenderPlanFromStruct(tpl, &Payload{Msg: "override"})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	text := mustTextFromParts(t, exec.Messages[0].Content)
	assert.Contains(t, text, "Bot")
	assert.Contains(t, text, "override")
}

func Test_bindTemplateVars_RequiresPromptTag(t *testing.T) {
	t.Parallel()
	type Payload struct {
		UserName string
	}
	_, _, err := bindTemplateVars(&Payload{UserName: "Ada"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func TestRenderPlan_RejectNonStructNonMapInput(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hi")},
	})
	require.NoError(t, err)
	_, err = NewRenderPlanFromStruct(tpl, "not-a-struct")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}
