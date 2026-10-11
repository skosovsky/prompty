package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
)

func ExampleNewResponses_strictReasoning() {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"resp_fixture","model":"fixture","status":"completed","output":[{"type":"function_call","id":"fc_fixture","call_id":"call_one","name":"lookup","arguments":"{\"n\":12}","status":"completed"}]}`,
		)
	}))
	defer server.Close()
	client := sdk.NewClient(option.WithAPIKey("offline"), option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()))
	profile := prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider: "openai.responses",
			Endpoint: server.URL + "/v1",
			Model:    "fixture",
		},
		Provenance: "offline SDK fixture; verify your own endpoint/model",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityResponseCreate: prompty.CapabilitySupported,
			prompty.CapabilityTools:          prompty.CapabilitySupported,
			prompty.CapabilityStrictTools:    prompty.CapabilitySupported,
			prompty.CapabilityReasoning:      prompty.CapabilitySupported,
		},
		ModelOptionsSchema: prompty.JSONDocument(
			`{"type":"object","properties":{"provider_settings":{"type":"object","properties":{"reasoning_effort":{"enum":["low"]},"reasoning_summary":{"enum":["concise"]}},"additionalProperties":false}}}`,
		),
	}
	manager := NewResponses(
		"host-account",
		WithClient(&client),
		WithModel("fixture"),
		WithProfiles(server.URL+"/v1", profile),
	)
	execution := prompty.SimplePrompt("lookup")
	execution.ModelOptions = &prompty.ModelOptions{
		ProviderSettings: prompty.JSONDocument(`{"reasoning_effort":"low","reasoning_summary":"concise"}`),
	}
	execution.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(
		`{"type":"object","properties":{"n":{"type":"integer","minimum":10}},"required":["n"],"additionalProperties":false}`,
	)}}
	request := prompty.RemoteResponseRequest{Execution: execution, Store: new(false), StrictTools: true}
	result, err := manager.Create(context.Background(), request)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Response.Content[0].(prompty.ToolCallPart).ID)

	// Strict mode refuses an open object rather than closing it implicitly.
	execution.Tools[0].Parameters = prompty.JSONDocument(`{"type":"object"}`)
	_, err = manager.Create(context.Background(), request)
	fmt.Println("open schema rejected:", err != nil, "requests:", calls.Load())
	// Output:
	// call_one
	// open schema rejected: true requests: 1
}
