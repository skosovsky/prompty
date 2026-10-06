package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func ExampleNewFileBatches() {
	// Local fixtures only. A real host supplies its own upload/download transport,
	// authentication and cleanup policy; prompty does not install a file store.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
			return
		}
		_, _ = io.WriteString(
			w,
			`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED"},"response":{"responsesFile":"files/output"}}`,
		)
	}))
	defer server.Close()
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"}})
	if err != nil {
		panic(err)
	}
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local file batch fixture; no live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityBatchSubmit:  prompty.CapabilitySupported,
			prompty.CapabilityBatchInspect: prompty.CapabilitySupported,
			prompty.CapabilityBatchResults: prompty.CapabilitySupported},
	}
	files := &batchFixtureFiles{
		binding: prompty.BatchFileBinding{Provider: "gemini", Endpoint: server.URL, AccountBinding: "host-account"},
		status:  http.StatusOK,
		output:  "{\"key\":\"one\",\"error\":{\"code\":3}}\n",
	}
	manager := NewFileBatches(
		"host-account",
		files,
		BatchFileOptions{},
		WithClient(client),
		WithModel("fixture"),
		WithProfiles(server.URL, profile),
	)
	input, options := inlineBatchFixtureInputs()
	input.Items, options.Bindings = input.Items[:1], options.Bindings[:1]
	submitted, err := manager.Submit(ctx, input)
	if err != nil {
		panic(err)
	}
	// The caller, not a background worker, chooses when to inspect and download.
	inspected, err := manager.Get(ctx, *submitted.Handle)
	if err != nil {
		panic(err)
	}
	results, err := manager.Results(ctx, *inspected.Handle, options)
	if err != nil {
		panic(err)
	}
	fmt.Println(results.Completeness.DownloadComplete, results.Completeness.CorrelationComplete)
	fmt.Println(results.Items[0].ID, results.Items[0].Outcome)
	// Output:
	// true true
	// one error
}
