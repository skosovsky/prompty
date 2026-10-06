package prompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceCleanupDoesNotCancelCaptureObservation(t *testing.T) {
	t.Parallel()
	// Arrange: the producer waits for source cancellation after its error yield.
	sentinel := errors.New("transport failure")
	observed := 0
	stream := NewStream(
		context.Background(),
		StreamNative,
		func(sourceCtx context.Context) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				if !yield(
					&ResponseChunk{
						Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
						Kind:     StreamDelta,
						Content:  []ContentPart{TextPart{Text: "partial"}},
					},
					nil,
				) {
					return
				}
				yield(nil, sentinel)
				<-sourceCtx.Done()
			}
		},
		WithStreamCaptureObserver(
			func(ctx context.Context, status StreamStatus, sourceErr error) (*CaptureReport, error) {
				observed++
				require.NoError(t, ctx.Err())
				require.ErrorIs(t, sourceErr, sentinel)
				require.Len(t, status.Result.Content, 1)
				require.True(t, status.Result.ContinuationUnavailable)
				status.Result.Content[0] = TextPart{Text: "mutated"}
				report := validFixtureCaptureReport()
				return &report, nil
			},
		),
	)
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == StreamTerminal {
			terminals++
			require.ErrorIs(t, err, sentinel)
		}
	}
	// Assert.
	require.Equal(t, 1, observed)
	require.Equal(t, 1, terminals)
	status := stream.Status()
	require.Equal(t, StreamFailed, status.State)
	require.Equal(t, TextPart{Text: "partial"}, status.Result.Content[0])
	require.True(t, status.Result.Diagnostics.Capture.Closed)
}

func TestCaptureObservationFollowsFinalValidationAndUsage(t *testing.T) {
	t.Parallel()
	// Arrange.
	finalized := false
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			if !yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil) {
				return
			}
			yield(
				&ResponseChunk{Kind: StreamUsage, Usage: Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}}},
				nil,
			)
		}
	}, WithStreamFinalizer(func(context.Context, *Response) error {
		finalized = true
		return ErrSchemaMismatch
	}), WithStreamCaptureObserver(func(_ context.Context, status StreamStatus, finalErr error) (*CaptureReport, error) {
		require.True(t, finalized)
		require.ErrorIs(t, finalErr, ErrSchemaMismatch)
		require.Equal(t, 9, status.Result.Usage.TotalTokens)
		report := validFixtureCaptureReport()
		return &report, nil
	}))
	// Act.
	for event, err := range stream.Events() {
		if event.Kind == StreamTerminal {
			require.ErrorIs(t, err, ErrSchemaMismatch)
		}
	}
	// Assert.
	require.Equal(t, StreamFailed, stream.Status().State)
	require.ErrorIs(t, stream.Status().Err, ErrSchemaMismatch)
}

func TestCloseCancelsCaptureObserverWithoutHeldStatusMutex(t *testing.T) {
	t.Parallel()
	// Arrange.
	entered, done := make(chan struct{}), make(chan struct{})
	var terminalErr error
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil)
		}
	}, WithStreamCaptureObserver(func(ctx context.Context, _ StreamStatus, _ error) (*CaptureReport, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	go func() {
		defer close(done)
		for event, err := range stream.Events() {
			if event.Kind == StreamTerminal {
				terminalErr = err
			}
		}
	}()
	// Act.
	<-entered
	require.Equal(t, StreamRunning, stream.Status().State)
	require.NoError(t, stream.Close())
	<-done
	// Assert.
	require.Equal(t, StreamCanceled, stream.Status().State)
	require.ErrorIs(t, stream.Status().Err, context.Canceled)
	require.ErrorIs(t, terminalErr, context.Canceled)
}
