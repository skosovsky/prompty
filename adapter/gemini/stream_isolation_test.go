package gemini

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/streamtest"
)

func TestNativeParallelCancellationKeepsCorrelationRequestLocal(t *testing.T) {
	t.Parallel()
	streamtest.CancellationIsolation(t, func(server *httptest.Server) prompty.Invoker {
		sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
			APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
		})
		require.NoError(t, err)
		profile := prompty.CapabilityProfile{
			Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
			Provenance: "local concurrent SDK/SSE fixture, not live conformance",
			Capabilities: map[string]prompty.CapabilitySupport{
				prompty.CapabilityNativeStream: prompty.CapabilitySupported,
			},
		}
		return adapter.NewClient(New(WithModel("fixture"), WithClient(sdk), WithProfiles(server.URL, profile)))
	}, func(w http.ResponseWriter, label string, final bool) {
		w.Header().Set("Content-Type", "text/event-stream")
		if final {
			_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n"+
				"data: {\"usageMetadata\":{\"promptTokenCount\":3,\"candidatesTokenCount\":2,\"totalTokenCount\":5}}\n\n")
			return
		}
		_, _ = fmt.Fprintf(
			w,
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":%q},{\"functionCall\":{\"id\":\"shared-call\",\"name\":\"lookup\",\"args\":{\"tag\":%q}}}]}}]}\n\n",
			label,
			label,
		)
	}, 5)
}
