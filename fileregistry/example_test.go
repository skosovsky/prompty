package fileregistry_test

import (
	"context"
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/fileregistry"
)

func ExampleRegistry_GetTemplate() {
	dir := "testdata/prompts"
	reg := fileregistry.New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	if err != nil {
		panic(err)
	}
	fmt.Println(tpl.Metadata.ID)
	fmt.Println(len(tpl.Messages))
	// Output:
	// support_agent
	// 1
}

func ExampleNew() {
	dir := "testdata/prompts"
	reg := fileregistry.New(dir)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	if err != nil {
		panic(err)
	}
	type Payload struct {
		UserName string `prompt:"user_name"`
	}
	exec, err := tpl.FormatStruct(ctx, &Payload{UserName: "Alice"})
	if err != nil {
		panic(err)
	}
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	fmt.Println(text)
	// Output:
	// You are a support agent. User: Alice.
}
