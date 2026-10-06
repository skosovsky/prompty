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

func TestCheckedNativeRefusalIsNotSchemaMismatchOrRetry(t *testing.T) {
	t.Parallel()
	t.Run("refusal", func(t *testing.T) {
		t.Parallel()
		nativeOutcomeSchemaConformance(t, `{"content":"{}","refusal":"denied"}`, "stop", prompty.OutcomeRefusal, 1)
	})
	t.Run("incomplete", func(t *testing.T) {
		t.Parallel()
		nativeOutcomeSchemaConformance(t, `{"content":"{}"}`, "length", prompty.OutcomeIncomplete, 0)
	})
}

func nativeOutcomeSchemaConformance(t *testing.T, delta, finish string, outcome prompty.Outcome, annotations int) {
	t.Helper()
	// Arrange: valid JSON content does not make a refused output successful.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"choices":[{"index":0,"delta":` + delta + `}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"` + finish + `"}]}`,
			`{"choices":[],"usage":{"total_tokens":9}}`,
			"[DONE]",
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local refusal fixture, not model capability evidence",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
	require.Zero(t, calls.Load())
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, prompty.ErrResponseOutcome)
			require.NotErrorIs(t, err, prompty.ErrSchemaMismatch)
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.Equal(t, outcome, status.Result.Outcome)
	require.Equal(t, finish, status.Result.FinishReason)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Len(t, status.Result.Annotations, annotations)
	require.Equal(t, 1, terminals)
	require.EqualValues(t, 1, calls.Load())
}
