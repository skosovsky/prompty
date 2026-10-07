// Direct BYOT example. No manifest or generator is required.
// Run: OPENAI_MODEL=<host-selected-model> OPENAI_API_KEY=<key> go run ./examples/basic_chat/direct
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

type Question struct {
	Text string `prompt:"question"`
}

func materialize(ctx context.Context, question Question) (*prompty.PromptExecution, error) {
	tpl, err := prompty.NewChatPromptTemplate([]prompty.MessageTemplate{
		{Role: prompty.RoleSystem, Content: prompty.TextContent("Answer briefly.")},
		{Role: prompty.RoleUser, Content: prompty.TextContent("{{ .Input.question }}")},
	})
	if err != nil {
		return nil, err
	}
	input, err := prompty.PlanInputFrom(question)
	if err != nil {
		return nil, err
	}
	plan, err := prompty.NewRenderPlanFromPlanInput(tpl, input)
	if err != nil {
		return nil, err
	}
	return plan.Execute(ctx)
}

func main() {
	key, model := os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_MODEL")
	if key == "" || model == "" {
		log.Fatal("set OPENAI_API_KEY and OPENAI_MODEL to a host-selected model")
	}
	ctx := context.Background()
	execution, err := materialize(ctx, Question{Text: "What is 2+2?"})
	if err != nil {
		log.Fatal(err)
	}
	const endpoint = "https://api.openai.com/v1"
	sdk := openaisdk.NewClient(option.WithAPIKey(key), option.WithBaseURL(endpoint))
	client := adapter.NewClient(
		openaiadapter.New(
			openaiadapter.WithClient(&sdk),
			openaiadapter.WithModel(model),
			openaiadapter.WithProfiles(endpoint),
		),
	)
	response, err := client.Execute(ctx, execution)
	if err != nil {
		log.Fatal(err)
	}
	if response.Outcome != prompty.OutcomeCompleted {
		log.Fatal("provider did not complete")
	}
	text, err := response.StrictText()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)
}
