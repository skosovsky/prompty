package openai

import (
	"context"
	"encoding/json"
	"errors"
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

func TestTransportFailureCaptureKeepsOutcomeUnknownWithoutReplay(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK retryable status and a failed terminal sink, with no parsed result.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"message":"private-provider-body","type":"unavailable"}}`))
			}))
			defer server.Close()
			sdk := openai.NewClient(
				option.WithBaseURL(server.URL),
				option.WithAPIKey("private-credential"),
				option.WithMaxRetries(3),
			)
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local SDK unknown-outcome fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream: prompty.CapabilitySupported},
			}
			client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
			sentinel := errors.New("private-sink-error")
			var terminals []prompty.CaptureRecord
			exec := prompty.SimplePrompt("fixture")
			exec.Requirements.Capture.Stages = []prompty.CaptureStage{prompty.CaptureTerminal}
			exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{},
				Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
					if record.Stage == prompty.CaptureTerminal {
						terminals = append(terminals, record)
						return sentinel
					}
					return nil
				}}}
			// Act.
			var err error
			if mode == "" {
				var response *prompty.Response
				response, err = client.Execute(context.Background(), exec)
				require.Nil(t, response)
			} else {
				stream := client.ExecuteStream(context.Background(), exec, mode)
				for range stream.Events() {
				}
				status := stream.Status()
				err = status.Err
				require.Equal(t, prompty.StreamFailed, status.State)
				require.Empty(t, status.Result.Outcome)
				require.Empty(t, status.Result.Content)
			}
			// Assert: separate SDK/capture errors, no success/not-executed fabrication or replay.
			require.ErrorIs(t, err, adapter.ErrInvalidResponse)
			require.NotContains(t, err.Error(), "private-provider-body")
			require.NotContains(t, err.Error(), "private-credential")
			require.ErrorIs(t, err, sentinel)
			var captureErr *prompty.CaptureError
			require.ErrorAs(t, err, &captureErr)
			require.True(t, captureErr.AfterDispatch)
			require.NotContains(t, captureErr.Error(), "private")
			require.Equal(t, int32(1), calls.Load())
			require.Len(t, terminals, 1)
			require.True(t, terminals[0].AfterDispatch)
			require.Empty(t, terminals[0].Projection.Metadata.Outcome)
			require.NotContains(t, string(terminals[0].Projection.Payload), "private")
			var terminal map[string]any
			require.NoError(t, json.Unmarshal(terminals[0].Projection.Payload, &terminal))
			// The sanitized provider failure has a safe parse category, without raw status text.
			require.Equal(t, "parse", terminal["error_kind"])
			require.Nil(t, terminal["response"])
		})
	}
}
