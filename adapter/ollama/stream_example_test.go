package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// This runnable example uses local NDJSON, not a downloaded model or credentials.
func ExampleAdapter_StreamEvents() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"message":{"content":"hello"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":1}`)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local protocol fixture, not live model conformance",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(api.NewClient(endpoint, server.Client())),
		WithProfiles(server.URL, profile)))
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
	fmt.Println(stream.Status().State, stream.Status().Result.Usage.TotalTokens)
	// Output:
	// hello
	// completed 4
}
