package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func ExampleAdapter_StreamEvents_incrementalArguments() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","willContinue":true}}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"functionCall":{"partialArgs":[{"jsonPath":"$.n","numberValue":1}]}}]},"finishReason":"STOP"}]}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer server.Close()
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendVertexAI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta1"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "offline Vertex protocol fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
			prompty.CapabilityTools:            prompty.CapabilitySupported,
			CapabilityIncrementalToolArguments: prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	execution := prompty.SimplePrompt("fixture")
	execution.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
	execution.Requirements.Capabilities = []string{CapabilityIncrementalToolArguments}
	stream := client.ExecuteStream(context.Background(), execution, prompty.StreamNative)
	defer stream.Close()
	for frame, streamErr := range stream.Events() {
		if streamErr != nil {
			fmt.Println(streamErr)
			return
		}
		for _, part := range frame.Content {
			if call, ok := part.(prompty.ToolCallPart); ok && call.Args == "" {
				fmt.Println("provisional") // Inspect partial_function_call annotation; no handler yet.
			}
		}
	}
	status := stream.Status()
	fmt.Println(status.Result.Content[0].(prompty.ToolCallPart).Args)
	fmt.Println(status.State)
	// Output:
	// provisional
	// {"n":1}
	// completed
}
