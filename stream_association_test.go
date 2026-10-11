package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamAssemblyRemapsUnrelatedStateAndAnnotations(t *testing.T) {
	t.Parallel()
	// Arrange: two interleaved calls collapse before a signed reasoning/text pair.
	fixture := stateFixture()
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			for _, event := range []*ResponseChunk{
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:a", "tool:b"}}, Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "a", Name: "fn", ArgsChunk: `{"x":`},
					ToolCallPart{ID: "b", Name: "fn", ArgsChunk: `{"x":`}}},
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:b", "tool:a"}}, Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "b", ArgsChunk: `2}`},
					ToolCallPart{ID: "a", ArgsChunk: `1}`}}},
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"fixture:0", "fixture:1", "fixture:2"}}, Kind: StreamDelta, Content: fixture.Content, ProviderState: fixture.ProviderState, Annotations: fixture.Annotations},
				{Kind: StreamFinish, Outcome: OutcomeToolCalls},
			} {
				if !yield(event, nil) {
					return
				}
			}
		}
	})
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Kind == StreamTerminal {
			terminals++
		}
	}
	result := stream.Status().Result
	// Assert.
	require.Equal(t, StreamCompleted, stream.Status().State)
	require.Equal(t, 1, terminals)
	require.Len(t, result.Content, 5)
	require.Equal(t, `{"x":1}`, result.Content[0].(ToolCallPart).Args)
	require.Equal(t, `{"x":2}`, result.Content[1].(ToolCallPart).Args)
	require.Equal(t, 3, result.ProviderState[0].PartIndex)
	require.Equal(t, 4, result.Annotations[0].PartIndex)
	require.Equal(t, fixture.ProviderState[0].Envelope, result.ProviderState[0].Envelope)
	require.Equal(t, fixture.Annotations[0].Value, result.Annotations[0].Value)
	wire, err := MarshalResponse(result, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	restored, err := UnmarshalResponse(wire)
	require.NoError(t, err)
	require.Equal(t, result, restored)
}

func TestStreamAssemblyRejectsChangedStateBoundDelta(t *testing.T) {
	t.Parallel()
	// Arrange: opaque state binds to the exact first source part, not just call ID.
	state := stateFixture().ProviderState[0]
	state.PartIndex = 0
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			for _, event := range []*ResponseChunk{
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:a"}}, Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "a", Name: "fn", ArgsChunk: `{"x":`}},
					ProviderState: []PartState{state}},
				{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"tool:a"}}, Kind: StreamDelta, Content: []ContentPart{ToolCallPart{ID: "a", ArgsChunk: `1}`}}},
				{Kind: StreamFinish, Outcome: OutcomeToolCalls},
			} {
				if !yield(event, nil) {
					return
				}
			}
		}
	})
	// Act.
	var terminalErr error
	for event, err := range stream.Events() {
		if event.Kind == StreamTerminal {
			terminalErr = err
		}
	}
	// Assert: failed materialization preserves all raw partial parts and state.
	require.ErrorIs(t, terminalErr, ErrProviderState)
	require.Equal(t, StreamFailed, stream.Status().State)
	require.Len(t, stream.Status().Result.Content, 2)
	require.Equal(t, state, stream.Status().Result.ProviderState[0])
}
