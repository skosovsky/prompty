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

// This executable example uses local protocol data, not a live model.
func ExampleAdapter_StreamEvents() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]},\"finishReason\":\"STOP\"}]}\n\n",
		)
		_, _ = fmt.Fprint(
			w,
			"data: {\"candidates\":[{\"citationMetadata\":{\"citationSources\":[{\"uri\":\"https://fixture.invalid\"}]}}]}\n\n",
		)
	}))
	defer server.Close()
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("hello"), prompty.StreamNative)
	defer stream.Close()
	for frame, streamErr := range stream.Events() {
		if streamErr != nil {
			fmt.Println(streamErr)
			return
		}
		for _, part := range frame.Content {
			if text, ok := part.(prompty.TextPart); ok {
				fmt.Println(text.Text)
			}
		}
		for _, annotation := range frame.MessageAnnotations {
			fmt.Println(annotation.Kind)
		}
	}
	fmt.Println(stream.Status().State)
	// Output:
	// hello
	// candidate_citation_metadata
	// completed
}
