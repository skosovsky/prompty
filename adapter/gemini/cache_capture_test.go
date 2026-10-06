package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type cacheCaptureSink struct {
	mu        sync.Mutex
	records   []prompty.CaptureRecord
	failStage prompty.CaptureStage
	cancel    context.CancelFunc
	durable   bool
}

func (s *cacheCaptureSink) Capabilities() prompty.CaptureSinkCapabilities {
	return prompty.CaptureSinkCapabilities{Durable: s.durable}
}
func (s *cacheCaptureSink) WriteCapture(_ context.Context, record prompty.CaptureRecord) (prompty.CaptureAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	if s.cancel != nil && record.Stage == prompty.CaptureTranslatedRequest {
		s.cancel()
	}
	if record.Stage == s.failStage {
		return prompty.CaptureAck{}, errors.New("sink-secret")
	}
	return prompty.CaptureAck{Durable: s.durable}, nil
}

type cacheCaptureProjector struct{}

func (cacheCaptureProjector) MaxAccuracy() prompty.CaptureAccuracy { return prompty.CaptureSemantic }

func (cacheCaptureProjector) ProjectCapture(
	_ context.Context,
	record prompty.CaptureRecord,
) (prompty.CaptureProjection, error) {
	return record.Projection, nil
}

func cacheCaptureServer(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = fmt.Fprintf(
			w,
			`{"name":"cachedContents/fixture","model":"models/fixture","expireTime":%q,"usageMetadata":{"totalTokenCount":0},"provider_extension":"original"}`,
			expires,
		)
	}))
	t.Cleanup(server.Close)
	return server
}

func captureCacheExecution(sink *cacheCaptureSink) *prompty.PromptExecution {
	exec := prompty.SimplePrompt("immutable sensitive source")
	exec.Capture = &prompty.CaptureBinding{Sink: sink, Projector: cacheCaptureProjector{}}
	exec.Requirements.Capture = prompty.CaptureRequirement{
		Accuracy: prompty.CaptureSemantic,
		Stages: []prompty.CaptureStage{
			prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderResponse,
			prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
		},
	}
	return exec
}

func TestCacheCapturePartialSourceMetadataAndSemanticRequirement(t *testing.T) {
	t.Parallel()
	for _, accuracy := range []prompty.CaptureAccuracy{prompty.CaptureMetadata, prompty.CaptureSemantic} {
		t.Run(string(accuracy), func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK response observation overflows; HTTP status is still observed.
			var calls atomic.Int32
			server := cacheCaptureServer(t, &calls)
			manager := cacheResourceFixtureManager(t, server)
			manager.base.responseLimit = 20
			sink := &cacheCaptureSink{}
			exec := captureCacheExecution(sink)
			exec.Requirements.Capture = prompty.CaptureRequirement{
				Accuracy: accuracy,
				Stages:   []prompty.CaptureStage{prompty.CaptureProviderResponse},
			}
			// Act.
			result, err := manager.Create(
				context.Background(),
				prompty.CacheResourceCreateRequest{
					Execution: exec,
					Expiry:    prompty.CacheResourceExpiry{TTL: time.Hour},
				},
			)
			// Assert: no truncated JSON is mislabeled full semantic capture or success.
			require.ErrorIs(t, err, prompty.ErrCacheResourceUnknown)
			if accuracy == prompty.CaptureSemantic {
				require.ErrorIs(t, err, prompty.ErrCapture)
			} else {
				require.NotErrorIs(t, err, prompty.ErrCapture)
			}
			require.False(t, result.Diagnostics.Capture.Complete)
			require.EqualValues(t, 1, calls.Load())
			if accuracy == prompty.CaptureMetadata {
				provider := sink.records[2]
				require.Empty(t, provider.Projection.Payload)
				require.Equal(t, prompty.CaptureMetadata, provider.Projection.Accuracy)
				require.False(t, provider.Projection.Complete)
				require.Equal(t, http.StatusOK, *provider.Projection.Metadata.HTTPStatus)
			}
		})
	}
}

type cacheFalseAckSink struct{ cacheCaptureSink }

func (s *cacheFalseAckSink) WriteCapture(
	ctx context.Context,
	record prompty.CaptureRecord,
) (prompty.CaptureAck, error) {
	_, err := s.cacheCaptureSink.WriteCapture(ctx, record)
	return prompty.CaptureAck{Durable: false}, err
}

func TestCacheCaptureDurableAckAndOptionCardinality(t *testing.T) {
	t.Parallel()
	// Arrange: a declared durable sink does not make an undurable acknowledgement valid.
	var calls atomic.Int32
	server := cacheCaptureServer(t, &calls)
	manager := cacheResourceFixtureManager(t, server)
	sink := &cacheFalseAckSink{cacheCaptureSink: cacheCaptureSink{durable: true}}
	exec := captureCacheExecution(&sink.cacheCaptureSink)
	exec.Capture.Sink = sink
	exec.Requirements.Capture.Durable = true
	// Act.
	result, err := manager.Create(
		context.Background(),
		prompty.CacheResourceCreateRequest{Execution: exec, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
	)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrCapture)
	require.Nil(t, result.Handle)
	require.Zero(t, calls.Load())
	handle := cacheResourceTestHandle(server.URL)
	_, err = manager.Get(
		context.Background(),
		handle,
		prompty.CacheResourceOperationOptions{},
		prompty.CacheResourceOperationOptions{},
	)
	require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
	require.Zero(t, calls.Load())
}

