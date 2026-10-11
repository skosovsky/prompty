package prompty

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamIDlessToolFragmentsUseCheckedLocalReferences(t *testing.T) {
	t.Parallel()
	// Arrange: same name, distinct source references; finish order differs.
	first := identityFixtureFrame([]string{"a", "b"}, []ContentPart{
		ToolCallPart{Name: "lookup", ArgsChunk: `{"n":`},
		ToolCallPart{Name: "lookup", ArgsChunk: `{"n":`},
	})
	first.Annotations = []PartAnnotation{
		{PartIndex: 1, Kind: "fixture.progress", Value: JSONDocument(`{"started":true}`)},
	}
	second := identityFixtureFrame([]string{"b", "a"}, []ContentPart{
		ToolCallPart{Name: "lookup", ArgsChunk: `2}`},
		ToolCallPart{Name: "lookup", ArgsChunk: `9007199254740993}`},
	})
	stream := NewStream(t.Context(), StreamNative, identityFixtureSource(first, second,
		&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls}))
	// Act.
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if frame.Kind == StreamTerminal {
			terminals++
		}
	}
	status := stream.Status()
	// Assert: final order, exact arguments and annotations retain logical binding.
	require.Equal(t, StreamCompleted, status.State)
	require.Equal(t, 1, terminals)
	require.Equal(t, []string{"a", "b"}, status.Identity.PartIDs)
	require.Equal(t, []ContentPart{
		ToolCallPart{Name: "lookup", Args: `{"n":9007199254740993}`},
		ToolCallPart{Name: "lookup", Args: `{"n":2}`},
	}, status.Result.Content)
	require.Equal(t, first.Annotations, status.Result.Annotations)
}

func TestStreamLocalToolIdentityDoesNotAliasProviderID(t *testing.T) {
	t.Parallel()
	// Arrange: provider ID equals a source logical reference, not its namespace.
	stream := NewStream(t.Context(), StreamNative, identityFixtureSource(
		identityFixtureFrame([]string{"a", "different"}, []ContentPart{
			ToolCallPart{Name: "lookup", ArgsChunk: `{"n":`},
			ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: `{"n":`},
		}), identityFixtureFrame([]string{"different", "a"}, []ContentPart{
			ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: `2}`},
			ToolCallPart{Name: "lookup", ArgsChunk: `1}`},
		}), &ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls}))
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert: independent calls, original empty/nonempty IDs, no generated ID.
	require.Equal(t, []ContentPart{
		ToolCallPart{Name: "lookup", Args: `{"n":1}`},
		ToolCallPart{ID: "a", Name: "lookup", Args: `{"n":2}`},
	}, stream.Status().Result.Content)
}

func TestStreamLocalToolConflictsAndUnprovenStandaloneDeltas(t *testing.T) {
	t.Parallel()
	for _, next := range []ToolCallPart{
		{Name: "different", ArgsChunk: `1}`},
		{Name: "lookup", Args: `{"n":1}`},
		{Name: "lookup", ArgsChunk: `invalid`},
	} {
		// Arrange: a local reference does not relax argument/name validation.
		first := ToolCallPart{Name: "lookup", ArgsChunk: `{"n":`}
		stream := NewStream(t.Context(), StreamNative, identityFixtureSource(
			identityFixtureFrame([]string{"a"}, []ContentPart{first}),
			identityFixtureFrame([]string{"a"}, []ContentPart{next}),
			&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls}))
		// Act.
		for range stream.Events() {
		}
		_, standaloneErr := AssembleToolCallParts([]ContentPart{first, next})
		// Assert: no unsafe successful call with guessed correlation.
		require.Equal(t, StreamFailed, stream.Status().State)
		require.ErrorIs(t, stream.Status().Err, ErrIncompleteToolCallArgs)
		require.ErrorIs(t, standaloneErr, ErrIncompleteToolCallArgs)
	}
}
