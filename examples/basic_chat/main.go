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
	"github.com/skosovsky/prompty/parser/yaml"
)

func main() {
	reg, err := fileregistry.New(".", fileregistry.WithParser(yaml.New()))
	if err != nil {
		log.Fatalf("fileregistry.New: %v", err)
	}
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
