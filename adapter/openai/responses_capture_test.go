package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesCaptureStageFailuresRetainKnownAcceptance(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest,
		prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			// Arrange: required capture can fail on either side of dispatch.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeResponseFixture(w, "completed", "answer")
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			exec := prompty.SimplePrompt("sensitive document")
			var records []prompty.CaptureRecord
			exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{}, Sink: captureFixtureSink{
				write: func(record prompty.CaptureRecord) error {
					records = append(records, record)
					if record.Stage == stage {
						return errors.New("sink-secret")
					}
					return nil
				},
			}}
			exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic,
				Stages: []prompty.CaptureStage{stage}}
			// Act.
			result, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
			// Assert: capture error is not proof that an accepted request did not execute.
			require.ErrorIs(t, err, prompty.ErrCapture)
			require.NotContains(t, err.Error(), "sink-secret")
			require.Equal(t, stage != prompty.CaptureTerminal, result.Diagnostics.Capture.Closed)
			require.False(t, result.Diagnostics.Capture.Complete)
			if stage == prompty.CaptureExecution || stage == prompty.CaptureTranslatedRequest {
				require.Zero(t, calls.Load())
				require.Nil(t, result.Handle)
			} else {
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, prompty.RemoteResponseCompleted, result.Handle.Status)
				require.Equal(t, "answer", result.Response.Content[0].(prompty.TextPart).Text)
			}
			terminals := 0
			for _, record := range records {
				if record.Stage == prompty.CaptureTerminal {
					terminals++
				}
			}
			require.Equal(t, 1, terminals)
		})
	}
}

func TestResponsesCaptureOriginalSourceAndDefaultRedaction(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"metadata", "semantic"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange: unknown source extension must not disappear before capture.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(
					w,
					`{"id":"resp_fixture","model":"fixture","status":"queued","output":[],"provider_extension":{"retained":true}}`,
				)
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			exec := prompty.SimplePrompt("sensitive document")
			var records []prompty.CaptureRecord
			exec.Capture = &prompty.CaptureBinding{
				Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
					records = append(records, record)
					return nil
				}},
			}
			if name == "semantic" {
				exec.Capture.Projector = captureFixtureProjector{}
				exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic}
			}
			// Act.
			result, err := adp.Create(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: exec, Store: new(false), Background: true},
			)
			// Assert: metadata-only is honest omission, not complete replay.
			require.NoError(t, err)
			require.Len(t, records, 5)
			require.True(t, result.Diagnostics.Capture.Closed)
			for index, record := range records {
				require.EqualValues(t, index+1, record.Sequence)
				require.Equal(t, records[0].InvocationID, record.InvocationID)
				require.Empty(t, record.PhysicalAttemptID)
				if name == "metadata" {
					require.Empty(t, record.Projection.Payload)
					require.Empty(t, record.Profile.Endpoint)
					require.Empty(t, record.Projection.Metadata.ProviderResponseID)
				}
			}
			if name == "semantic" {
				require.Contains(t, string(records[2].Projection.Payload), "provider_extension")
				require.Equal(t, http.StatusOK, *records[2].Projection.Metadata.HTTPStatus)
				require.True(t, result.Diagnostics.Capture.Complete)
			} else {
				require.False(t, result.Diagnostics.Capture.Complete)
			}
		})
	}
}

func TestResponsesCaptureUnavailableSourceIsMetadataOnly(t *testing.T) {
	t.Parallel()
	// Arrange: accepted truncated JSON is not a complete semantic document.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"resp_fixture"`)
	}))
	defer server.Close()
	adp := responseFixtureAdapter(server)
	exec := prompty.SimplePrompt("document")
	var records []prompty.CaptureRecord
	exec.Capture = &prompty.CaptureBinding{Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
		records = append(records, record)
		return nil
	}}}
	exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureMetadata,
		Stages: []prompty.CaptureStage{prompty.CaptureProviderResponse}}
	// Act.
	result, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
	// Assert: observed status without payload or invented reconcilable ID.
	require.ErrorIs(t, err, prompty.ErrRemoteResponseUnknown)
	require.NotErrorIs(t, err, prompty.ErrCapture)
	require.Nil(t, result.Handle)
	require.Equal(t, prompty.CaptureMetadata, records[2].Projection.Accuracy)
	require.Empty(t, records[2].Projection.Payload)
	require.Equal(t, http.StatusOK, *records[2].Projection.Metadata.HTTPStatus)
	require.False(t, result.Diagnostics.Capture.Complete)
}
