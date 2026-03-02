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
	"github.com/skosovsky/prompty/adapter"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
	"github.com/skosovsky/prompty/fileregistry"
)

func main() {
	reg := fileregistry.New(".")
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	if err != nil {
		log.Fatalf("GetTemplate: %v", err)
	}
	type Payload struct {
		UserName string `prompt:"user_name"`
		Query    string `prompt:"query"`
	}
	exec, err := tpl.FormatStruct(ctx, &Payload{UserName: "Alice", Query: "What is 2+2?"})
	if err != nil {
		log.Fatalf("FormatStruct: %v", err)
	}

	adp := openaiadapter.New()
	params, err := adp.TranslateTyped(ctx, exec)
	if err != nil {
		log.Fatalf("Translate: %v", err)
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}
	client := openaisdk.NewClient(option.WithAPIKey(apiKey))
	resp, err := client.Chat.Completions.New(ctx, *params)
	if err != nil {
		log.Fatalf("OpenAI API: %v", err)
	}
	parts, err := adp.ParseResponse(ctx, resp)
	if err != nil {
		log.Fatalf("ParseResponse: %v", err)
	}
	fmt.Println(adapter.TextFromParts(parts))
}
