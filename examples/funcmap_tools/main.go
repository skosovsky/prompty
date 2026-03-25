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
	"github.com/skosovsky/prompty/parser/yaml"
)

func main() {
	reg, err := fileregistry.New(".", fileregistry.WithParser(yaml.New()))
	if err != nil {
		log.Fatalf("fileregistry.New: %v", err)
	}
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "tools_demo")
	if err != nil {
		log.Fatalf("GetTemplate: %v", err)
	}
	type Payload struct {
		Query string `prompt:"query"`
	}
	exec, err := tpl.FormatStruct(&Payload{Query: "What is the weather in Paris? (hint: use a tool)"})
	if err != nil {
		log.Fatalf("FormatStruct: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}
	openaiClient := openaisdk.NewClient(option.WithAPIKey(apiKey))
	adp := openaiadapter.New(openaiadapter.WithClient(&openaiClient))
	client := adapter.NewClient(adp)
	resp, err := client.Generate(ctx, exec)
	if err != nil {
		log.Fatalf("Generate: %v", err)
	}
	fmt.Println(resp.Text())
}
