package embedregistry

import (
	"context"
	"embed"
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

//go:embed testdata/prompts/*.json
var exampleFS embed.FS

func ExampleNew() {
	reg, err := New(exampleFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	tpl, err := templateFromPlan(ctx, reg, "agent")
	if err != nil {
		panic(err)
	}
	fmt.Println(tpl.Metadata.ID)
	fmt.Println(len(tpl.Messages))
	// Output:
	// agent
	// 1
}

func ExampleRegistry_Plan() {
	reg, err := New(exampleFS, "testdata/prompts", WithParser(manifest.NewJSONParser()))
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	tpl, err := templateFromPlan(ctx, reg, "agent")
	if err != nil {
		panic(err)
	}
	exec, err := executeTemplatePlan(tpl, map[string]any{"user_name": "Bob"})
	if err != nil {
		panic(err)
	}
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	fmt.Println(text)
	// Output:
	// Agent Bob
}
