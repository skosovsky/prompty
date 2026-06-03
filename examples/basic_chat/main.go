// Basic chat example: load a prompt from a local YAML file (fileregistry) and call OpenAI.
// Run from this directory: go run .  (requires OPENAI_API_KEY in the environment).
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
	"github.com/skosovsky/prompty/fileregistry"
	"github.com/skosovsky/prompty/parser/yaml"
)

func main() {
	reg, err := fileregistry.New(".", fileregistry.WithParser(yaml.New()))
	if err != nil {
		log.Fatalf("fileregistry.New: %v", err)
	}
	type supportInput struct {
		UserName string `prompt:"user_name"`
		Query    string `prompt:"query"`
	}

	ctx := context.Background()
	planInput, err := prompty.RegistryPlanInputFrom(&supportInput{
		UserName: "Alice",
		Query:    "What is 2+2?",
	})
	if err != nil {
		log.Fatalf("RegistryPlanInputFrom: %v", err)
	}
	plan, err := reg.Plan(ctx, "support_agent", planInput)
	if err != nil {
		log.Fatalf("Plan: %v", err)
	}
	exec, err := plan.Execute(ctx)
	if err != nil {
		log.Fatalf("Execute: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}
	openaiClient := openaisdk.NewClient(option.WithAPIKey(apiKey))
	adp := openaiadapter.New(openaiadapter.WithClient(&openaiClient))
	client := adapter.NewClient(adp)
	resp, err := client.Execute(ctx, exec)
	if err != nil {
		log.Fatalf("Execute: %v", err)
	}
	text, err := resp.StrictText()
	if err != nil {
		log.Fatalf("StrictText: %v", err)
	}
	fmt.Println(text)
}
