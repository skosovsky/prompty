package ollama

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/streamtest"
)

func TestNativeParallelCancellationKeepsCorrelationRequestLocal(t *testing.T) {
	t.Parallel()
	streamtest.CancellationIsolation(t, func(server *httptest.Server) prompty.Invoker {
		endpoint, err := url.Parse(server.URL)
		require.NoError(t, err)
		chat, err := NewHTTPChatClient(endpoint, server.Client(), 4*1024*1024)
		require.NoError(t, err)
		profile := prompty.CapabilityProfile{
			Identity:   prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
			Provenance: "local concurrent NDJSON fixture, not live conformance",
			Capabilities: map[string]prompty.CapabilitySupport{
				prompty.CapabilityNativeStream: prompty.CapabilitySupported,
			},
		}
		return adapter.NewClient(New(WithModel("fixture"), WithClient(chat), WithProfiles(server.URL, profile)))
	}, func(w http.ResponseWriter, label string, final bool) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		if final {
			_, _ = fmt.Fprintln(w, `{"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":2}`)
			return
		}
		_, _ = fmt.Fprintf(
			w,
			"{\"message\":{\"content\":%q,\"tool_calls\":[{\"id\":\"shared-call\",\"function\":{\"name\":\"lookup\",\"arguments\":{\"tag\":%q}}}]},\"done\":false}\n",
			label,
			label,
		)
	}, 5)
}
