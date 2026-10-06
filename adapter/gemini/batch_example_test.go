package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func ExampleNewBatches() {
	// Local protocol fixture, not live account/model conformance.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, ":batchGenerateContent") {
			_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
			return
		}
		_, _ = io.WriteString(w, inlineBatchFixtureOutput)
	}))
	defer server.Close()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	if err != nil {
		panic(err)
	}
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityBatchSubmit:  prompty.CapabilitySupported,
			prompty.CapabilityBatchInspect: prompty.CapabilitySupported,
			prompty.CapabilityBatchResults: prompty.CapabilitySupported,
		},
	}
	manager := NewBatches("host-account", WithClient(client), WithModel("fixture"), WithProfiles(server.URL, profile))
	input, bindings := inlineBatchFixtureInputs()
	submitted, err := manager.Submit(context.Background(), input)
	if err != nil {
		panic(err)
	}
	// Host persists handle/input and decides when to inspect, without a worker.
	inspected, err := manager.Get(context.Background(), *submitted.Handle)
	if err != nil {
		panic(err)
	}
	results, err := manager.Results(context.Background(), *inspected.Handle, bindings)
	if err != nil {
		panic(err)
	}
	fmt.Println(results.Completeness.CorrelationComplete)
	for _, item := range results.Items {
		fmt.Println(item.ID, item.Outcome)
	}
	// Output:
	// true
	// three refusal
	// two error
	// one success
}
