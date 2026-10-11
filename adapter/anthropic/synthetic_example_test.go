package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// This example exercises a deterministic protocol fixture, not a live model.
func ExampleNewSyntheticOutput() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"fixture","type":"message","role":"assistant","model":"fixture",`+
			`"content":[{"type":"tool_use","id":"call","name":"extract","input":{"value":7}}],`+
			`"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilitySyntheticOutput:     prompty.CapabilitySupported,
			prompty.CapabilityTools:               prompty.CapabilitySupported,
			prompty.CapabilityForcedToolSelection: prompty.CapabilitySupported,
		},
	}
	exec := prompty.SimplePrompt("extract value")
	exec.Requirements.OutputMode = prompty.OutputSyntheticTool
	exec.ResponseFormat = &prompty.SchemaDefinition{Name: "extract", Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"],"additionalProperties":false}`,
	)}
	client := adapter.NewClient(
		NewSyntheticOutput(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)),
	)
	result, err := client.Execute(context.Background(), exec)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result.Content[0].(prompty.TextPart).Text)
	fmt.Println(result.Outcome)
	// Output:
	// {"value":7}
	// completed
}
