// Funcmap and tool-calling example: template with render_tools_as_json and OpenAI tool calling.
// Run from this directory: go run .
// Requires OPENAI_API_KEY.
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
	tpl, err := reg.GetTemplate(ctx, "tools_demo")
	if err != nil {
		log.Fatalf("GetTemplate: %v", err)
	}
	type Payload struct {
		Query string `prompt:"query"`
	}
	exec, err := tpl.FormatStruct(ctx, &Payload{Query: "What is the weather in Paris? (hint: use a tool)"})
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
