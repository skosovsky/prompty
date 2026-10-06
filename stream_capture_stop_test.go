package prompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumerBreakClosesCaptureAndPreservesObservationErrors(t *testing.T) {
	t.Parallel()
	frameFailure := errors.New("frame observation failed")
	closeoutFailure := errors.New("capture closeout failed")
	for _, frameErr := range []error{nil, frameFailure} {
		for _, captureErr := range []error{nil, closeoutFailure} {
			t.Run(errors.Join(frameErr, captureErr, context.Canceled).Error(), func(t *testing.T) {
				t.Parallel()
				checkConsumerBreakCapture(t, frameErr, captureErr)
			})
		}
	}
}

func checkConsumerBreakCapture(t *testing.T, frameErr, captureErr error) {
	t.Helper()
	// Arrange: the stopped producer must unwind before capture sees the snapshot.
	cleaned, finalized := false, false
	observed, yielded := 0, 0
	stream := NewStream(
		context.Background(),
		StreamNative,
		func(sourceCtx context.Context) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				defer func() { cleaned = true }()
				if !yield(&ResponseChunk{Kind: StreamDelta,
					Identity: StreamIdentity{PartIDs: []string{"part"}},
					Content:  []ContentPart{TextPart{Text: "partial"}}}, nil) {
					require.ErrorIs(t, sourceCtx.Err(), context.Canceled)
					return
				}
				t.Fatal("producer continued after consumer stop")
			}
		},
		WithStreamFrameObserver(func(context.Context, *ResponseChunk) error { return frameErr }),
		WithStreamFinalizer(func(context.Context, *Response) error { finalized = true; return nil }),
		WithStreamCaptureObserver(
			func(ctx context.Context, status StreamStatus, err error) (*CaptureReport, error) {
				observed++
				require.True(t, cleaned)
				require.NoError(t, ctx.Err())
				require.ErrorIs(t, err, context.Canceled)
				if frameErr != nil {
					require.ErrorIs(t, err, frameErr)
				}
				require.Len(t, status.Result.Content, 1)
				report := validFixtureCaptureReport()
				return &report, captureErr
			},
		),
	)
	// Act: false is final; no terminal event may be sent to this consumer.
	for range stream.Events() {
		yielded++
		break
	}
	// Assert: one closeout, readable immutable partial/status and all causes.
	status := stream.Status()
	require.Equal(t, 1, yielded)
	require.Equal(t, 1, observed)
	require.False(t, finalized)
	require.Equal(t, StreamCanceled, status.State)
	require.ErrorIs(t, status.Err, context.Canceled)
	if frameErr != nil {
		require.ErrorIs(t, status.Err, frameErr)
	}
	if captureErr != nil {
		require.ErrorIs(t, status.Err, captureErr)
	}
	require.NotNil(t, status.Result.Diagnostics.Capture)
	require.True(t, status.Result.Diagnostics.Capture.Closed)
	require.NoError(t, stream.Close())
	require.Equal(t, status, stream.Status())
}
