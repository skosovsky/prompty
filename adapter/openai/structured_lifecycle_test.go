package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type structuredHandleFixture struct {
	inner  prompty.Invoker
	handle *prompty.Stream
}

func (f *structuredHandleFixture) Execute(
	ctx context.Context,
	execution *prompty.PromptExecution,
) (*prompty.Response, error) {
	return f.inner.Execute(ctx, execution)
}

func (f *structuredHandleFixture) ExecuteStream(
	ctx context.Context,
	execution *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	f.handle = f.inner.ExecuteStream(ctx, execution, mode)
	return f.handle
}

func TestStructuredSSERefusalKeepsUsageAndFailedTerminalCapture(t *testing.T) {
	t.Parallel()
	// Arrange: a real SDK SSE refusal delta before finish and trailing usage.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"choices":[{"index":0,"delta":{"refusal":"denied"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"total_tokens":9}}`, "[DONE]",
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK/SSE fixture, not live model support",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
		},
	}
	invoker := &structuredHandleFixture{inner: adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"),
		WithProfiles(server.URL, profile))), handle: nil}
	var terminal []prompty.CaptureRecord
	execution := prompty.SimplePrompt("fixture")
	execution.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{}, Sink: captureFixtureSink{
		write: func(record prompty.CaptureRecord) error {
			if record.Stage == prompty.CaptureTerminal {
				terminal = append(terminal, record)
			}
			return nil
		},
	}}
	// Act.
	items, failures := 0, 0
	for _, err := range prompty.StreamStructuredOutput[struct{}](context.Background(), invoker, execution, prompty.StreamNative) {
		if err != nil {
			failures++
			require.ErrorIs(t, err, prompty.ErrResponseOutcome)
		} else {
			items++
		}
	}
	// Assert: refusal is not malformed JSON, no replay or loss of terminal usage.
	require.Zero(t, items)
	require.Equal(t, 1, failures)
	require.EqualValues(t, 1, calls.Load())
	status := invoker.handle.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.Equal(t, prompty.OutcomeRefusal, status.Result.Outcome)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.NotErrorIs(t, status.Err, prompty.ErrStructuredStream)
	require.Len(t, terminal, 1)
	require.Equal(t, prompty.OutcomeRefusal, terminal[0].Projection.Metadata.Outcome)
	require.Equal(t, 9, terminal[0].Projection.Metadata.Usage.TotalTokens)
	require.Contains(t, string(terminal[0].Projection.Payload), `"error_kind":"provider_outcome"`)
}
