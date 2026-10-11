package prompty

import (
	"context"
	"iter"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStructuredStreamInvocationLimits(t *testing.T) {
	t.Parallel()
	document := `{"answer":"same document"}`
	for _, limit := range []int{16, len(document), 128} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			t.Parallel()
			// Arrange: parallel invocations parse identical content with different policies.
			invoker := &scriptedInvoker{
				generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
					return func(yield func(*ResponseChunk, error) bool) {
						yield(&ResponseChunk{
							Kind:    StreamFinish,
							Outcome: OutcomeCompleted,
							Identity: StreamIdentity{
								PartIDs: []string{"known"},
							},
							Content: []ContentPart{TextPart{Text: document}},
						}, nil)
					}
				},
			}
			// Act.
			items, err := collectSeq(StreamStructuredOutput[valueSchemaResult](t.Context(), invoker,
				SimplePrompt("known"), StreamNative, StructuredStreamOptions{MaxObjectBytes: limit}))
			// Assert: exact boundary accepts and smaller limit rejects without changing peers.
			if limit < len(document) {
				require.ErrorIs(t, err, ErrStructuredStream)
				require.Empty(t, items)
			} else {
				require.NoError(t, err)
				require.Len(t, items, 1)
				require.Equal(t, "same document", items[0].Answer)
			}
		})
	}
}

func TestStructuredStreamInvalidLimitBeforeWork(t *testing.T) {
	t.Parallel()
	for _, options := range [][]StructuredStreamOptions{
		{{MaxObjectBytes: 0}}, {{MaxObjectBytes: -1}}, {{MaxObjectBytes: 16}, {MaxObjectBytes: 32}},
	} {
		// Arrange: no invoker or execution can be admitted, but policy failure takes precedence.
		// Act.
		items, err := collectSeq(
			StreamStructuredOutput[valueSchemaResult](t.Context(), nil, nil, StreamNative, options...),
		)
		// Assert.
		require.ErrorIs(t, err, ErrStructuredStream)
		require.Empty(t, items)
	}
}
