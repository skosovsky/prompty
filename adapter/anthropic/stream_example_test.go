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

// This executable example uses a local protocol fixture, not a live model.
func ExampleAdapter_StreamEvents() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\ndata: "+messageStartFixture+"\n\n")
		_, _ = fmt.Fprint(w, `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("hello"), prompty.StreamNative)
	defer stream.Close()
	for frame, err := range stream.Events() {
		if err != nil {
			fmt.Println(err)
			return
		}
		for _, part := range frame.Content {
			if text, ok := part.(prompty.TextPart); ok {
				fmt.Println(text.Text)
			}
		}
	}
	fmt.Println(stream.Status().State)
	// Output:
	// hello
	// completed
}
