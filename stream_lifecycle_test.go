package prompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamTrailingUsageAndSingleTerminal(t *testing.T) {
	t.Parallel()
	// Arrange.
	calls := 0
	stream := NewStream(context.Background(), StreamNative,
		func(context.Context) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				calls++
				for _, event := range []*ResponseChunk{
					{Kind: StreamStart},
					{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}}, Kind: StreamDelta, Content: []ContentPart{TextPart{Text: "ok"}}},
					{Kind: StreamFinish, Outcome: OutcomeCompleted, FinishReason: "stop"},
					{Kind: StreamUsage, Usage: Usage{
						TotalTokens: 7, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative,
					}},
				} {
					if !yield(event, nil) {
						return
					}
				}
			}
		})
	require.Zero(t, calls, "creating handle must not dispatch")
	// Act.
	var terminal int
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Kind == StreamTerminal {
			terminal++
		}
	}
	snapshot := stream.Status()
	// Assert.
	require.Equal(t, 1, calls)
	require.Equal(t, 1, terminal)
	require.Equal(t, StreamCompleted, snapshot.State)
	require.Equal(t, 7, snapshot.Result.Usage.TotalTokens)
	require.Equal(t, []UsageCounter{UsageTotal}, snapshot.Result.Usage.Known)
	require.Equal(t, "ok", snapshot.Result.Content[0].(TextPart).Text)
	snapshot.Result.Usage.Known[0] = UsagePrompt
	require.Equal(t, []UsageCounter{UsageTotal}, stream.Status().Result.Usage.Known)
	for _, err := range stream.Events() {
		require.ErrorIs(t, err, ErrStreamConsumed)
	}
}

func TestStreamEarlyStopRetainsPartialAndCancels(t *testing.T) {
	t.Parallel()
	// Arrange.
	released := false
	var sourceDone <-chan struct{}
	stream := NewStream(context.Background(), StreamNative,
		func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
			sourceDone = ctx.Done()
			return func(yield func(*ResponseChunk, error) bool) {
				defer func() { released = true }()
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
				t.Fatal("source must not continue after consumer break")
			}
		})
	// Act.
	for range stream.Events() {
		break
	}
	// Assert.
	require.True(t, released)
	require.True(t, stream.Status().Result.ContinuationUnavailable)
	select {
	case <-sourceDone:
	default:
		t.Fatal("source context was not canceled")
	}
	require.Equal(t, StreamCanceled, stream.Status().State)
	require.Equal(t, "partial", stream.Status().Result.Content[0].(TextPart).Text)
	require.NoError(t, stream.Close())
	require.Equal(t, StreamCanceled, stream.Status().State)
}

func TestStreamFailureCancelsBeforeSourceUnwinds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		chunk *ResponseChunk
		err   error
	}{
		{"invalid kind", &ResponseChunk{Kind: "invalid"}, nil},
		{"invalid outcome", &ResponseChunk{Kind: StreamFinish, Outcome: "invalid"}, nil},
		{"transport error", nil, errors.New("fixture transport error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: source releases its body only once its context is canceled.
			released := false
			stream := NewStream(
				context.Background(),
				StreamNative,
				func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
					return func(yield func(*ResponseChunk, error) bool) {
						yield(tc.chunk, tc.err)
						<-ctx.Done()
						released = true
					}
				},
			)
			// Act.
			var terminal int
			for event, err := range stream.Events() {
				require.Error(t, err)
				require.Equal(t, StreamTerminal, event.Kind)
				terminal++
			}
			// Assert.
			require.True(t, released)
			require.Equal(t, 1, terminal)
			require.Equal(t, StreamFailed, stream.Status().State)
		})
	}
}

func TestStreamErrorsPreservePartialAndNeverRetry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"transport", errors.New("transport fixture")},
		{"missing finish", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			calls := 0
			stream := NewStream(context.Background(), StreamNative,
				func(context.Context) iter.Seq2[*ResponseChunk, error] {
					return func(yield func(*ResponseChunk, error) bool) {
						calls++
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
						if tc.err != nil {
							yield(nil, tc.err)
						}
					}
				})
			// Act.
			var terminal int
			for event, err := range stream.Events() {
				if err != nil {
					require.Equal(t, StreamTerminal, event.Kind)
					terminal++
				}
			}
			// Assert.
			require.Equal(t, 1, calls)
			require.Equal(t, 1, terminal)
			require.Equal(t, StreamFailed, stream.Status().State)
			require.Len(t, stream.Status().Result.Content, 1)
		})
	}
}

func TestStructuredStreamAcceptsControlEvents(t *testing.T) {
	t.Parallel()
	// Arrange.
	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				for _, event := range []*ResponseChunk{
					{Kind: StreamStart},
					{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}}, Kind: StreamDelta, Content: []ContentPart{TextPart{Text: `{"answer":`}}},
					{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}}, Kind: StreamDelta, Content: []ContentPart{TextPart{Text: `"ok"}`}}},
					{Kind: StreamFinish, Outcome: OutcomeCompleted},
					{Kind: StreamUsage, Usage: Usage{
						TotalTokens: 1, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative,
					}},
				} {
					if !yield(event, nil) {
						return
					}
				}
			}
		},
	}
	// Act.
	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](
		context.Background(), invoker, SimplePrompt("fixture"), StreamNative))
	// Assert.
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "ok", items[0].Answer)
}

func TestStreamCanceledBeforeConsumptionDoesNotDispatch(t *testing.T) {
	t.Parallel()
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := NewStream(ctx, StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		t.Fatal("canceled stream dispatched")
		return nil
	})
	// Act.
	for _, err := range stream.Events() {
		require.ErrorIs(t, err, context.Canceled)
	}
	// Assert.
	require.Equal(t, StreamCanceled, stream.Status().State)
}
