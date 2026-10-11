package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_bindTemplateVars_NilOptionalBool(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("flag={{if eq .Input.flag nil}}null{{else}}{{.Input.flag}}{{end}}")},
	})
	require.NoError(t, err)
	type Payload struct {
		Flag *bool `prompt:"flag"`
	}
	plan, err := NewRenderPlanFromStruct(tpl, &Payload{})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	vars, _, bindErr := bindTemplateVars(&Payload{})
	require.NoError(t, bindErr)
	require.Contains(t, vars, "flag")
	assert.Nil(t, vars["flag"])
	assert.Equal(t, "flag=null", mustTextFromParts(t, exec.Messages[0].Content))
}

func Test_bindTemplateVars_NilOptionalSlice(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ range .Input.tags }}x{{ end }}done")},
	})
	require.NoError(t, err)
	type Payload struct {
		Tags *[]string `prompt:"tags"`
	}
	plan, err := NewRenderPlanFromStruct(tpl, &Payload{})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	vars, _, bindErr := bindTemplateVars(&Payload{})
	require.NoError(t, bindErr)
	require.Contains(t, vars, "tags")
	assert.Nil(t, vars["tags"])
	assert.Equal(t, "done", mustTextFromParts(t, exec.Messages[0].Content))
}

func Test_bindTemplateVars_RecursiveNestedNilPointer(t *testing.T) {
	t.Parallel()
	type UserDTO struct {
		FirstName string `prompt:"first_name"`
	}
	type Payload struct {
		User *UserDTO `prompt:"user"`
	}
	vars, _, err := bindTemplateVars(&Payload{})
	require.NoError(t, err)
	require.Contains(t, vars, "user")
	assert.Nil(t, vars["user"])

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role:    RoleUser,
			Content: TextContent("{{if eq .Input.user nil}}null{{else}}{{.Input.user.first_name}}{{end}}"),
		},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromStruct(tpl, &Payload{})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages[0].Content, 1)
	text, ok := exec.Messages[0].Content[0].(TextPart)
	require.True(t, ok)
	assert.Equal(t, "null", text.Text)
}

func TestPlanInputFrom_EmptyRegistryPlanInput(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("{{ .Input.bot_name }}")},
	}, MustWithPartialVariablesJSON(MustJSONDocumentFromMap(map[string]any{"bot_name": "Bot"})))
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, RegistryPlanInput{})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bot", mustTextFromParts(t, exec.Messages[0].Content))
}

func TestPlanInputFrom_HistoryNotInBoundVars(t *testing.T) {
	t.Parallel()
	history := []ChatMessage{NewUserMessage("prior")}
	type Payload struct {
		Query   string        `prompt:"query"`
		History []ChatMessage `prompt:"history"`
	}
	input, err := PlanInputFrom(Payload{Query: "hi", History: history})
	require.NoError(t, err)
	_, hasHistory := input.boundVars["history"]
	assert.False(t, hasHistory)
	require.Len(t, input.chatHistory, 1)

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.query }}")},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "hi", mustTextFromParts(t, exec.Messages[1].Content))
	assert.Equal(t, "prior", mustTextFromParts(t, exec.Messages[0].Content))
}

func Test_bindTemplateVars_JsonOmitemptyDoesNotAffectBinding(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: TextContent(
				"flag={{if eq .Input.flag nil}}null{{else}}{{.Input.flag}}{{end}} tags={{if eq .Input.tags nil}}null{{else}}{{len .Input.tags}}{{end}}",
			),
		},
	})
	require.NoError(t, err)
	type Payload struct {
		Flag *bool     `json:"flag,omitempty" prompt:"flag"`
		Tags *[]string `json:"tags,omitempty" prompt:"tags"`
	}
	plan, err := NewRenderPlanFromStruct(tpl, &Payload{})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "flag=null tags=null", mustTextFromParts(t, exec.Messages[0].Content))
}

