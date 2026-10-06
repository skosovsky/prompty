package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type stateDispatchSink struct {
	expires time.Time
	records []prompty.CaptureRecord
}

func (*stateDispatchSink) Capabilities() prompty.CaptureSinkCapabilities {
	return prompty.CaptureSinkCapabilities{}
}

func (s *stateDispatchSink) WriteCapture(_ context.Context, record prompty.CaptureRecord) (prompty.CaptureAck, error) {
	s.records = append(s.records, record)
	if record.Stage == prompty.CaptureTranslatedRequest {
		waitForStateDispatchExpiry(s.expires)
	}
	return prompty.CaptureAck{}, nil
}

func waitForStateDispatchExpiry(expires time.Time) {
	if delay := time.Until(expires.Add(time.Millisecond)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
	}
}

func stateDispatchFixture(t *testing.T) (*Adapter, *httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"unexpected dispatch"}],"stop_reason":"end_turn"}`)
	}))
	t.Cleanup(server.Close)
	client := sdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local state dispatch fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState: prompty.CapabilitySupported,
			prompty.CapabilityNativeStream:  prompty.CapabilitySupported,
			prompty.CapabilityBatchSubmit:   prompty.CapabilitySupported,
		},
	}
	return New(WithClient(&client), WithModel("fixture"), WithProfiles(server.URL, profile)), server, calls
}

func stateDispatchExecution(t *testing.T, adp *Adapter) (*prompty.PromptExecution, time.Time) {
	t.Helper()
	var message sdk.Message
	require.NoError(t, message.UnmarshalJSON([]byte(
		`{"content":[{"type":"thinking","thinking":"summary","signature":"signature"}],"stop_reason":"end_turn"}`,
	)))
	response, err := adp.ParseResponseForProfile(&message, adp.profiles[0].Identity)
	require.NoError(t, err)
	expires := time.Now().Add(time.Second)
	response.ProviderState[0].Envelope.ExpiresAt = &expires
	part, err := response.AssistantMessage()
	require.NoError(t, err)
	return prompty.SimplePrompt("fixture").AddMessage(part), expires
}

func TestActualSDKStateExpiryImmediatelyBeforeGenerationDispatch(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: capture blocks sync/buffered; native pauses without any sink.
			adp, _, calls := stateDispatchFixture(t)
			exec, expires := stateDispatchExecution(t, adp)
			sink := &stateDispatchSink{expires: expires}
			if mode != prompty.StreamNative {
				exec.Capture = &prompty.CaptureBinding{Sink: sink}
			}
			client := adapter.NewClient(adp)
			// Act.
			var err error
			if mode == "" {
				_, err = client.Execute(t.Context(), exec)
			} else {
				stream := client.ExecuteStream(t.Context(), exec, mode)
				terminals := 0
				for frame := range stream.Events() {
					if frame != nil && frame.Kind == prompty.StreamStart && mode == prompty.StreamNative {
						waitForStateDispatchExpiry(expires)
						// Caller mutation cannot extend the already-prepared state lease.
						future := time.Now().Add(time.Hour)
						exec.Messages[1].ProviderState[0].Envelope.ExpiresAt = &future
					}
					if frame != nil && frame.Kind == prompty.StreamTerminal {
						terminals++
					}
				}
				err = stream.Status().Err
				require.Equal(t, prompty.StreamFailed, stream.Status().State)
				require.Equal(t, 1, terminals)
			}
			// Assert: expired immutable state is a local failure, not a remote attempt.
			require.ErrorIs(t, err, prompty.ErrStateExpired)
			require.Zero(t, calls.Load())
			if mode != prompty.StreamNative {
				require.Equal(t, prompty.CaptureTerminal, sink.records[len(sink.records)-1].Stage)
				require.False(t, sink.records[len(sink.records)-1].AfterDispatch)
			}
		})
	}
}

func TestActualSDKBatchStateExpiryBeforeSubmit(t *testing.T) {
	t.Parallel()
	// Arrange: batch capture outlives the prepared original item's state lease.
	adp, server, calls := stateDispatchFixture(t)
	exec, expires := stateDispatchExecution(t, adp)
	sink := &stateDispatchSink{expires: expires}
	manager := NewMessageBatches("account", WithClient(adp.client), WithModel("fixture"),
		WithProfiles(server.URL, adp.profiles[0]))
	// Act.
	result, err := manager.Submit(t.Context(), prompty.BatchSubmitRequest{
		Items:   []prompty.BatchItem{{ID: "item", AttemptID: "attempt", Execution: exec}},
		Options: prompty.BatchOperationOptions{Binding: &prompty.CaptureBinding{Sink: sink}},
	})
	// Assert: no job acceptance, identity, refresh or second attempt is invented.
	require.ErrorIs(t, err, prompty.ErrStateExpired)
	require.NotErrorIs(t, err, prompty.ErrBatchUnknown)
	require.Zero(t, calls.Load())
	require.False(t, result.SubmissionAttempted)
	require.Nil(t, result.Handle)
	require.True(t, result.Capture.Closed)
	require.False(t, sink.records[len(sink.records)-1].AfterDispatch)
}
