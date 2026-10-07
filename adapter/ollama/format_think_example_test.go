package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func ExampleAdapter_nativeFormatAndThink() {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request api.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(api.ChatResponse{Message: api.Message{Role: "assistant",
			Thinking: "fixture reasoning", Content: `{"n":12}`}, Done: true, DoneReason: "stop"})
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance: "offline SDK fixture; host must verify its own endpoint/model",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
			prompty.CapabilityReasoning:        prompty.CapabilitySupported,
		},
		ModelOptionsSchema: prompty.JSONDocument(
			`{"type":"object","properties":{"provider_settings":{"type":"object","properties":{"think":{"enum":[false]}},"additionalProperties":false}}}`,
		),
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(api.NewClient(endpoint, server.Client())),
		WithProfiles(server.URL, profile)))
	execution := prompty.SimplePrompt("return n")
	execution.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"n":{"type":"integer","minimum":10}},"required":["n"]}`)}
	execution.ModelOptions = &prompty.ModelOptions{ProviderSettings: prompty.JSONDocument(`{"think":false}`)}
	response, err := client.Execute(context.Background(), execution)
	if err != nil {
		panic(err)
	}
	fmt.Println(response.Content[1].(prompty.TextPart).Text)

	// The same endpoint/model refuses a control outside the host's knowledge.
	execution.ModelOptions.ProviderSettings = prompty.JSONDocument(`{"think":"high"}`)
	_, err = client.Execute(context.Background(), execution)
	fmt.Println("unsupported rejected:", err != nil, "requests:", calls.Load())
	// Output:
	// {"n":12}
	// unsupported rejected: true requests: 1
}
