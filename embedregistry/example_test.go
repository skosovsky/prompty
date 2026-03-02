package embedregistry

import (
	"context"
	"embed"
	"fmt"

	"github.com/skosovsky/prompty"
)

//go:embed testdata/prompts/*.yaml
var exampleFS embed.FS

func ExampleNew() {
	reg, err := New(exampleFS, "testdata/prompts")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	if err != nil {
		panic(err)
	}
	fmt.Println(tpl.Metadata.ID)
	fmt.Println(len(tpl.Messages))
	// Output:
	// agent
	// 1
}

func ExampleRegistry_GetTemplate() {
	reg, err := New(exampleFS, "testdata/prompts")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "agent")
	if err != nil {
		panic(err)
	}
	type Payload struct {
		UserName string `prompt:"user_name"`
	}
	exec, err := tpl.FormatStruct(ctx, &Payload{UserName: "Bob"})
	if err != nil {
		panic(err)
	}
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	fmt.Println(text)
	// Output:
	// Agent Bob
}