func Test_bindTemplateVars_PromptTagOverridesJson(t *testing.T) {
	t.Parallel()
	type Payload struct {
		Value string `json:"wrong" prompt:"right"`
	}
	vars, _, err := bindTemplateVars(Payload{Value: "v"})
	require.NoError(t, err)
	require.Contains(t, vars, "right")
	assert.Equal(t, "v", vars["right"])
	_, hasWrong := vars["wrong"]
	assert.False(t, hasWrong)
}

func Test_bindTemplateVars_InterfaceNilPointer(t *testing.T) {
	t.Parallel()
	type Payload struct {
		Flag any `prompt:"flag"`
	}
	vars, _, err := bindTemplateVars(&Payload{Flag: (*bool)(nil)})
	require.NoError(t, err)
	require.Contains(t, vars, "flag")
	assert.Nil(t, vars["flag"])
}

func Test_bindTemplateVars_InterfaceFieldUnset(t *testing.T) {
	t.Parallel()
	type Payload struct {
		Flag any `prompt:"flag"`
	}
	vars, _, err := bindTemplateVars(Payload{})
	require.NoError(t, err)
	require.Contains(t, vars, "flag")
	assert.Nil(t, vars["flag"])
}

func TestPlanInputFrom_HistoryPointerElements(t *testing.T) {
	t.Parallel()
	msg := NewUserMessage("prior")
	history := []*ChatMessage{&msg}
	type Payload struct {
		Query   string         `prompt:"query"`
		History []*ChatMessage `prompt:"history"`
	}
	input, err := PlanInputFrom(Payload{Query: "hi", History: history})
	require.NoError(t, err)
	require.Len(t, input.chatHistory, 1)
	_, hasHistory := input.boundVars["history"]
	assert.False(t, hasHistory)

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.query }}")},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "prior", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, "hi", mustTextFromParts(t, exec.Messages[1].Content))
}

func Test_bindTemplateVars_DuplicatePromptAliasFails(t *testing.T) {
	t.Parallel()
	type Payload struct {
		A string `prompt:"x"`
		B string `prompt:"x"`
	}
	_, _, err := bindTemplateVars(Payload{A: "1", B: "2"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func TestPlanInputFrom_HistoryPointerSlice(t *testing.T) {
	t.Parallel()
	history := []ChatMessage{NewUserMessage("prior")}
	type Payload struct {
		Query   string         `prompt:"query"`
		History *[]ChatMessage `prompt:"history"`
	}
	input, err := PlanInputFrom(Payload{Query: "hi", History: &history})
	require.NoError(t, err)
	_, hasHistory := input.boundVars["history"]
	assert.False(t, hasHistory)
	require.Len(t, input.chatHistory, 1)

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.query }}")},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "prior", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, "hi", mustTextFromParts(t, exec.Messages[1].Content))
}

func TestPlanInputFrom_HistoryTypeAlias(t *testing.T) {
	t.Parallel()
	type History []ChatMessage
	history := History{NewUserMessage("prior")}
	type Payload struct {
		Query   string  `prompt:"query"`
		History History `prompt:"history"`
	}
	input, err := PlanInputFrom(Payload{Query: "hi", History: history})
	require.NoError(t, err)
	require.Len(t, input.chatHistory, 1)
	_, hasHistory := input.boundVars["history"]
	assert.False(t, hasHistory)

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.query }}")},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "prior", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, "hi", mustTextFromParts(t, exec.Messages[1].Content))
}

func TestPlanInputFrom_HistorySpliceViaPlanInputFrom(t *testing.T) {
	t.Parallel()
	history := []ChatMessage{NewUserMessage("prior")}
	type Payload struct {
		Query   string        `prompt:"query"`
		History []ChatMessage `prompt:"history"`
	}
	input, err := PlanInputFrom(Payload{Query: "last", History: history})
	require.NoError(t, err)
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("sys")},
		{Role: RoleDeveloper, Content: TextContent("dev")},
		{Role: RoleUser, Content: TextContent("{{ .Input.query }}")},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 4)
	assert.Equal(t, "sys", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, "dev", mustTextFromParts(t, exec.Messages[1].Content))
	assert.Equal(t, "prior", mustTextFromParts(t, exec.Messages[2].Content))
	assert.Equal(t, "last", mustTextFromParts(t, exec.Messages[3].Content))
}

