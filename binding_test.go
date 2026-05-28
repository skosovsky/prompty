package prompty

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStructTemplateInput_SnakeCaseFieldWithoutTags(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.user_name }}")},
	})
	require.NoError(t, err)

	type Payload struct {
		UserName string
	}
	exec, err := NewRenderPlan(tpl, &Payload{UserName: "Ada"}).Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Ada", TextFromParts(exec.Messages[0].Content))
}

func TestBuildStructTemplateInput_MergesPartialsWhenNeeded(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("{{ .Input.bot_name }}: {{ .Input.msg }}")},
	}, WithPartialVariables(map[string]any{"bot_name": "Bot", "msg": "default"}))
	require.NoError(t, err)

	type Payload struct {
		Msg string `prompt:"msg"`
	}
	exec, err := NewRenderPlan(tpl, &Payload{Msg: "override"}).Execute(context.Background())
	require.NoError(t, err)
	text := TextFromParts(exec.Messages[0].Content)
	assert.Contains(t, text, "Bot")
	assert.Contains(t, text, "override")
}

func TestFieldAliases_PromptOverridesJsonAndSnakeCase(t *testing.T) {
	t.Parallel()
	type Payload struct {
		UserName string `json:"json_name" prompt:"display_name"`
	}
	binding, err := getStructBinding(reflect.TypeFor[Payload]())
	require.NoError(t, err)
	require.Len(t, binding.fields, 1)
	assert.Equal(t, "display_name", binding.fields[0].alias)
	_, hasPrompt := binding.aliasToIdx["display_name"]
	_, hasJSON := binding.aliasToIdx["json_name"]
	_, hasSnake := binding.aliasToIdx["user_name"]
	assert.True(t, hasPrompt)
	assert.True(t, hasJSON)
	assert.True(t, hasSnake)

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.display_name }} / {{ .Input.json_name }}")},
	})
	require.NoError(t, err)
	exec, err := NewRenderPlan(tpl, &Payload{UserName: "Ada"}).Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Ada / Ada", TextFromParts(exec.Messages[0].Content))
}

func TestRenderPlan_RejectNonStructNonMapInput(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hi")},
	})
	require.NoError(t, err)
	_, err = NewRenderPlan(tpl, "not-a-struct").Execute(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}
