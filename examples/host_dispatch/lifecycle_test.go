package hostdispatch

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestCallbackCloseAndLifetimeJoin(t *testing.T) {
	t.Parallel()
	// Arrange: transport unwind is held by an explicit channel, not a timer.
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	callbackClosed := make(chan struct{})
	stream := prompty.NewStream(
		t.Context(),
		prompty.StreamNative,
		func(ctx context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				defer func() { close(cleanupStarted); <-releaseCleanup }()
				if !yield(
					&prompty.ResponseChunk{
						Kind:     prompty.StreamDelta,
						Identity: prompty.StreamIdentity{PartIDs: []string{"text"}},
						Content:  []prompty.ContentPart{prompty.TextPart{Text: "partial"}},
					},
					nil,
				) {
					return
				}
				<-ctx.Done()
			}
		},
	)
	owner := OwnStream(stream)
	closed := make(chan struct{})
	owner.Lifetime.OnClose(func() { close(closed) })
	go owner.Consume(func(event *prompty.ResponseChunk, _ error) {
		if event != nil && event.Kind == prompty.StreamDelta {
			_ = stream.Close()
			close(callbackClosed)
		}
	})
	// Act.
	<-callbackClosed
	<-cleanupStarted
	var callers sync.WaitGroup
	for range 8 {
		callers.Go(func() { _ = stream.Close() })
	}
	callers.Wait()
	joinStarted := make(chan struct{})
	joinDone := make(chan error, 1)
	go func() { close(joinStarted); joinDone <- owner.Lifetime.Close() }()
	<-joinStarted
	// Assert: cancellation callback returned while source cleanup is still blocked.
	select {
	case <-closed:
		t.Fatal("lifetime released before transport cleanup")
	default:
	}
	select {
	case <-owner.done:
		t.Fatal("consumption completed before transport cleanup")
	default:
	}
	close(releaseCleanup)
	require.NoError(t, <-joinDone)
	<-closed
	<-owner.done
	require.Equal(t, prompty.StreamCanceled, stream.Status().State)
	require.NoError(t, owner.Lifetime.Close())
}

func TestCloseBeforeConsumptionNeverDispatches(t *testing.T) {
	t.Parallel()
	// Arrange.
	dispatches := 0
	stream := prompty.NewStream(
		t.Context(),
		prompty.StreamNative,
		func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] { dispatches++; return nil },
	)
	owner := OwnStream(stream)
	// Act.
	require.NoError(t, stream.Close())
	owner.Consume(func(_ *prompty.ResponseChunk, err error) { require.ErrorIs(t, err, context.Canceled) })
	require.NoError(t, owner.Lifetime.Close())
	// Assert.
	require.Zero(t, dispatches)
}
