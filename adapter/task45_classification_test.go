package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type classificationFixture struct {
	*checkedFixture

	err error
}

func (f *classificationFixture) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	f.calls++
	return nil, f.err
}

func (f *classificationFixture) StreamEvents(
	context.Context,
	*prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) { f.calls++; yield(nil, f.err) }
}
func TestCaptureErrorClassificationAcrossModes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
		kind string
	}{
		{"deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "canceled"},
		{"cancel", fmt.Errorf("wrapped: %w", context.Canceled), "canceled"},
		{"local", errors.New("private local preparation error"), "operation"},
		{"transport", &url.Error{Op: "Get", URL: "https://private.invalid", Err: errors.New("private connection")}, "transport"},
		{"URL parse", &url.Error{Op: "parse", URL: "https://private.invalid", Err: errors.New("private local URL")}, "operation"},
		{"parse", prompty.ErrSchemaJSONDecode, "parse"},
		{"schema", prompty.ErrSchemaMismatch, "schema_validation"},
		{"capture", prompty.ErrCapture, "capture"},
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			t.Run(test.name+"/"+string(mode), func(t *testing.T) {
				checkCaptureClassification(t, mode, test.err, test.kind)
			})
		}
	}
}

func checkCaptureClassification(t *testing.T, mode prompty.StreamMode, cause error, kind string) {
	t.Helper()
	t.Parallel()
	// Arrange: caller context remains alive; errors occur after executor admission.
	fixture := &classificationFixture{
		checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
		err:            cause,
	}
	var records []prompty.CaptureRecord
	ports := captureBindingFixture{records: &records}
	execution := prompty.SimplePrompt("fixture")
	execution.Capture = &prompty.CaptureBinding{Sink: ports, Projector: ports}
	execution.Requirements.Capture = prompty.CaptureRequirement{
		Stages:   []prompty.CaptureStage{prompty.CaptureTerminal},
		Accuracy: prompty.CaptureSemantic,
	}
	client := NewClient(fixture)
	// Act.
	var err error
	if mode == "" {
		result, e := client.Execute(t.Context(), execution)
		err = e
		require.Nil(t, result)
	} else {
		stream := client.ExecuteStream(t.Context(), execution, mode)
		terminals := 0
		for event, e := range stream.Events() {
			if event.Kind == prompty.StreamTerminal {
				terminals++
				err = e
			}
		}
		require.Equal(t, 1, terminals)
		if result := stream.Status().Result; result != nil {
			require.Empty(t, result.Outcome)
			require.Empty(t, result.Content)
		}
	}
	// Assert: cause, dispatch possibility and known outcome are independent.
	require.ErrorIs(t, err, cause)
	require.NoError(t, t.Context().Err())
	require.Equal(t, 1, fixture.calls)
	terminalRecords := 0
	for _, record := range records {
		if record.Stage != prompty.CaptureTerminal {
			continue
		}
		terminalRecords++
		require.True(t, record.AfterDispatch)
		require.Empty(t, record.Projection.Metadata.Outcome)
		var payload struct {
			ErrorKind string `json:"error_kind"`
		}
		require.NoError(t, json.Unmarshal(record.Projection.Payload, &payload))
		require.Equal(t, kind, payload.ErrorKind)
		require.NotContains(t, string(record.Projection.Payload), "private")
	}
	require.Equal(t, 1, terminalRecords)
}
