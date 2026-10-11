package openai

import (
	"context"
	"encoding/json"
	"errors"
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

type captureFixtureSink struct {
	write func(prompty.CaptureRecord) error
}

func (captureFixtureSink) Capabilities() prompty.CaptureSinkCapabilities {
	return prompty.CaptureSinkCapabilities{}
}

func (s captureFixtureSink) WriteCapture(_ context.Context, record prompty.CaptureRecord) (prompty.CaptureAck, error) {
	return prompty.CaptureAck{}, s.write(record)
}

type captureFixtureProjector struct{}

func (captureFixtureProjector) MaxAccuracy() prompty.CaptureAccuracy { return prompty.CaptureSemantic }

func (captureFixtureProjector) ProjectCapture(
	_ context.Context, record prompty.CaptureRecord,
) (prompty.CaptureProjection, error) {
	return record.Projection, nil
}

const captureResponseFixture = `{"id":"fixture-id","choices":[{"index":0,` +
	`"message":{"role":"assistant","content":"safe"},"finish_reason":"stop"}],` +
	`"usage":{"total_tokens":9},"provider_extension":{"retained":true}}`

func TestBufferedCaptureUsesTheSameStreamIdentityAndOneTerminal(t *testing.T) {
	t.Parallel()
	// Arrange: the underlying provider request remains synchronous, but canonical closeout belongs to the handle.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, captureResponseFixture)
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL)))
	var records []prompty.CaptureRecord
	exec := prompty.SimplePrompt("fixture")
	exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{},
		Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
			records = append(records, record)
			return nil
		}}}
	exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic,
		Stages: []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest,
			prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal}}
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamBuffered)
	messageID := stream.Status().Identity.MessageID
	require.Zero(t, calls.Load())
	// Act.
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		require.Equal(t, messageID, frame.Identity.MessageID)
		if frame.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	// Assert: no earlier sync canonical terminal or independent message identity was recorded.
	status := stream.Status()
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.StreamBuffered, status.Mode)
	require.Equal(t, []string{"buffered:0"}, status.Identity.PartIDs)
	require.Len(t, records, 5)
	require.Equal(t, 1, terminals)
	require.Equal(t, int32(1), calls.Load())
	require.True(t, status.Result.Diagnostics.Capture.Complete)
	require.True(t, status.Result.Diagnostics.Capture.Closed)
	for _, index := range []int{3, 4} {
		var envelope struct {
			Identity prompty.StreamIdentity `json:"stream_identity"`
		}
		require.NoError(t, json.Unmarshal(records[index].Projection.Payload, &envelope))
		require.Equal(t, status.Identity, envelope.Identity)
	}
	require.Equal(t, prompty.CaptureCanonicalParts, records[3].Stage)
	require.Equal(t, prompty.CaptureTerminal, records[4].Stage)
}

func TestBufferedCaptureFailureKeepsPartialIdentityWithoutDuplicateCloseout(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{
		prompty.CaptureExecution, prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
	} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			// Arrange: required sink failure before/after dispatch must not replay or create a second terminal record.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, captureResponseFixture)
			}))
			defer server.Close()
			sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL)))
			sentinel := errors.New("fixture required sink failure")
			recordedTerminals := 0
			exec := prompty.SimplePrompt("fixture")
			exec.Requirements.Capture.Stages = []prompty.CaptureStage{stage}
			exec.Capture = &prompty.CaptureBinding{
				Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
					if record.Stage == prompty.CaptureTerminal {
						recordedTerminals++
					}
					if record.Stage == stage {
						return sentinel
					}
					return nil
				}},
			}
			stream := client.ExecuteStream(context.Background(), exec, prompty.StreamBuffered)
			messageID := stream.Status().Identity.MessageID
			// Act.
			terminals := 0
			for frame := range stream.Events() {
				if frame.Kind == prompty.StreamTerminal {
					terminals++
				}
			}
			// Assert.
			status := stream.Status()
			require.Equal(t, prompty.StreamFailed, status.State)
			require.ErrorIs(t, status.Err, sentinel)
			require.Equal(t, messageID, status.Identity.MessageID)
			require.Equal(t, 1, terminals)
			require.Equal(t, 1, recordedTerminals)
			if stage == prompty.CaptureExecution {
				require.Zero(t, calls.Load())
				require.Empty(t, status.Result.Content)
				require.Empty(t, status.Identity.PartIDs)
			} else {
				require.Equal(t, int32(1), calls.Load())
				require.Equal(t, []string{"buffered:0"}, status.Identity.PartIDs)
				require.Len(t, status.Result.Content, 1)
				require.Equal(t, 9, status.Result.Usage.TotalTokens)
			}
		})
	}
}

