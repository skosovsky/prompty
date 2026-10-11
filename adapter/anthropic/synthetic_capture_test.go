package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const syntheticCaptureFixture = `{"id":"fixture","type":"message","role":"assistant","model":"fixture",` +
	`"content":[{"type":"tool_use","id":"call","name":"extract_record",` +
	`"input":{"kind":"ok","n":9007199254740993}}],"stop_reason":"tool_use",` +
	`"usage":{"input_tokens":3,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},` +
	`"provider_extension":{"exact":9007199254740993}}`

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestSyntheticCaptureFailureStages(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{
		prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderResponse,
		prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered} {
			t.Run(string(stage)+"/"+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange: required sink failure with default metadata-only redaction.
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, syntheticCaptureFixture)
				}))
				defer server.Close()
				sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("private-credential"))
				client := adapter.NewClient(NewSyntheticOutput(
					WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, syntheticProfile(server.URL)),
				))
				exec := syntheticExecution()
				var records []prompty.CaptureRecord
				exec.Requirements.Capture.Stages = []prompty.CaptureStage{stage}
				exec.Capture = &prompty.CaptureBinding{
					Sink: batchFixtureSink{write: func(record prompty.CaptureRecord) error {
						records = append(records, record)
						if record.Stage == stage {
							return errors.New("sink-private-secret")
						}
						return nil
					}},
				}
				// Act.
				var response *prompty.Response
				var err error
				if mode == "" {
					response, err = client.Execute(t.Context(), exec)
				} else {
					stream := client.ExecuteStream(t.Context(), exec, mode)
					for range stream.Events() {
					}
					status := stream.Status()
					response, err = status.Result, status.Err
					require.Equal(t, prompty.StreamFailed, status.State)
				}
				// Assert: known provider success survives post-dispatch capture failure, no retry.
				require.ErrorIs(t, err, prompty.ErrCapture)
				require.NotContains(t, err.Error(), "sink-private-secret")
				var captureErr *prompty.CaptureError
				require.ErrorAs(t, err, &captureErr)
				after := stage != prompty.CaptureExecution && stage != prompty.CaptureTranslatedRequest
				require.Equal(t, after, captureErr.AfterDispatch)
				if after {
					require.EqualValues(t, 1, calls.Load())
					require.NotNil(t, response)
					require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
					require.Equal(t, 5, response.Usage.TotalTokens)
					require.Contains(t, response.Usage.Known, prompty.UsageTotal)
				} else {
					require.Zero(t, calls.Load())
				}
				terminals := 0
				for index, record := range records {
					require.EqualValues(t, index+1, record.Sequence)
					require.Equal(t, records[0].InvocationID, record.InvocationID)
					require.Empty(t, record.PhysicalAttemptID)
					require.Empty(t, record.Projection.Payload)
					require.Contains(t, record.SDKBuild, "anthropic-sdk-go")
					if record.Stage == prompty.CaptureTerminal {
						terminals++
					}
				}
				require.Equal(t, 1, terminals)
			})
		}
	}
}

type syntheticCaptureProjector struct{}

func (syntheticCaptureProjector) MaxAccuracy() prompty.CaptureAccuracy {
	return prompty.CaptureSemantic
}

func (syntheticCaptureProjector) ProjectCapture(
	_ context.Context, record prompty.CaptureRecord,
) (prompty.CaptureProjection, error) {
	return record.Projection, nil
}

func TestSyntheticSemanticCaptureSeparatesOutputGuarantee(t *testing.T) {
	t.Parallel()
	// Arrange: native output is unsupported; capture preserves private tool semantics.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, syntheticCaptureFixture)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("private-credential"))
	client := adapter.NewClient(NewSyntheticOutput(
		WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, syntheticProfile(server.URL)),
	))
	exec := syntheticExecution()
	var records []prompty.CaptureRecord
	exec.Requirements.Capture.Accuracy = prompty.CaptureSemantic
	exec.Requirements.Capture.Stages = []prompty.CaptureStage{
		prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderResponse,
		prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
	}
	exec.Capture = &prompty.CaptureBinding{
		Projector: syntheticCaptureProjector{},
		Sink: batchFixtureSink{write: func(record prompty.CaptureRecord) error {
			records = append(records, record)
			return nil
		}},
	}
	// Act.
	response, err := client.Execute(t.Context(), exec)
	// Assert: source schema and received validation are not native constrained decoding.
	require.NoError(t, err)
	require.Len(t, records, 5)
	require.True(t, response.Diagnostics.Capture.Complete)
	require.True(t, response.Diagnostics.Capture.Closed)
	require.Contains(t, string(records[0].Projection.Payload), "synthetic_tool")
	require.Contains(t, string(records[1].Projection.Payload), `"tool_choice":{"name":"extract_record","type":"tool"}`)
	require.NotContains(t, string(records[1].Projection.Payload), "output_config")
	require.Contains(t, string(records[2].Projection.Payload), `"provider_extension":{"exact":9007199254740993}`)
	require.Contains(t, string(records[2].Projection.Payload), `"type":"tool_use"`)
	for _, record := range records[3:] {
		require.NotContains(t, string(record.Projection.Payload), `"type":"tool_call"`)
		require.Equal(t, prompty.OutcomeCompleted, record.Projection.Metadata.Outcome)
	}
	for _, record := range records {
		require.Equal(t, prompty.CaptureSemantic, record.Projection.Accuracy)
		require.Empty(t, record.PhysicalAttemptID)
		require.NotContains(t, string(record.Projection.Payload), "private-credential")
	}
}
