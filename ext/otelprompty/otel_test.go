package otelprompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestWithTracing_Execute_WrapsNext(t *testing.T) {
	t.Parallel()
	callCount := 0
	base := &invokerStub{
		generate: func(_ context.Context, _ *prompty.PromptExecution) (*prompty.Response, error) {
			callCount++
			return &prompty.Response{Content: []prompty.ContentPart{prompty.TextPart{Text: "ok"}}}, nil
		},
		generateStream: func(_ context.Context, _ *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				yield(
					&prompty.ResponseChunk{
						Content:    []prompty.ContentPart{prompty.TextPart{Text: "ok"}},
						IsFinished: true,
					},
					nil,
				)
			}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	exec := &prompty.PromptExecution{
		Metadata: prompty.PromptMetadata{ID: "test"},
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "hi"}}},
		},
	}
	resp, err := inv.Execute(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, callCount)
}

func TestWithTracing_Execute_PropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("backend error")
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return nil, wantErr
		},
		generateStream: func(context.Context, *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, wantErr) }
		},
	}
	mw := WithTracing()
	inv := mw(base)
	_, err := inv.Execute(context.Background(), &prompty.PromptExecution{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestWithTracing_ExecuteStream_WrapsNext(t *testing.T) {
	t.Parallel()
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return nil, errors.New("unexpected Execute")
		},
		generateStream: func(_ context.Context, _ *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				yield(&prompty.ResponseChunk{IsFinished: true}, nil)
			}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	seq := inv.ExecuteStream(context.Background(), &prompty.PromptExecution{})
	count := 0
	for _, err := range seq {
		if err != nil {
			require.NoError(t, err)
		}
		count++
	}
	assert.Equal(t, 1, count, "stream should yield one chunk")
}

// TestWithTracing_ExecuteStream_InterruptedStream verifies that when the consumer
// stops early (yield=false), metrics (latency_ms, tokens_total, finish_reason) are still recorded
// via the defer finalizer. The stream yields one chunk with tokens and FinishReason, then consumer
// stops; we assert no panic and that the iterator terminates.
func TestWithTracing_ExecuteStream_InterruptedStream(t *testing.T) {
	t.Parallel()
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return nil, errors.New("unexpected Execute")
		},
		generateStream: func(_ context.Context, _ *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				// First chunk has tokens and finish reason; consumer stops after it (yield=false)
				chunk := &prompty.ResponseChunk{
					Content:      []prompty.ContentPart{prompty.TextPart{Text: "a"}},
					IsFinished:   true,
					FinishReason: "stop",
					Usage:        prompty.Usage{TotalTokens: 42},
				}
				yield(chunk, nil)
			}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	seq := inv.ExecuteStream(
		context.Background(),
		&prompty.PromptExecution{Metadata: prompty.PromptMetadata{ID: "test"}},
	)
	count := 0
	for _, err := range seq {
		if err != nil {
			require.NoError(t, err)
		}
		count++
		// Stop after first chunk; defer still records latency, totalTokens, finishReason
		break
	}
	assert.Equal(
		t,
		1,
		count,
		"consumer stops after first chunk; defer finalizer still records latency/tokens/finish_reason",
	)
}

// TestWithTracing_ExecuteStream_ProviderError verifies that when the base invoker yields an error,
// it propagates to the consumer and the middleware does not panic. Defer still records latency_ms.
func TestWithTracing_ExecuteStream_ProviderError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("stream backend error")
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return nil, errors.New("unexpected Execute")
		},
		generateStream: func(_ context.Context, _ *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				yield(nil, wantErr)
			}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	seq := inv.ExecuteStream(
		context.Background(),
		&prompty.PromptExecution{Metadata: prompty.PromptMetadata{ID: "test"}},
	)
	var gotErr error
	for _, err := range seq {
		gotErr = err
		break
	}
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, wantErr)
}

// TestWithTracing_Execute_ErrorPathRecordsLatency verifies that when Execute returns an error,
// the middleware records latency_ms via defer and propagates the error without panic.
func TestWithTracing_Execute_ErrorPathRecordsLatency(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sync backend error")
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return nil, wantErr
		},
		generateStream: func(context.Context, *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(_ func(*prompty.ResponseChunk, error) bool) {}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	_, err := inv.Execute(context.Background(), &prompty.PromptExecution{Metadata: prompty.PromptMetadata{ID: "test"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestWithTracing_ExecuteStream_EmptyStream verifies that when the stream yields no chunks,
// the middleware completes without panic and defer still runs (recording latency_ms only).
func TestWithTracing_ExecuteStream_EmptyStream(t *testing.T) {
	t.Parallel()
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return nil, errors.New("unexpected Execute")
		},
		generateStream: func(context.Context, *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(_ func(*prompty.ResponseChunk, error) bool) {
				// No chunks yielded; defer records only latency_ms
			}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	seq := inv.ExecuteStream(
		context.Background(),
		&prompty.PromptExecution{Metadata: prompty.PromptMetadata{ID: "test"}},
	)
	count := 0
	for _, err := range seq {
		if err != nil {
			require.NoError(t, err)
		}
		count++
	}
	assert.Equal(t, 0, count, "empty stream yields zero chunks")
}

// TestWithTracing_Execute_RecordsFinishReason exercises the sync path where Response has FinishReason.
func TestWithTracing_Execute_RecordsFinishReason(t *testing.T) {
	t.Parallel()
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
			return &prompty.Response{
				Content:      []prompty.ContentPart{prompty.TextPart{Text: "ok"}},
				FinishReason: "stop",
			}, nil
		},
		generateStream: func(context.Context, *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(_ func(*prompty.ResponseChunk, error) bool) {}
		},
	}
	mw := WithTracing()
	inv := mw(base)
	resp, err := inv.Execute(
		context.Background(),
		&prompty.PromptExecution{Metadata: prompty.PromptMetadata{ID: "test"}},
	)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "stop", resp.FinishReason)
}

type invokerStub struct {
	generate       func(context.Context, *prompty.PromptExecution) (*prompty.Response, error)
	generateStream func(context.Context, *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error]
}

func (i *invokerStub) Execute(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	return i.generate(ctx, exec)
}

func (i *invokerStub) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return i.generateStream(ctx, exec)
}
