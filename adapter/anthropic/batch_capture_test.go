package anthropic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type batchFixtureSink struct {
	write func(prompty.CaptureRecord) error
}

func (batchFixtureSink) Capabilities() prompty.CaptureSinkCapabilities {
	return prompty.CaptureSinkCapabilities{}
}
func (s batchFixtureSink) WriteCapture(_ context.Context, record prompty.CaptureRecord) (prompty.CaptureAck, error) {
	return prompty.CaptureAck{}, s.write(record)
}

func TestMessageBatchesCaptureFailureRetainsAcceptedHandle(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest,
		prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeBatchFixture(w, "in_progress") },
				),
			)
			defer server.Close()
			manager, input := batchFixtureManager(server), batchFixtureInput()
			terminals := 0
			input.Options.Capture = prompty.CaptureRequirement{
				Stages:   []prompty.CaptureStage{stage},
				Accuracy: prompty.CaptureMetadata,
			}
			input.Options.Binding = &prompty.CaptureBinding{
				Sink: batchFixtureSink{write: func(record prompty.CaptureRecord) error {
					if record.Stage == prompty.CaptureTerminal {
						terminals++
					}
					require.Empty(t, record.Projection.Payload)
					if record.Stage == stage {
						return errors.New("sink-private-secret")
					}
					return nil
				}},
			}
			// Act.
			result, err := manager.Submit(t.Context(), input)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrCapture)
			require.NotContains(t, err.Error(), "sink-private-secret")
			require.Equal(t, 1, terminals)
			require.NotNil(t, result.Capture)
			if stage == prompty.CaptureExecution || stage == prompty.CaptureTranslatedRequest {
				require.Zero(t, calls.Load())
				require.Nil(t, result.Handle)
			} else {
				require.Equal(t, int32(1), calls.Load())
				require.Equal(t, prompty.BatchRunning, result.Handle.Status)
				require.Equal(t, "msgbatch_fixture", result.Handle.ProviderID)
			}
		})
	}
}
