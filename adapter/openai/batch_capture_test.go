package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestBatchesCaptureAfterUploadRetainsFileWithoutSubmitting(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{prompty.CaptureProviderResponse, prompty.CaptureTranslatedRequest} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			// Arrange: upload is accepted; fail only its result or actual batch request.
			var uploads, submits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/files" {
					uploads.Add(1)
					writeBatchUploadFixture(w)
				} else {
					submits.Add(1)
					writeBatchFixture(w, "validating")
				}
			}))
			defer server.Close()
			manager, input := batchFixtureManager(server), batchFixtureInput()
			terminals := 0
			input.Options.Capture = prompty.CaptureRequirement{
				Stages:   []prompty.CaptureStage{stage},
				Accuracy: prompty.CaptureMetadata,
			}
			input.Options.Binding = &prompty.CaptureBinding{
				Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
					if record.Stage == prompty.CaptureTerminal {
						terminals++
					}
					require.Empty(t, record.Projection.Payload)
					if record.Stage == stage && record.AfterDispatch {
						return errors.New("private-recorder-error")
					}
					return nil
				}},
			}
			// Act.
			result, err := manager.Submit(t.Context(), input)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrCapture)
			require.NotContains(t, err.Error(), "private-recorder-error")
			require.Equal(t, int32(1), uploads.Load())
			require.Zero(t, submits.Load())
			require.False(t, result.SubmissionAttempted)
			require.Nil(t, result.Handle)
			require.Equal(t, "file-input", result.Resources[0].ProviderID)
			require.Equal(t, 1, terminals)
		})
	}
}

type opaqueBatchFiles struct{ prompty.BatchFileTransport }

func TestBatchesOpaqueFileTransportCannotPromiseRequiredSourceCapture(t *testing.T) {
	t.Parallel()
	// Arrange: binding and transport are real; the wrapper declares no source hooks.
	var calls atomic.Int32
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeBatchUploadFixture(w) }),
	)
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	manager.files = opaqueBatchFiles{BatchFileTransport: manager.files}
	input.Options.Capture = prompty.CaptureRequirement{
		Stages:   []prompty.CaptureStage{prompty.CaptureProviderResponse},
		Accuracy: prompty.CaptureSemantic,
	}
	// Act.
	_, err := manager.Submit(t.Context(), input)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
	require.Zero(t, calls.Load())
}

func TestBatchesRequiredCaptureFailsBeforeUpload(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeBatchUploadFixture(w) }),
	)
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	input.Options.Capture = prompty.CaptureRequirement{
		Stages:   []prompty.CaptureStage{prompty.CaptureExecution},
		Accuracy: prompty.CaptureMetadata,
	}
	input.Options.Binding = &prompty.CaptureBinding{
		Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
			if record.Stage == prompty.CaptureExecution {
				return errors.New("recorder-failure")
			}
			return nil
		}},
	}
	// Act.
	result, err := manager.Submit(t.Context(), input)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrCapture)
	require.Zero(t, calls.Load())
	require.False(t, result.SubmissionAttempted)
	require.Empty(t, result.Resources)
}
