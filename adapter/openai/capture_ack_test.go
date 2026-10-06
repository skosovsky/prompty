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

type durableCaptureFixtureSink struct {
	declared bool
	write    func(prompty.CaptureRecord) prompty.CaptureAck
}

func (s durableCaptureFixtureSink) Capabilities() prompty.CaptureSinkCapabilities {
	return prompty.CaptureSinkCapabilities{Durable: s.declared}
}

func (s durableCaptureFixtureSink) WriteCapture(
	_ context.Context,
	record prompty.CaptureRecord,
) (prompty.CaptureAck, error) {
	return s.write(record), nil
}

func TestCheckedCaptureDurableAcknowledgementsBeforeAndAfterDispatch(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, scenario := range []string{"undeclared", "before dispatch", "after dispatch", "confirmed"} {
			t.Run(string(mode)+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				checkedDurableAckConformance(t, mode, scenario)
			})
		}
	}
}

func checkedDurableAckConformance(t *testing.T, mode prompty.StreamMode, scenario string) {
	t.Helper()
	// Arrange: actual SDK transport, configured retries cannot replay an ack failure.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if mode == prompty.StreamNative {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range []string{nativeCaptureDelta,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`{"choices":[],"usage":{"total_tokens":9}}`, "[DONE]"} {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, captureResponseFixture)
		}
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"), option.WithMaxRetries(3))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK durable acknowledgement fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	failStage := prompty.CaptureExecution
	if scenario == "after dispatch" {
		failStage = prompty.CaptureProviderResponse
		if mode == prompty.StreamNative {
			failStage = prompty.CaptureProviderChunk
		}
	}
	writes := 0
	exec := prompty.SimplePrompt("fixture")
	exec.Requirements.Capture = prompty.CaptureRequirement{Durable: true, Accuracy: prompty.CaptureSemantic}
	exec.Capture = &prompty.CaptureBinding{
		Projector: captureFixtureProjector{},
		Sink: durableCaptureFixtureSink{
			declared: scenario != "undeclared",
			write: func(record prompty.CaptureRecord) prompty.CaptureAck {
				writes++
				return prompty.CaptureAck{Durable: scenario == "confirmed" || record.Stage != failStage}
			},
		},
	}
	// Act.
	var response *prompty.Response
	var err error
	if mode == "" {
		response, err = client.Execute(context.Background(), exec)
	} else {
		stream := client.ExecuteStream(context.Background(), exec, mode)
		require.Zero(t, calls.Load())
		for range stream.Events() {
		}
		status := stream.Status()
		response, err = status.Result, status.Err
		if scenario == "confirmed" {
			require.Equal(t, prompty.StreamCompleted, status.State)
		} else {
			require.Equal(t, prompty.StreamFailed, status.State)
		}
	}
	// Assert.
	assertDurableAckResult(t, scenario, mode, response, err, calls.Load(), writes)
}

func assertDurableAckResult(
	t *testing.T,
	scenario string,
	mode prompty.StreamMode,
	response *prompty.Response,
	err error,
	calls int32,
	writes int,
) {
	t.Helper()
	if scenario == "confirmed" {
		require.NoError(t, err)
		require.Equal(t, int32(1), calls)
		require.Equal(t, 9, response.Usage.TotalTokens)
		require.True(t, response.Diagnostics.Capture.Complete)
		return
	}
	require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
	if scenario == "undeclared" {
		require.Zero(t, writes)
		require.Zero(t, calls)
		return
	}
	var captureErr *prompty.CaptureError
	require.ErrorAs(t, err, &captureErr)
	require.Equal(t, scenario == "after dispatch", captureErr.AfterDispatch)
	if scenario == "before dispatch" {
		require.Zero(t, calls)
	} else {
		require.Equal(t, int32(1), calls)
		require.NotNil(t, response)
		require.NotEmpty(t, response.Content)
		require.False(t, response.Diagnostics.Capture.Complete)
		if mode != prompty.StreamNative {
			require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
		}
	}
}
