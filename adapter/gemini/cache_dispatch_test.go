package gemini

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type cacheExpirySink struct {
	*cacheCaptureSink

	expires time.Time
}

func (s *cacheExpirySink) WriteCapture(ctx context.Context, record prompty.CaptureRecord) (prompty.CaptureAck, error) {
	if record.Stage == prompty.CaptureTranslatedRequest {
		waitForCacheExpiry(s.expires)
	}
	return s.cacheCaptureSink.WriteCapture(ctx, record)
}

func waitForCacheExpiry(expires time.Time) {
	if delay := time.Until(expires.Add(time.Millisecond)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
	}
}

func TestCacheReuseExpiryAtDispatch(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: a valid handle expires during capture or native consumer pause.
			var calls atomic.Int32
			server := cacheCaptureServer(t, &calls)
			manager := cacheResourceFixtureManager(t, server)
			profile := manager.base.RuntimeDescriptor().Profiles[0]
			profile.Capabilities[prompty.CapabilityCacheReuse] = prompty.CapabilitySupported
			profile.Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			adp := New(WithClient(manager.base.client), WithModel("fixture"),
				WithProfiles(server.URL, profile), WithCacheAccountBinding("account-fixture"))
			handle := cacheResourceTestHandle(server.URL)
			handle.ExpiresAt = time.Now().Add(time.Second)
			sink := &cacheExpirySink{cacheCaptureSink: &cacheCaptureSink{}, expires: handle.ExpiresAt}
			exec := prompty.SimplePrompt("question")
			exec.CacheResource = &handle
			if mode == prompty.StreamNative {
				// Delay only after StreamStart, not during capture.
				sink.expires = time.Time{}
			}
			exec.Capture = &prompty.CaptureBinding{Sink: sink}
			client := adapter.NewClient(adp)
			// Act.
			var resultErr error
			if mode == "" {
				_, resultErr = client.Execute(t.Context(), exec)
			} else {
				stream := client.ExecuteStream(t.Context(), exec, mode)
				for frame, streamErr := range stream.Events() {
					if frame != nil && frame.Kind == prompty.StreamStart {
						waitForCacheExpiry(handle.ExpiresAt)
					}
					if streamErr != nil {
						resultErr = streamErr
					}
				}
				require.ErrorIs(t, stream.Status().Err, prompty.ErrCacheResourceExpired)
			}
			// Assert: no provider dispatch or hidden recache; one pre-dispatch terminal.
			require.ErrorIs(t, resultErr, prompty.ErrCacheResourceExpired)
			require.Zero(t, calls.Load())
			terminals := 0
			for _, record := range sink.records {
				if record.Stage == prompty.CaptureTerminal {
					terminals++
					require.False(t, record.AfterDispatch)
				}
			}
			require.Equal(t, 1, terminals)
			require.Equal(t, prompty.CacheResourceActive, handle.State)
		})
	}
}

func TestCacheAbsoluteExpiryAtMutationDispatch(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			// Arrange: initial future expiry becomes invalid inside a host callback.
			var calls atomic.Int32
			server := cacheCaptureServer(t, &calls)
			manager := cacheResourceFixtureManager(t, server)
			expiry := prompty.CacheResourceExpiry{ExpiresAt: time.Now().Add(time.Second)}
			sink := &cacheExpirySink{cacheCaptureSink: &cacheCaptureSink{}, expires: expiry.ExpiresAt}
			binding := &prompty.CaptureBinding{Sink: sink}
			// Act.
			var result *prompty.CacheResourceResult
			var err error
			if operation == "create" {
				exec := prompty.SimplePrompt("immutable document")
				exec.Capture = binding
				result, err = manager.Create(
					t.Context(),
					prompty.CacheResourceCreateRequest{Execution: exec, Expiry: expiry},
				)
			} else {
				result, err = manager.UpdateExpiry(t.Context(), cacheResourceTestHandle(server.URL), expiry,
					prompty.CacheResourceOperationOptions{Binding: binding})
			}
			// Assert: expired absolute intent is not submitted as an unknown mutation.
			require.ErrorIs(t, err, prompty.ErrCacheResourceExpiry)
			require.NotErrorIs(t, err, prompty.ErrCacheResourceUnknown)
			require.Zero(t, calls.Load())
			require.NotNil(t, result.Diagnostics.Capture)
			require.True(t, result.Diagnostics.Capture.Closed)
			require.False(t, sink.records[len(sink.records)-1].AfterDispatch)
		})
	}
}

func TestCacheCreateRejectsStateExpiredDuringCapture(t *testing.T) {
	t.Parallel()
	// Arrange: valid signed input expires after translation inside host capture.
	var calls atomic.Int32
	server := cacheCaptureServer(t, &calls)
	manager := cacheResourceFixtureManager(t, server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	response, err := manager.base.ParseResponseForProfile(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "signed source", ThoughtSignature: []byte("signature")}},
			},
			FinishReason: genai.FinishReasonStop,
		}},
	}, manager.base.profiles[0].Identity)
	require.NoError(t, err)
	expires := time.Now().Add(time.Second)
	response.ProviderState[0].Envelope.ExpiresAt = &expires
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	exec := prompty.SimplePrompt("immutable source").AddMessage(message)
	sink := &cacheExpirySink{cacheCaptureSink: &cacheCaptureSink{}, expires: expires}
	exec.Capture = &prompty.CaptureBinding{Sink: sink}
	// Act.
	result, err := manager.Create(t.Context(), prompty.CacheResourceCreateRequest{
		Execution: exec, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour},
	})
	// Assert: no expired signed payload or unknown remote mutation is dispatched.
	require.ErrorIs(t, err, prompty.ErrStateExpired)
	require.NotErrorIs(t, err, prompty.ErrCacheResourceUnknown)
	require.Zero(t, calls.Load())
	require.Nil(t, result.Handle)
	require.True(t, result.Diagnostics.Capture.Closed)
	require.False(t, sink.records[len(sink.records)-1].AfterDispatch)
}
