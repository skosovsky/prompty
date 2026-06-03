package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlueToolCallArgChunks_MultiChunkSameID(t *testing.T) {
	t.Parallel()
	parts, err := GlueToolCallArgChunks([]ContentPart{
		ToolCallPart{ID: "call_1", Name: "lookup", ArgsChunk: `{"q":`},
		ToolCallPart{ID: "call_1", Name: "lookup", ArgsChunk: `1}`},
	})
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(ToolCallPart)
	assert.Equal(t, `{"q":1}`, tc.Args)
	assert.Empty(t, tc.ArgsChunk)
}

func TestGlueToolCallArgChunks_ConflictingArgsFails(t *testing.T) {
	t.Parallel()
	_, err := GlueToolCallArgChunks([]ContentPart{
		ToolCallPart{ID: "call_1", Name: "lookup", Args: `{"q":1}`},
		ToolCallPart{ID: "call_1", Name: "lookup", Args: `{"q":2}`},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting Args")
}

func TestGlueToolCallArgChunks_DoesNotMergeWithoutID(t *testing.T) {
	t.Parallel()
	parts, err := GlueToolCallArgChunks([]ContentPart{
		ToolCallPart{Name: "lookup", ArgsChunk: `{"q":`},
		ToolCallPart{Name: "lookup", ArgsChunk: `1}`},
	})
	require.NoError(t, err)
	require.Len(t, parts, 2)
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
