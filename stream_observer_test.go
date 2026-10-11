package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

type streamObserverFixtureKey string

func TestStreamLifecycleObserversAreLazyNestedAndPreserveFinalStatus(t *testing.T) {
	t.Parallel()
	// Arrange: middleware attaches to the existing handle; metadata reaches its one source invocation.
	var order []string
	called, finalized := 0, false
	stream := NewStream(context.Background(), StreamNative, func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			called++
			require.Equal(t, "outer", ctx.Value(streamObserverFixtureKey("outer")))
			require.Equal(t, "inner", ctx.Value(streamObserverFixtureKey("inner")))
			yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil)
		}
	}, WithStreamFinalizer(func(context.Context, *Response) error {
		finalized = true
		return nil
	}))
	messageID := stream.Status().Identity.MessageID
	for _, name := range []string{"inner", "outer"} {
		require.NoError(t, stream.ObserveLifecycle(func(ctx context.Context) (context.Context, func(StreamStatus)) {
			order = append(order, name+" start")
			return context.WithValue(ctx, streamObserverFixtureKey(name), name), func(status StreamStatus) {
				order = append(order, name+" end")
				require.True(t, finalized)
				require.Equal(t, StreamCompleted, status.State)
				require.Equal(t, messageID, status.Identity.MessageID)
			}
		}))
	}
	require.Empty(t, order)
	require.Zero(t, called)
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert.
	require.Equal(t, 1, called)
	require.Equal(t, []string{"outer start", "inner start", "inner end", "outer end"}, order)
	require.ErrorIs(t, stream.ObserveLifecycle(func(ctx context.Context) (context.Context, func(StreamStatus)) {
		return ctx, func(StreamStatus) {}
	}), ErrStreamConsumed)
}

func TestStreamLifecycleObserverCannotDetachCancellation(t *testing.T) {
	t.Parallel()
	// Arrange: the observer tries to replace the invocation with an unrelated context.
	called, closed := false, false
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		called = true
		return func(func(*ResponseChunk, error) bool) {}
	})
	require.NoError(t, stream.ObserveLifecycle(func(context.Context) (context.Context, func(StreamStatus)) {
		return context.Background(), func(status StreamStatus) {
			closed = true
			require.Equal(t, StreamFailed, status.State)
			require.ErrorIs(t, status.Err, ErrStreamProtocol)
		}
	}))
	// Act.
	terminals := 0
	for frame, err := range stream.Events() {
		require.ErrorIs(t, err, ErrStreamProtocol)
		require.Equal(t, StreamTerminal, frame.Kind)
		terminals++
	}
	// Assert.
	require.False(t, called)
	require.True(t, closed)
	require.Equal(t, 1, terminals)
}

func TestStreamLifecycleObserverSeesCanceledPartialIdentity(t *testing.T) {
	t.Parallel()
	// Arrange.
	stream := NewStream(context.Background(), StreamNative,
		identityFixtureSource(identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: "partial"}})))
	var observed StreamStatus
	require.NoError(t, stream.ObserveLifecycle(func(ctx context.Context) (context.Context, func(StreamStatus)) {
		return ctx, func(status StreamStatus) { observed = status }
	}))
	// Act.
	for range stream.Events() {
		break
	}
	// Assert.
	require.Equal(t, StreamCanceled, observed.State)
	require.ErrorIs(t, observed.Err, context.Canceled)
	require.Equal(t, stream.Status().Identity, observed.Identity)
	require.Equal(t, []string{"text"}, observed.Identity.PartIDs)
	observed.Identity.PartIDs[0] = "mutated"
	require.Equal(t, "text", stream.Status().Identity.PartIDs[0])
}
