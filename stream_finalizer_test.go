package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamFinalizerRunsAfterUsageAndAssemblyOnIsolatedResult(t *testing.T) {
	t.Parallel()
	// Arrange.
	sourceEnded := false
	finalized := 0
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			for _, event := range []*ResponseChunk{
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:a"}}, Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: `{"x":`}}},
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:a"}}, Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: `1}`}}},
				{Kind: StreamFinish, Outcome: OutcomeToolCalls},
				{Kind: StreamUsage, Usage: Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative}},
			} {
				if !yield(event, nil) {
					return
				}
			}
			sourceEnded = true
		}
	}, WithStreamFinalizer(func(_ context.Context, response *Response) error {
		finalized++
		require.True(t, sourceEnded)
		require.Equal(t, 9, response.Usage.TotalTokens)
		require.Len(t, response.Content, 1)
		call := response.Content[0].(ToolCallPart)
		require.Empty(t, call.ArgsChunk)
		require.JSONEq(t, `{"x":1}`, call.Args)
		response.Content[0] = TextPart{Text: "mutated snapshot"}
		return ErrSchemaMismatch
	}))
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == StreamTerminal {
			terminals++
			require.ErrorIs(t, err, ErrSchemaMismatch)
		}
	}
	// Assert.
	require.Equal(t, 1, finalized)
	require.Equal(t, 1, terminals)
	status := stream.Status()
	require.Equal(t, StreamFailed, status.State)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.IsType(t, ToolCallPart{}, status.Result.Content[0])
}

func TestCloseCancelsFinalizerWithoutHoldingStatusLock(t *testing.T) {
	t.Parallel()
	// Arrange.
	entered := make(chan struct{})
	done := make(chan struct{})
	terminals := 0
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil)
		}
	}, WithStreamFinalizer(func(ctx context.Context, _ *Response) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}))
	go func() {
		defer close(done)
		for event := range stream.Events() {
			if event.Kind == StreamTerminal {
				terminals++
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
	require.Equal(t, 1, terminals)
}

func TestMaterializeCannotRewriteCanceledPartialResult(t *testing.T) {
	t.Parallel()
	// Arrange: cancellation wins before EOF materialization takes the status lock.
	stream := NewStream(context.Background(), StreamNative, nil)
	require.NoError(t, stream.begin())
	for _, chunk := range []string{`{"x":`, `1}`} {
		require.NoError(
			t,
			stream.accumulate(&ResponseChunk{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:a"}},
				Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: chunk}},
			}),
		)
	}
	require.NoError(t, stream.Close())
	before := stream.Status()
	// Act.
	err := stream.materialize()
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, before, stream.Status())
	require.Len(t, stream.Status().Result.Content, 2)
}