func TestCacheCreateCaptureStagesAndOriginalSource(t *testing.T) {
	t.Parallel()
	// Arrange: source observation precedes SDK dropping unknown fields.
	var calls atomic.Int32
	server := cacheCaptureServer(t, &calls)
	manager := cacheResourceFixtureManager(t, server)
	sink := &cacheCaptureSink{}
	exec := captureCacheExecution(sink)
	// Act.
	result, err := manager.Create(
		context.Background(),
		prompty.CacheResourceCreateRequest{Execution: exec, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
	)
	// Assert: one logical invocation and semantic—not transport/attempt—coverage.
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, sink.records, 5)
	report := result.Diagnostics.Capture
	require.NotNil(t, report)
	require.True(t, report.Closed)
	require.True(t, report.Complete)
	for i, record := range sink.records {
		require.EqualValues(t, i+1, record.Sequence)
		require.Equal(t, sink.records[0].InvocationID, record.InvocationID)
		require.Empty(t, record.PhysicalAttemptID)
		require.Empty(t, record.Profile.Endpoint)
		require.Equal(t, i >= 2, record.AfterDispatch)
	}
	require.Contains(t, string(sink.records[2].Projection.Payload), `"provider_extension":"original"`)
	require.Equal(t, http.StatusOK, *sink.records[2].Projection.Metadata.HTTPStatus)
	require.Contains(t, string(sink.records[0].Projection.Payload), "immutable sensitive source")
	require.Contains(t, string(sink.records[1].Projection.Payload), `"ttl":"3600s"`)
	var terminal struct {
		Result *prompty.CacheResourceResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal(sink.records[4].Projection.Payload, &terminal))
	require.Equal(t, prompty.CacheResourceActive, terminal.Result.Handle.State)
}

func TestCacheCaptureRequiredFailureBeforeAndAfterDispatch(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := cacheCaptureServer(t, &calls)
			manager := cacheResourceFixtureManager(t, server)
			sink := &cacheCaptureSink{failStage: stage}
			// Act.
			result, err := manager.Create(
				context.Background(),
				prompty.CacheResourceCreateRequest{
					Execution: captureCacheExecution(sink),
					Expiry:    prompty.CacheResourceExpiry{TTL: time.Hour},
				},
			)
			// Assert: capture cannot erase remote success or authorize its repetition.
			require.ErrorIs(t, err, prompty.ErrCapture)
			require.NotContains(t, err.Error(), "sink-secret")
			var captureErr *prompty.CaptureError
			require.ErrorAs(t, err, &captureErr)
			before := stage == prompty.CaptureExecution || stage == prompty.CaptureTranslatedRequest
			require.Equal(t, !before, captureErr.AfterDispatch)
			if before {
				require.Zero(t, calls.Load())
				require.Nil(t, result.Handle)
			} else {
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, prompty.CacheResourceActive, result.Handle.State)
			}
			require.NotNil(t, result.Diagnostics.Capture)
			require.False(t, result.Diagnostics.Capture.Complete)
		})
	}
}

func TestCacheCaptureInspectUpdateDeleteAndDefaultRedaction(t *testing.T) {
	t.Parallel()
	// Arrange: default projection delivers no provider/host payload.
	var calls atomic.Int32
	server := cacheCaptureServer(t, &calls)
	manager := cacheResourceFixtureManager(t, server)
	sink := &cacheCaptureSink{}
	options := prompty.CacheResourceOperationOptions{Binding: &prompty.CaptureBinding{Sink: sink}}
	handle := cacheResourceTestHandle(server.URL)
	// Act.
	inspected, err := manager.Get(context.Background(), handle, options)
	require.NoError(t, err)
	updated, err := manager.UpdateExpiry(
		context.Background(),
		*inspected.Handle,
		prompty.CacheResourceExpiry{TTL: time.Hour},
		options,
	)
	require.NoError(t, err)
	deleted, err := manager.Delete(context.Background(), *updated.Handle, options)
	// Assert: each independent call has one sequence/terminal; no raw reference leaks.
	require.NoError(t, err)
	require.Equal(t, prompty.CacheResourceDeleted, deleted.Handle.State)
	require.Len(t, sink.records, 15)
	for i, record := range sink.records {
		require.Empty(t, record.Projection.Payload)
		require.Empty(t, record.Profile.Endpoint)
		require.Empty(t, record.Projection.Metadata.ProviderResponseID)
		require.EqualValues(t, i%5+1, record.Sequence)
	}
	require.NotEqual(t, sink.records[0].InvocationID, sink.records[5].InvocationID)
	require.NotEqual(t, sink.records[5].InvocationID, sink.records[10].InvocationID)
	require.True(t, deleted.Diagnostics.Capture.Closed)
	require.False(t, deleted.Diagnostics.Capture.Complete)
	require.EqualValues(t, 3, calls.Load())
}

func TestCacheCaptureUnsupportedAndCanceledBeforeDispatch(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := cacheCaptureServer(t, &calls)
	manager := cacheResourceFixtureManager(t, server)
	for _, requirement := range []prompty.CaptureRequirement{{Accuracy: prompty.CaptureBytes}, {PhysicalAttempts: true}, {Stages: []prompty.CaptureStage{prompty.CaptureTransportRequest}}} {
		exec := captureCacheExecution(&cacheCaptureSink{})
		exec.Requirements.Capture = requirement
		// Act.
		_, err := manager.Create(
			context.Background(),
			prompty.CacheResourceCreateRequest{Execution: exec, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
		)
		// Assert.
		require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &cacheCaptureSink{cancel: cancel}
	result, err := manager.Create(
		ctx,
		prompty.CacheResourceCreateRequest{
			Execution: captureCacheExecution(sink),
			Expiry:    prompty.CacheResourceExpiry{TTL: time.Hour},
		},
	)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result.Handle)
	require.Zero(t, calls.Load())
}
