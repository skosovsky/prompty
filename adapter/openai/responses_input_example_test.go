package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
)

func ExampleNewResponses_mediaInput() {
	// Local SDK fixture, not a live vision/document-model check.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeResponseFixture(w, "completed", "document summary")
	}))
	defer server.Close()
	client := sdk.NewClient(
		option.WithAPIKey("fixture"),
		option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()),
	)
	profile := prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider: "openai.responses",
			Endpoint: server.URL + "/v1",
			Model:    "fixture",
		},
		Provenance: "local media mapping fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityResponseCreate: prompty.CapabilitySupported,
		},
		InputMIME: []string{"application/pdf", "image/png"},
	}
	manager := NewResponses(
		"host-account",
		WithClient(&client),
		WithModel("fixture"),
		WithProfiles(server.URL+"/v1", profile),
	)
	exec := prompty.NewExecution([]prompty.ChatMessage{{Role: prompty.RoleUser, Content: []prompty.ContentPart{
		prompty.TextPart{Text: "Summarize this document and compare the image."},
		prompty.MediaPart{MediaType: "document", MIMEType: "application/pdf", Data: []byte("%PDF-local-fixture")},
		prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://fixture.invalid/image.png"},
	}}})
	// Host supplies data and URL policy; this adapter never fetches or uploads media.
	result, err := manager.Create(
		context.Background(),
		prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Response.Content[0].(prompty.TextPart).Text)
	// Output:
	// document summary
}
