package prompty_test

import (
	"context"
	"fmt"
	"iter"

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
	exec, err := tpl.FormatStruct(&Payload{Name: "Alice"})
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
	exec, err := tpl.FormatStruct(&Payload{Query: "What is 2+2?"})
	if err != nil {
		panic(err)
	}
	fmt.Println(exec.Messages[0].Content[0].(prompty.TextPart).Text)
	fmt.Println(exec.Messages[1].Content[0].(prompty.TextPart).Text)
	// Output:
	// You are HelperBot.
	// What is 2+2?
}

type exampleStructuredInvoker struct{}

func (exampleStructuredInvoker) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	return prompty.NewResponse([]prompty.ContentPart{
		prompty.TextPart{Text: `{"name":"Alice"}`},
	}), nil
}

func (exampleStructuredInvoker) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		resp, err := exampleStructuredInvoker{}.Execute(ctx, exec)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(&prompty.ResponseChunk{Content: resp.Content, IsFinished: true}, nil)
	}
}

func ExampleGenerateStructured() {
	type Result struct {
		Name string `json:"name"`
	}

	result, err := prompty.GenerateStructured[Result](context.Background(), exampleStructuredInvoker{}, "Extract name")
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Name)
	// Output: Alice
}