func TestCheckedSyncCaptureSeparatesStagesAndPreservesProviderJSON(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, captureResponseFixture)
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("private-credential"))
	client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL)))
	var records []prompty.CaptureRecord
	exec := prompty.SimplePrompt("fixture")
	exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{}, CorrelationID: "caller",
		Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
			records = append(records, record)
			return nil
		}}}
	exec.Requirements.Capture = prompty.CaptureRequirement{
		Stages: []prompty.CaptureStage{
			prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderResponse,
			prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
		}, Accuracy: prompty.CaptureSemantic,
	}
	// Act.
	response, err := client.Execute(context.Background(), exec)
	// Assert.
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, records, 5)
	require.True(t, response.Diagnostics.Capture.Complete)
	require.True(t, response.Diagnostics.Capture.Closed)
	require.Contains(t, response.Diagnostics.Capture.SDKBuild, "github.com/openai/openai-go/v3@")
	var canonical, translated map[string]any
	require.NoError(t, json.Unmarshal(records[0].Projection.Payload, &canonical))
	require.NoError(t, json.Unmarshal(records[1].Projection.Payload, &translated))
	require.NotContains(t, canonical, "model")
	require.Equal(t, "fixture", translated["model"])
	require.JSONEq(t, captureResponseFixture, string(records[2].Projection.Payload))
	require.Equal(t, "fixture-id", records[2].Projection.Metadata.ProviderResponseID)
	require.Empty(t, records[2].Projection.Metadata.RequestID)
	require.Nil(t, records[2].Projection.Metadata.HTTPStatus)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, records[2].Projection.Metadata.Usage.Known)
	require.Equal(t, 9, records[2].Projection.Metadata.Usage.TotalTokens)
	require.Equal(t, response.Usage, *records[4].Projection.Metadata.Usage)
	require.Equal(t, response.Outcome, records[4].Projection.Metadata.Outcome)
	for index, record := range records {
		require.EqualValues(t, index+1, record.Sequence)
		require.Equal(t, response.Diagnostics.Capture.InvocationID, record.InvocationID)
		require.Equal(t, "caller", record.CorrelationID)
		require.Empty(t, record.PhysicalAttemptID)
		require.NotContains(t, string(record.Projection.Payload), "private-credential")
	}
}

func TestRequiredSyncCaptureFailureDoesNotReplayOrLoseKnownResponse(t *testing.T) {
	t.Parallel()
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before dispatch", true: "after dispatch"}[after], func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, captureResponseFixture)
			}))
			defer server.Close()
			sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			client := adapter.NewClient(New(WithClient(&sdk), WithProfiles(server.URL)))
			stage := prompty.CaptureExecution
			if after {
				stage = prompty.CaptureProviderResponse
			}
			sentinel := errors.New("sink failure with secret")
			exec := prompty.SimplePrompt("fixture")
			exec.Requirements.Capture.Stages = []prompty.CaptureStage{stage}
			sink := captureFixtureSink{write: func(record prompty.CaptureRecord) error {
				if record.Stage == stage {
					return sentinel
				}
				return nil
			}}
			exec.Capture = &prompty.CaptureBinding{Sink: sink}
			// Act.
			response, err := client.Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, sentinel)
			var captureError *prompty.CaptureError
			require.ErrorAs(t, err, &captureError)
			require.Equal(t, after, captureError.AfterDispatch)
			require.NotContains(t, err.Error(), "secret")
			if after {
				require.EqualValues(t, 1, calls.Load())
				require.NotNil(t, response)
				require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
				require.Equal(t, 9, response.Usage.TotalTokens)
				require.False(t, response.Diagnostics.Capture.Complete)
			} else {
				require.Zero(t, calls.Load())
				require.Nil(t, response)
			}
		})
	}
}

func TestRequiredUnsupportedCaptureDoesNotDispatch(t *testing.T) {
	t.Parallel()
	for _, requirement := range []prompty.CaptureRequirement{
		{Stages: []prompty.CaptureStage{prompty.CaptureTransportRequest}},
		{Stages: []prompty.CaptureStage{prompty.CaptureTransportResponse}},
		{PhysicalAttempts: true},
		{Accuracy: prompty.CaptureBytes},
		{Durable: true},
	} {
		// Arrange.
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			http.Error(w, "must not dispatch", http.StatusInternalServerError)
		}))
		sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
		client := adapter.NewClient(New(WithClient(&sdk), WithProfiles(server.URL)))
		exec := prompty.SimplePrompt("fixture")
		exec.Requirements.Capture = requirement
		exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{},
			Sink: captureFixtureSink{write: func(prompty.CaptureRecord) error { return nil }}}
		// Act.
		_, err := client.Execute(context.Background(), exec)
		server.Close()
		// Assert.
		require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
		require.Zero(t, calls.Load())
	}
}
