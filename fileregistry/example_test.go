package fileregistry_test

import (
	"context"
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/parser/yaml"
)

func ExampleRegistry_Plan() {
	dir := "testdata/prompts"
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	plan, err := reg.Plan(ctx, "support_agent", nil)
	if err != nil {
		panic(err)
	}
	tpl := plan.Template()
	fmt.Println(tpl.Metadata.ID)
	fmt.Println(len(tpl.Messages))
	// Output:
	// support_agent
	// 1
}

func ExampleNew() {
	dir := "testdata/prompts"
	reg, err := fileregistry.New(dir, fileregistry.WithParser(yaml.New()))
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	plan, err := reg.Plan(ctx, "support_agent", map[string]any{"user_name": "Alice"})
	if err != nil {
		panic(err)
	}
	exec, err := plan.Execute(ctx)
	if err != nil {
		panic(err)
	}
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	fmt.Println(text)
	// Output:
	// You are a support agent. User: Alice.
}
