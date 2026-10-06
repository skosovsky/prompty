package openai

import (
	"context"
	"fmt"
	"io"
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

type captureRedactionFixtureProjector struct{}

func (captureRedactionFixtureProjector) MaxAccuracy() prompty.CaptureAccuracy {
	return prompty.CaptureSemantic
}

func (captureRedactionFixtureProjector) ProjectCapture(
	_ context.Context,
	record prompty.CaptureRecord,
) (prompty.CaptureProjection, error) {
	// An adversarial host callback may mutate its copy, not the SDK request/result.
	if len(record.Projection.Payload) != 0 {
		record.Projection.Payload[0] = 'x'
	}
	if record.Projection.Metadata.Usage != nil {
		record.Projection.Metadata.Usage.TotalTokens = 100
	}
	return prompty.CaptureProjection{Payload: []byte(`{}`), MediaType: "application/json", Codec: "host.allowlist",
		Accuracy: prompty.CaptureSemantic, Redacted: true, Complete: false}, nil
}

func TestSDKCaptureHostPolicyKeepsCredentialsAndPayloadOutOfSink(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK auth exists on real HTTP request, never in approved capture.
			var calls atomic.Int32
			requests := make(chan *http.Request, 1)
			bodies := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				requests <- r
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", http.StatusBadRequest)
					return
				}
				bodies <- body
				if mode == prompty.StreamNative {
					w.Header().Set("Content-Type", "text/event-stream")
					for _, chunk := range []string{
						`{"id":"private-response","choices":[{"index":0,"delta":{"content":"private-output"}}]}`,
						`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
						`{"choices":[],"usage":{"total_tokens":9}}`, "[DONE]"} {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
					}
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(
						w,
						`{"id":"private-response","choices":[{"index":0,"message":{"content":"private-output"},"finish_reason":"stop"}],"usage":{"total_tokens":9}}`,
					)
				}
			}))
			defer server.Close()
			sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("private-credential"))
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local SDK host policy fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream: prompty.CapabilitySupported},
			}
			client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
			var records []prompty.CaptureRecord
			exec := prompty.SimplePrompt("private-prompt")
			exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic,
				Stages: []prompty.CaptureStage{prompty.CaptureTerminal}}
			exec.Capture = &prompty.CaptureBinding{
				MaxBytes:  2,
				Projector: captureRedactionFixtureProjector{},
				Sink: captureFixtureSink{
					write: func(record prompty.CaptureRecord) error { records = append(records, record); return nil },
				},
			}
			// Act.
			var response *prompty.Response
			if mode == "" {
				var err error
				response, err = client.Execute(context.Background(), exec)
				require.NoError(t, err)
			} else {
				stream := client.ExecuteStream(context.Background(), exec, mode)
				for _, err := range stream.Events() {
					require.NoError(t, err)
				}
				require.Equal(t, prompty.StreamCompleted, stream.Status().State)
				response = stream.Status().Result
			}
			// Assert: callback edits and delivery bound do not alter actual transport/result.
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, "Bearer private-credential", (<-requests).Header.Get("Authorization"))
			require.Contains(t, string(<-bodies), "private-prompt")
			require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "private-output"}}, response.Content)
			require.Equal(t, 9, response.Usage.TotalTokens)
			require.True(t, response.Diagnostics.Capture.Closed)
			require.False(t, response.Diagnostics.Capture.Complete)
			require.Empty(t, response.Diagnostics.Capture.Failures)
			require.GreaterOrEqual(t, len(records), 5)
			for index, record := range records {
				require.EqualValues(t, index+1, record.Sequence)
				require.Equal(t, response.Diagnostics.Capture.InvocationID, record.InvocationID)
				require.Equal(t, []byte(`{}`), record.Projection.Payload)
				require.True(t, record.Projection.Redacted)
				require.False(t, record.Projection.Complete)
				require.Empty(t, record.Profile.Endpoint)
				require.Empty(t, record.Projection.Metadata.ProviderResponseID)
				require.Empty(t, record.Projection.Metadata.RequestID)
			}
			require.Equal(t, prompty.CaptureTerminal, records[len(records)-1].Stage)
			require.Equal(t, "private-prompt", exec.Messages[0].Content[0].(prompty.TextPart).Text)
		})
	}
}
