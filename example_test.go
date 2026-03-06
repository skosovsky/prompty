package prompty_test

import (
	"context"
	"fmt"

	"github.com/skosovsky/prompty"
)

func ExampleNewChatPromptTemplate() {
	msgs := []prompty.MessageTemplate{
		{Role: prompty.RoleSystem, Content: prompty.TextContent("You are a helpful assistant.")},
		{Role: prompty.RoleUser, Content: prompty.TextContent("Hello, {{ .user_name }}!")},
	}
	tpl, err := prompty.NewChatPromptTemplate(msgs)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(tpl.Messages))
	// Output: 2
}

func ExampleChatPromptTemplate_FormatStruct() {
	tpl, _ := prompty.NewChatPromptTemplate([]prompty.MessageTemplate{
		{Role: prompty.RoleSystem, Content: prompty.TextContent("Hello, {{ .name }}!")},
	})
	type Payload struct {
		Name string `prompt:"name"`
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{Name: "Alice"})
	if err != nil {
		panic(err)
	}
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	fmt.Println(text)
	// Output: Hello, Alice!
}

func ExampleWithTools() {
	tpl, _ := prompty.NewChatPromptTemplate(
		[]prompty.MessageTemplate{
			{Role: prompty.RoleSystem, Content: prompty.TextContent("Tools: {{ render_tools_as_json .Tools }}")},
		},
		prompty.WithTools([]prompty.ToolDefinition{
			{Name: "get_weather", Description: "Get weather", Parameters: nil},
		}),
	)
	fmt.Println(len(tpl.Tools))
	// Output: 1
}

func Example() {
	tpl, err := prompty.NewChatPromptTemplate(
		[]prompty.MessageTemplate{
			{Role: prompty.RoleSystem, Content: prompty.TextContent("You are {{ .bot_name }}.")},
			{Role: prompty.RoleUser, Content: prompty.TextContent("{{ .query }}")},
		},
		prompty.WithPartialVariables(map[string]any{"bot_name": "HelperBot"}),
	)
	if err != nil {
		panic(err)
	}
	type Payload struct {
		Query string `prompt:"query"`
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{Query: "What is 2+2?"})
	if err != nil {
		panic(err)
	}
	fmt.Println(exec.Messages[0].Content[0].(prompty.TextPart).Text)
	fmt.Println(exec.Messages[1].Content[0].(prompty.TextPart).Text)
	// Output:
	// You are HelperBot.
	// What is 2+2?
}
