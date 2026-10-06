package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleToolCallParts_MultiChunkSameID(t *testing.T) {
	t.Parallel()
	parts, err := AssembleToolCallParts([]ContentPart{
		ToolCallPart{ID: "call_1", Name: "lookup", ArgsChunk: `{"q":`},
		ToolCallPart{ID: "call_1", Name: "lookup", ArgsChunk: `1}`},
	})
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(ToolCallPart)
	assert.Equal(t, `{"q":1}`, tc.Args)
	assert.Empty(t, tc.ArgsChunk)
}

func TestAssembleRejectsAmbiguousOrIncompleteSnapshot(t *testing.T) {
	t.Parallel()
	for _, parts := range [][]ContentPart{
		{ToolCallPart{ID: "a", Name: "fn", Args: `{}`, ArgsChunk: `{}`}},
		{ToolCallPart{ID: "a", Name: "fn", Args: `{`}, ToolCallPart{ID: "a", ArgsChunk: `}`}},
		{ToolCallPart{ID: "a", Name: "fn", Args: `{}`}, ToolCallPart{ID: "a", ArgsChunk: ` `}},
		{ToolCallPart{ID: "a", Name: "fn", ArgsChunk: `{}`}, ToolCallPart{ID: "a", Args: `{}`}},
	} {
		// Arrange/Act: Args promises a complete snapshot, never a partial delta.
		_, err := AssembleToolCallParts(parts)
		// Assert.
		require.ErrorIs(t, err, ErrIncompleteToolCallArgs)
	}
}

func TestAssembleRepeatedSnapshotsDoNotConcatenate(t *testing.T) {
	t.Parallel()
	// Arrange: identity-only frames do not choose a representation; repeated equal snapshots are idempotent.
	parts := []ContentPart{
		ToolCallPart{ID: "a", Name: "fn"},
		ToolCallPart{ID: "a", Args: `{}`},
		ToolCallPart{ID: "a", Args: `{}`},
	}
	// Act.
	assembled, err := AssembleToolCallParts(parts)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, []ContentPart{ToolCallPart{ID: "a", Name: "fn", Args: `{}`}}, assembled)
}

func TestAssembleCompleteSnapshotClonesCacheControls(t *testing.T) {
	t.Parallel()
	// Arrange.
	policy := &CachePolicy{Mechanism: CacheBlockMarker, Provider: "fixture", Controls: JSONDocument(`{"ttl":"1h"}`)}
	input := []ContentPart{ToolCallPart{ID: "a", Name: "fn", Args: `{}`, CachePolicy: policy}}
	// Act.
	parts, err := AssembleToolCallParts(input)
	require.NoError(t, err)
	parts[0].(ToolCallPart).CachePolicy.Controls[0] = 'x'
	// Assert.
	require.JSONEq(t, `{"ttl":"1h"}`, string(policy.Controls))
}

func TestAssembleToolCallParts_ConflictingArgsFails(t *testing.T) {
	t.Parallel()
	_, err := AssembleToolCallParts([]ContentPart{
		ToolCallPart{ID: "call_1", Name: "lookup", Args: `{"q":1}`},
		ToolCallPart{ID: "call_1", Name: "lookup", Args: `{"q":2}`},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIncompleteToolCallArgs)
	assert.Contains(t, err.Error(), "conflicting Args")
}

func TestAssembleToolNamesConflictHasTypedSafeError(t *testing.T) {
	t.Parallel()
	// Arrange: a stable ID cannot change function identity during assembly.
	parts := []ContentPart{
		ToolCallPart{ID: "private-id", Name: "private-first", ArgsChunk: `{`},
		ToolCallPart{ID: "private-id", Name: "private-second", ArgsChunk: `}`},
	}
	// Act.
	_, err := AssembleToolCallParts(parts)
	// Assert.
	require.ErrorIs(t, err, ErrIncompleteToolCallArgs)
	require.NotContains(t, err.Error(), "private")
}

func TestAssembleToolCallParts_DoesNotMergeWithoutID(t *testing.T) {
	t.Parallel()
	parts, err := AssembleToolCallParts([]ContentPart{
		ToolCallPart{Name: "lookup", ArgsChunk: `{"q":`},
		ToolCallPart{Name: "lookup", ArgsChunk: `1}`},
	})
	require.ErrorIs(t, err, ErrIncompleteToolCallArgs)
	require.Nil(t, parts)
}

func TestToolCallsFromContent_MultiChunkStream(t *testing.T) {
	t.Parallel()
	calls, err := toolCallsFromContent([]ContentPart{
		ToolCallPart{ID: "c1", Name: "fn", ArgsChunk: `{"a":`},
		ToolCallPart{ID: "c1", Name: "fn", ArgsChunk: `1}`},
	})
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, `{"a":1}`, calls[0].Args)
}
