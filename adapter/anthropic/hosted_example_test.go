package anthropic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Local SDK fixture; model/account support must be supplied by the caller.
func ExampleHostedItems() {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, hostedHistoryFixture)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityHostedWebSearch: prompty.CapabilitySupported,
			prompty.CapabilityHostedWebFetch:  prompty.CapabilitySupported,
			prompty.CapabilityProviderState:   prompty.CapabilitySupported,
			prompty.CapabilityTools:           prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	exec := prompty.SimplePrompt("fixture")
	exec.HostedTools = []prompty.HostedToolRequest{{Kind: HostedWebSearch}, {Kind: HostedWebFetch}}
	exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object"}`)}}
	response, err := client.Execute(context.Background(), exec)
	if err != nil {
		panic(err)
	}
	items, err := HostedItems(response)
	if err != nil {
		panic(err)
	}
	fmt.Println(response.Outcome, len(items), calls.Load())
	// Persist portable state, then explicitly choose whether to resume.
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	if err != nil {
		panic(err)
	}
	restored, err := prompty.UnmarshalResponse(wire)
	if err != nil {
		panic(err)
	}
	message, err := restored.AssistantMessage()
	if err != nil {
		panic(err)
	}
	exec.Messages = append(exec.Messages, message)
	_, err = client.Execute(context.Background(), exec)
	if err != nil {
		panic(err)
	}
	fmt.Println("explicit resume", calls.Load())
	// Unknown hosted versions fail before any request.
	exec.HostedTools[0].Kind = "anthropic.future_tool"
	_, err = client.Execute(context.Background(), exec)
	fmt.Println("unsupported", errors.Is(err, adapter.ErrUnsupportedCapability), calls.Load())
	// Output:
	// paused 6 1
	// explicit resume 2
	// unsupported true 2
}
