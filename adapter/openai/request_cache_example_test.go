package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func ExampleAdapter_requestCache() {
	// Local SDK fixture: this profile is evidence only for this test transport.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":3}}}`,
		)
	}))
	defer server.Close()
	client := sdk.NewClient(option.WithAPIKey("fixture"), option.WithBaseURL(server.URL))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local fixture, not live model support",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityCacheImplicit:        prompty.CapabilitySupported,
			"cache_control:prompt_cache_key":       prompty.CapabilitySupported,
			"cache_control:prompt_cache_retention": prompty.CapabilitySupported,
		},
	}
	exec := prompty.SimplePrompt("question")
	exec.RequestCache = &prompty.CachePolicy{
		Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: "openai",
		Controls: prompty.JSONDocument(`{"prompt_cache_key":"host-document-key","prompt_cache_retention":"in-memory"}`),
	}
	result, err := adapter.NewClient(New(
		WithClient(&client), WithModel("fixture"), WithProfiles(server.URL, profile),
	)).Execute(context.Background(), exec)
	if err != nil {
		panic(err)
	}
	// Accepted settings do not guarantee a hit; usage is a separate observation.
	fmt.Println(result.Diagnostics.Cache[0].Disposition)
	fmt.Println(result.Usage.PromptTokensCached, result.Usage.TotalTokens)
	// Output:
	// accepted
	// 3 12
}
