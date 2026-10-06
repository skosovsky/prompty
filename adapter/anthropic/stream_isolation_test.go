package anthropic

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/streamtest"
)

func TestNativeParallelCancellationKeepsCorrelationRequestLocal(t *testing.T) {
	t.Parallel()
	streamtest.CancellationIsolation(t, func(server *httptest.Server) prompty.Invoker {
		sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"),
			option.WithHTTPClient(server.Client()))
		profile := prompty.CapabilityProfile{
			Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
			Provenance: "local concurrent SDK/SSE fixture, not live conformance",
			Capabilities: map[string]prompty.CapabilitySupport{
				prompty.CapabilityNativeStream: prompty.CapabilitySupported,
			},
		}
		return adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
	}, func(w http.ResponseWriter, label string, final bool) {
		w.Header().Set("Content-Type", "text/event-stream")
		if final {
			_, _ = fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"+
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":2}}\n\n"+
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", messageStartFixture)
		_, _ = fmt.Fprintf(
			w,
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":%q}}\n\n",
			label,
		)
		_, _ = fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"+
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"shared-call\",\"name\":\"lookup\",\"input\":{}}}\n\n")
		args := fmt.Sprintf(`{"tag":%q}`, label)
		_, _ = fmt.Fprintf(
			w,
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%q}}\n\n",
			args,
		)
	}, 5)
}
