package otelprompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestRemoteObserverRejectsConsumedHandleWithoutReplay(t *testing.T) {
	t.Parallel()
	// Arrange: malformed port transfers an already completed handle with known result.
	result := &prompty.RemoteResponseResult{
		Response: prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "known"}}),
	}
	starts := 0
	original := &responseStreamFixture{result: result, Stream: prompty.NewStream(t.Context(), prompty.StreamNative,
		func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			starts++
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				yield(&prompty.ResponseChunk{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeCompleted}, nil)
			}
		})}
	for _, err := range original.Events() {
		require.NoError(t, err)
	}
	before := original.Status()
	exporter, option := operationExporter(t)
	// Act.
	wrapped := WrapResponseStreaming(responseStreamingFixture{stream: original}, option).
		CreateStream(t.Context(), prompty.RemoteResponseRequest{})
	for _, err := range wrapped.Events() {
		require.ErrorIs(t, err, prompty.ErrStreamConsumed)
	}
	// Assert: known result and original terminal remain available, no second dispatch.
	require.Equal(t, 1, starts)
	require.Same(t, result, wrapped.RemoteResult())
	require.Equal(t, before, original.Status())
	require.Equal(t, prompty.StreamFailed, wrapped.Status().State)
	require.Len(t, exporter.GetSpans(), 1)
}

func TestRemoteObserverClosesAlreadyStartedTransferredHandle(t *testing.T) {
	t.Parallel()
	// Arrange: a malformed port has started work before transferring ownership.
	started, stopped, consumed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	original := &responseStreamFixture{Stream: prompty.NewStream(t.Context(), prompty.StreamNative,
		func(ctx context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				close(started)
				<-ctx.Done()
				close(stopped)
				yield(nil, ctx.Err())
			}
		})}
	go func() {
		for range original.Events() {
		}
		close(consumed)
	}()
	<-started
	_, option := operationExporter(t)
	// Act.
	wrapped := WrapResponseStreaming(responseStreamingFixture{stream: original}, option).
		CreateStream(t.Context(), prompty.RemoteResponseRequest{})
	// Assert: Close cancels original work; fallback fails without consuming it again.
	<-stopped
	<-consumed
	for _, err := range wrapped.Events() {
		require.ErrorIs(t, err, prompty.ErrStreamConsumed)
	}
	require.Equal(t, prompty.StreamStopClosed, original.Status().StopReason)
	require.Equal(t, prompty.StreamFailed, wrapped.Status().State)
}
