package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCallArgsForTranslate_RejectsArgsChunkWithoutGlue(t *testing.T) {
	t.Parallel()
	_, err := ToolCallArgsForTranslate(ToolCallPart{Name: "lookup", ArgsChunk: `{"q":1}`})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIncompleteToolCallArgs)
}

func TestResolvedToolCallArgs_RejectsArgsChunkWithoutGlue(t *testing.T) {
	t.Parallel()
	_, err := resolvedToolCallArgs(ToolCallPart{Name: "lookup", ArgsChunk: `{"q":1}`})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIncompleteToolCallArgs)
}

func TestTranslateRejectsDualSnapshotAndDeltaEvenWhenEqual(t *testing.T) {
	t.Parallel()
	// Arrange: equal strings do not make two different argument representations unambiguous.
	call := ToolCallPart{Name: "private-name", Args: `{}`, ArgsChunk: `{}`}
	// Act.
	_, err := ToolCallArgsForTranslate(call)
	// Assert.
	require.ErrorIs(t, err, ErrIncompleteToolCallArgs)
	require.NotContains(t, err.Error(), "private")
}

func TestAssembleToolCallParts_CopiesChunkToArgs(t *testing.T) {
	t.Parallel()
	parts, err := AssembleToolCallParts([]ContentPart{
		ToolCallPart{ID: "call_1", Name: "lookup", ArgsChunk: `{"q":1}`},
	})
	require.NoError(t, err)
	tc := parts[0].(ToolCallPart)
	assert.Equal(t, `{"q":1}`, tc.Args)
	assert.Empty(t, tc.ArgsChunk)
	args, err := resolvedToolCallArgs(tc)
	require.NoError(t, err)
	assert.Equal(t, `{"q":1}`, args)
}