func Test_bindTemplateVars_NestedTwoLevelsNil(t *testing.T) {
	t.Parallel()
	type Inner struct {
		X string `prompt:"x"`
	}
	type Middle struct {
		Inner *Inner `prompt:"inner"`
	}
	type Payload struct {
		Middle *Middle `prompt:"middle"`
	}
	vars, _, err := bindTemplateVars(&Payload{})
	require.NoError(t, err)
	require.Contains(t, vars, "middle")
	assert.Nil(t, vars["middle"])
}

func Test_bindTemplateVars_BindSliceOfPromptStructs(t *testing.T) {
	t.Parallel()
	type Item struct {
		Name string `prompt:"name"`
	}
	type Payload struct {
		Items []Item `prompt:"items"`
	}
	vars, _, err := bindTemplateVars(Payload{Items: []Item{{Name: "a"}, {Name: "b"}}})
	require.NoError(t, err)
	items, ok := vars["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	assert.Equal(t, "a", items[0]["name"])
	assert.Equal(t, "b", items[1]["name"])
}

func Test_bindTemplateVars_NilOptionalsNeverErrMissingVariable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
		payload any
	}{
		{
			name:    "nil_bool",
			content: "flag={{ .Input.flag }}",
			payload: &struct {
				Flag *bool `prompt:"flag"`
			}{},
		},
		{
			name:    "nil_slice",
			content: "{{ range .Input.tags }}x{{ end }}done",
			payload: &struct {
				Tags *[]string `prompt:"tags"`
			}{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tpl, err := NewChatPromptTemplate([]MessageTemplate{
				{Role: RoleUser, Content: TextContent(tc.content)},
			})
			require.NoError(t, err)
			plan, err := NewRenderPlanFromStruct(tpl, tc.payload)
			require.NoError(t, err)
			_, err = plan.Execute(context.Background())
			require.NoError(t, err)
		})
	}
}

func Test_bindTemplateVars_NilOptionalScalars(t *testing.T) {
	t.Parallel()
	type Payload struct {
		Label *string `prompt:"label"`
		Count *int    `prompt:"count"`
	}
	vars, _, err := bindTemplateVars(&Payload{})
	require.NoError(t, err)
	require.Contains(t, vars, "label")
	require.Contains(t, vars, "count")
	assert.Nil(t, vars["label"])
	assert.Nil(t, vars["count"])

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: TextContent(
				"{{if eq .Input.label nil}}null{{else}}{{.Input.label}}{{end}}:{{if eq .Input.count nil}}null{{else}}{{.Input.count}}{{end}}",
			),
		},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromStruct(tpl, &Payload{})
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "null:null", mustTextFromParts(t, exec.Messages[0].Content))
}

func Test_bindTemplateVars_NestedStructWithoutPromptFields(t *testing.T) {
	t.Parallel()
	type Inner struct {
		X string
	}
	type Payload struct {
		Inner Inner `prompt:"inner"`
	}
	_, _, err := bindTemplateVars(Payload{Inner: Inner{X: "x"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func Test_bindTemplateVars_AnonymousEmbeddedStructFails(t *testing.T) {
	t.Parallel()
	type Inner struct {
		FirstName string `prompt:"first_name"`
	}
	type Payload struct {
		Inner
	}
	_, _, err := bindTemplateVars(Payload{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func TestPlanInputFrom_HistoryOnlyPayload(t *testing.T) {
	t.Parallel()
	history := []ChatMessage{NewUserMessage("prior")}
	type Payload struct {
		History []ChatMessage `prompt:"history"`
	}
	input, err := PlanInputFrom(Payload{History: history})
	require.NoError(t, err)
	require.Len(t, input.chatHistory, 1)

	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("sys")},
	})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	assert.Equal(t, "sys", mustTextFromParts(t, exec.Messages[0].Content))
	assert.Equal(t, "prior", mustTextFromParts(t, exec.Messages[1].Content))
}

func TestPlanInputFrom_WrapsBindError(t *testing.T) {
	t.Parallel()
	_, err := PlanInputFrom("not-a-struct")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
	assert.Contains(t, err.Error(), "plan input:")
}
