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
	input, err := prompty.PlanInputFrom(struct {
		UserName string `prompt:"user_name"`
	}{UserName: "Bob"})
	if err != nil {
		panic(err)
	}
	plan, err := reg.Plan(ctx, "agent", input)
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
	// Agent Bob
}
