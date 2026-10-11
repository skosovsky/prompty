package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// This executable example uses local protocol fixtures, not live credentials.
func ExampleNewCacheResources() {
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			_, _ = fmt.Fprint(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "generateContent"):
			_, _ = fmt.Fprint(
				w,
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":"STOP"}]}`,
			)
		default:
			_, _ = fmt.Fprintf(w, `{"name":"cachedContents/example","model":"models/fixture","expireTime":%q}`, expires)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	sdk, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"}})
	if err != nil {
		fmt.Println(err)
		return
	}
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityCacheCreate:  prompty.CapabilitySupported,
			prompty.CapabilityCacheInspect: prompty.CapabilitySupported,
			prompty.CapabilityCacheUpdate:  prompty.CapabilitySupported,
			prompty.CapabilityCacheDelete:  prompty.CapabilitySupported,
			prompty.CapabilityCacheReuse:   prompty.CapabilitySupported,
		},
	}
	opts := []Option{WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)}
	manager := NewCacheResources("host-account", opts...)
	created, err := manager.Create(
		ctx,
		prompty.CacheResourceCreateRequest{Execution: prompty.SimplePrompt("immutable document"),
			Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	exec := prompty.SimplePrompt("question")
	exec.CacheResource = created.Handle
	client := adapter.NewClient(New(append(opts, WithCacheAccountBinding("host-account"))...))
	answer, err := client.Execute(ctx, exec)
	if err != nil {
		fmt.Println(err)
		return
	}
	// Independent question reuses the same immutable remote content/config.
	second := prompty.SimplePrompt("another independent question")
	second.CacheResource = created.Handle
	secondAnswer, err := client.Execute(ctx, second)
	if err != nil {
		fmt.Println(err)
		return
	}
	updated, err := manager.UpdateExpiry(ctx, *created.Handle, prompty.CacheResourceExpiry{TTL: 2 * time.Hour})
	if err != nil {
		fmt.Println(err)
		return
	}
	inspected, err := manager.Get(ctx, *updated.Handle)
	if err != nil {
		fmt.Println(err)
		return
	}
	deleted, err := manager.Delete(ctx, *inspected.Handle)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(answer.Content[0].(prompty.TextPart).Text)
	fmt.Println(secondAnswer.Content[0].(prompty.TextPart).Text)
	fmt.Println(deleted.Handle.State)
	// Output:
	// answer
	// answer
	// deleted
}
