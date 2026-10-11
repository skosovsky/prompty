package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type captureStopFixture struct{ *nativeLoweringFixture }

func (f *captureStopFixture) Translate(exec *prompty.PromptExecution) (*prompty.PromptExecution, error) {
	return f.checkedFixture.Translate(exec)
}

func TestRequiredTerminalCaptureClosesAfterConsumerBreak(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{prompty.StreamNative, prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: require actual canonical closeout, not an ordinary logging callback.
			fixture := &captureStopFixture{&nativeLoweringFixture{loweringFixture: &loweringFixture{
				checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
				response:       prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "partial"}}),
			}, value: "partial"}}
			var records []prompty.CaptureRecord
			ports := captureBindingFixture{records: &records}
			exec := prompty.SimplePrompt("fixture")
			exec.Capture = &prompty.CaptureBinding{Sink: ports, Projector: ports}
			exec.Requirements.Capture = prompty.CaptureRequirement{
				Stages: []prompty.CaptureStage{prompty.CaptureTerminal}, Accuracy: prompty.CaptureSemantic,
			}
			stream := NewClient(fixture).ExecuteStream(context.Background(), exec, mode)
			// Act: stop after content, never receive another item or terminal.
			contentEvents := 0
			for event, err := range stream.Events() {
				require.NoError(t, err)
				if len(event.Content) != 0 {
					contentEvents++
					break
				}
			}
			// Assert: the sink gets one terminal, status freezes the acknowledged report.
			status := stream.Status()
			require.Equal(t, prompty.StreamCanceled, status.State)
			require.ErrorIs(t, status.Err, context.Canceled)
			require.Equal(t, 1, contentEvents)
			require.Equal(t, 1, fixture.calls)
			require.Len(t, status.Result.Content, 1)
			require.NotNil(t, status.Result.Diagnostics.Capture)
			require.True(t, status.Result.Diagnostics.Capture.Closed)
			terminals := 0
			for _, record := range records {
				if record.Stage == prompty.CaptureTerminal {
					terminals++
					require.True(t, record.AfterDispatch)
					var payload struct {
						ErrorKind string `json:"error_kind"`
					}
					require.NoError(t, json.Unmarshal(record.Projection.Payload, &payload))
					require.Equal(t, "canceled", payload.ErrorKind)
				}
			}
			require.Equal(t, 1, terminals)
		})
	}
}
