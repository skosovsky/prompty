package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
)

func ExampleNewResponses() {
	// Local SDK fixture, not a live endpoint/model guarantee.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status := "queued"
		if calls.Add(1) > 1 {
			status = "completed"
		}
		writeResponseFixture(w, status, "answer")
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
		Provenance: "local protocol fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityResponseCreate:     prompty.CapabilitySupported,
			prompty.CapabilityResponseBackground: prompty.CapabilitySupported,
			prompty.CapabilityResponseRetrieve:   prompty.CapabilitySupported,
		},
	}
	manager := NewResponses(
		"host-account",
		WithClient(&client),
		WithModel("fixture"),
		WithProfiles(server.URL+"/v1", profile),
	)
	created, err := manager.Create(context.Background(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("analyze document"), Background: true, Store: new(false),
	})
	if err != nil {
		panic(err)
	}
	// Host chooses persistence and when to inspect. No worker is started.
	wire, err := prompty.EncodeRemoteResponseHandle(*created.Handle)
	if err != nil {
		panic(err)
	}
	handle, err := prompty.DecodeRemoteResponseHandle(wire)
	if err != nil {
		panic(err)
	}
	completed, err := manager.Get(context.Background(), handle)
	if err != nil {
		panic(err)
	}
	fmt.Println(created.Handle.Status, completed.Handle.Status)
	fmt.Println(completed.Response.Content[0].(prompty.TextPart).Text)
	// Output:
	// queued completed
	// answer
}
