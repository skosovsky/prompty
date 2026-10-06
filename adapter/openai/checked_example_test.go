package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

func ExampleAdapter_checkedStreamAndPersistence() {
	// Arrange: an injected local SDK transport, not live model support evidence.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n"+
			"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"+
			"data: {\"choices\":[],\"usage\":{\"total_tokens\":9}}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local SDK/SSE fixture only",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(openaiadapter.New(openaiadapter.WithClient(&sdk),
		openaiadapter.WithModel("fixture"), openaiadapter.WithProfiles(server.URL, profile)))
	execution := prompty.SimplePrompt("caller-owned execution")
	// Act: creation is lazy; consume once, including trailing usage and lifecycle terminal.
	stream := client.ExecuteStream(context.Background(), execution, prompty.StreamNative)
	defer stream.Close()
	for _, err := range stream.Events() {
		if err != nil {
			panic(err) // production callers retain Status rather than blindly replaying
		}
	}
	status := stream.Status()
	wire, err := prompty.MarshalResponse(status.Result, prompty.WirePolicy{State: prompty.StatePreserve})
	if err != nil {
		panic(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		panic(err)
	}
	restored, err := prompty.DecodeResponseJSON(data)
	if err != nil {
		panic(err)
	}
	message, err := restored.AssistantMessage()
	if err != nil {
		panic(err)
	}
	continuation := execution.AddMessage(message)
	// Assert: unknown prompt/completion counters remain unknown after persistence.
	fmt.Println(status.State)
	fmt.Println(restored.Content[0].(prompty.TextPart).Text)
	fmt.Println(restored.Usage.Availability(), restored.Usage.TotalTokens)
	fmt.Println("messages:", len(continuation.Messages))
	// Output:
	// completed
	// hello
	// partial 9
	// messages: 2
}
