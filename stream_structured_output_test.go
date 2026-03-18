package prompty

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validatedStreamResult struct {
	Answer string `json:"answer"`
}

func (r validatedStreamResult) Validate() error {
	if r.Answer == "" {
		return errors.New("answer is required")
	}
	return nil
}

func TestStreamStructuredOutput_Object(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				yield(&ResponseChunk{
					Content:    []ContentPart{TextPart{Text: `{"answer":"ok"}`}},
					IsFinished: true,
				}, nil)
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "ok", items[0].Answer)
}

func TestStreamStructuredOutput_ArrayAcrossChunks(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				chunks := []string{
					`[`,
					`{"answer":"a"},`,
					`{"answer":"b"}`,
					`]`,
				}
				for i, chunk := range chunks {
					if !yield(&ResponseChunk{
						Content:    []ContentPart{TextPart{Text: chunk}},
						IsFinished: i == len(chunks)-1,
					}, nil) {
						return
					}
				}
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "a", items[0].Answer)
	assert.Equal(t, "b", items[1].Answer)
}

func TestStreamStructuredOutput_EscapedQuotesAndBracesInsideStrings(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				yield(&ResponseChunk{
					Content: []ContentPart{
						TextPart{Text: `{"answer":"He said: \"Hello, {world}\""}`},
					},
					IsFinished: true,
				}, nil)
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, `He said: "Hello, {world}"`, items[0].Answer)
}

func TestStreamStructuredOutput_MarkdownFencedJSON(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				chunks := []string{"```json\n", `{"answer":"ok"}`, "\n```"}
				for i, chunk := range chunks {
					if !yield(&ResponseChunk{
						Content:    []ContentPart{TextPart{Text: chunk}},
						IsFinished: i == len(chunks)-1,
					}, nil) {
						return
					}
				}
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "ok", items[0].Answer)
}

func TestStreamStructuredOutput_IncompleteJSONIncludesPreview(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				yield(&ResponseChunk{
					Content:    []ContentPart{TextPart{Text: `{"answer":"oops"`}},
					IsFinished: true,
				}, nil)
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.Error(t, err)
	assert.Empty(t, items)
	assert.Contains(t, err.Error(), "incomplete JSON")
	assert.Contains(t, err.Error(), "oops")
	assert.Contains(t, err.Error(), "partial buffer tail")
}

func TestStreamStructuredOutput_ObjectTooLarge(t *testing.T) {
	oldLimit := streamMaxObjectSizeBytes
	streamMaxObjectSizeBytes = 16
	t.Cleanup(func() {
		streamMaxObjectSizeBytes = oldLimit
	})

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				yield(&ResponseChunk{
					Content: []ContentPart{
						TextPart{Text: `{"answer":"this-is-too-long"}`},
					},
					IsFinished: true,
				}, nil)
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.Error(t, err)
	assert.Empty(t, items)
	assert.Contains(t, err.Error(), "stream object too large")
	assert.Contains(t, err.Error(), "partial buffer tail")
}

func TestStreamStructuredOutput_ArrayOfPrimitivesUnsupported(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				yield(&ResponseChunk{
					Content:    []ContentPart{TextPart{Text: `["a"]`}},
					IsFinished: true,
				}, nil)
			}
		},
	}

	items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi")))
	require.Error(t, err)
	assert.Empty(t, items)
	assert.Contains(t, err.Error(), "unsupported non-object item")
}

func TestStreamStructuredOutput_SemanticValidationCancelsContext(t *testing.T) {
	t.Parallel()

	canceled := make(chan struct{}, 1)
	invoker := &scriptedInvoker{
		generateStream: func(ctx context.Context, _ *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				if !yield(&ResponseChunk{
					Content:    []ContentPart{TextPart{Text: `{"answer":""}`}},
					IsFinished: true,
				}, nil) {
					<-ctx.Done()
					canceled <- struct{}{}
					return
				}
				<-ctx.Done()
				canceled <- struct{}{}
			}
		},
	}

	_, err := collectSeq(StreamStructuredOutput[validatedStreamResult](context.Background(), invoker, SimplePrompt("hi")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "semantic validation failed")
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected stream context to be canceled after semantic validation failure")
	}
}

func TestStreamStructuredOutput_EarlyStopCancelsContext(t *testing.T) {
	t.Parallel()

	canceled := make(chan struct{}, 1)
	invoker := &scriptedInvoker{
		generateStream: func(ctx context.Context, _ *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				if !yield(&ResponseChunk{
					Content: []ContentPart{TextPart{Text: `{"answer":"first"}`}},
				}, nil) {
					<-ctx.Done()
					canceled <- struct{}{}
					return
				}
				if !yield(&ResponseChunk{
					Content:    []ContentPart{TextPart{Text: `{"answer":"second"}`}},
					IsFinished: true,
				}, nil) {
					<-ctx.Done()
					canceled <- struct{}{}
				}
			}
		},
	}

	seq := StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"))
	calls := 0
	seq(func(item valueSchemaResult, err error) bool {
		require.NoError(t, err)
		assert.Equal(t, "first", item.Answer)
		calls++
		return false
	})
	assert.Equal(t, 1, calls)
	select {
	case <-canceled:
	default:
		t.Fatalf("expected stream context to be canceled after early stop")
	}
}
