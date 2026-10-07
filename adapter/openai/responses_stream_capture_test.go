package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesCreateStreamCapturePreservesAcceptance(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "sink failure"}[fail], func(t *testing.T) {
			t.Parallel()
			// Arrange: required semantic capture observes each decoded provider event.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeResponseSSEFixture(w, mixedResponseSSEFixture(true))
			}))
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			var records []prompty.CaptureRecord
			exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic,
				Stages: []prompty.CaptureStage{prompty.CaptureProviderResponse, prompty.CaptureTerminal}}
			exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{}, Sink: captureFixtureSink{
				write: func(record prompty.CaptureRecord) error {
					records = append(records, record)
					if fail && record.Stage == prompty.CaptureProviderResponse {
						return errors.New("sink-private-error")
					}
					return nil
				},
			}}
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: withResponseHostedTools(exec,
					[]prompty.HostedToolRequest{{Kind: HostedWebSearch}}), Store: new(false)},
			)
			// Act.
			var terminalErr error
			for _, err := range stream.Events() {
				if err != nil {
					terminalErr = err
				}
			}
			result := stream.RemoteResult()
			// Assert: capture failure is distinct from proof of provider rejection.
			require.NotNil(t, result.Handle)
			require.Equal(t, "resp_fixture", result.Handle.ProviderID)
			require.NotNil(t, result.Diagnostics.Capture)
			if fail {
				require.ErrorIs(t, terminalErr, prompty.ErrCapture)
				require.NotContains(t, terminalErr.Error(), "sink-private-error")
				require.Equal(t, prompty.RemoteResponseRunning, result.Handle.Status)
				require.False(t, result.Diagnostics.Capture.Complete)
			} else {
				require.NoError(t, terminalErr)
				require.True(t, result.Diagnostics.Capture.Complete)
				eventCount := 0
				for _, record := range records {
					if record.Projection.Codec == "openai.responses.sse.event" {
						eventCount++
						require.NotNil(t, record.Projection.Metadata.HTTPStatus)
						require.Equal(t, http.StatusOK, *record.Projection.Metadata.HTTPStatus)
					}
				}
				require.Equal(t, len(mixedResponseSSEFixture(true)), eventCount)
			}
		})
	}
}
