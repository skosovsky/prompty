package prompty

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func updateStateFixture(index int, payload string) PartState {
	state := stateFixture().ProviderState[0]
	state.PartIndex = index
	state.Envelope.Payload = []byte(payload)
	return state
}

func stateUpdateStream(events ...*ResponseChunk) *Stream {
	return NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			for _, event := range events {
				if !yield(event, nil) {
					return
				}
			}
		}
	})
}

func TestToolStateUpdateInterleavedMaterializationAndIsolation(t *testing.T) {
	t.Parallel()
	// Arrange: local/provider IDs differ; two calls close in reverse order.
	a, b := updateStateFixture(0, "pending-a"), updateStateFixture(1, "pending-b")
	closedB, closedA := updateStateFixture(0, "complete-b"), updateStateFixture(1, "complete-a")
	stream := stateUpdateStream(
		&ResponseChunk{Kind: StreamDelta, Identity: StreamIdentity{PartIDs: []string{"local-a", "local-b"}},
			Content:      []ContentPart{ToolCallPart{ID: "a", Name: "fn"}, ToolCallPart{ID: "b", Name: "fn"}},
			StateUpdates: []PartState{a, b}},
		&ResponseChunk{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"local-b", "local-a"}},
			Content: []ContentPart{
				ToolCallPart{ID: "b", Name: "fn", Args: `{"n":2}`},
				ToolCallPart{ID: "a", Name: "fn", Args: `{"n":1}`},
			},
			StateUpdates: []PartState{closedB, closedA},
		},
		&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls},
	)
	// Act: mutate yielded update payloads, never producer/status snapshots.
	terminals := 0
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Kind == StreamTerminal {
			terminals++
		}
		for i := range event.StateUpdates {
			event.StateUpdates[i].Envelope.Payload[0] = 'x'
		}
	}
	status := stream.Status()
	// Assert: only latest bindings, exact final content, source-proof reindexing.
	require.Equal(t, StreamCompleted, status.State)
	require.Equal(t, 1, terminals)
	require.Equal(
		t,
		[]ContentPart{
			ToolCallPart{ID: "a", Name: "fn", Args: `{"n":1}`},
			ToolCallPart{ID: "b", Name: "fn", Args: `{"n":2}`},
		},
		status.Result.Content,
	)
	require.Len(t, status.Result.ProviderState, 2)
	require.Equal(t, 1, status.Result.ProviderState[0].PartIndex)
	require.Equal(t, 0, status.Result.ProviderState[1].PartIndex)
	require.Equal(t, "complete-b", string(status.Result.ProviderState[0].Envelope.Payload))
	require.Equal(t, "complete-a", string(status.Result.ProviderState[1].Envelope.Payload))
	require.Equal(t, "pending-a", string(a.Envelope.Payload))
	status.Result.ProviderState[0].Envelope.Payload[0] = 'x'
	require.Equal(t, "complete-b", string(stream.Status().Result.ProviderState[0].Envelope.Payload))
}

func TestToolStateUpdateFailureAtomicity(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"scope", "codec", "required", "format", "payload", "index", "duplicate", "text", "usage", "identity", "expiry", "also-ordinary"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			// Arrange: one accepted provisional state, then a corrupt update.
			original := updateStateFixture(0, "pending")
			update := updateStateFixture(0, "complete")
			first := &ResponseChunk{
				Kind:         StreamDelta,
				Identity:     StreamIdentity{PartIDs: []string{"tool"}},
				Content:      []ContentPart{ToolCallPart{ID: "a", Name: "fn"}},
				StateUpdates: []PartState{original},
			}
			bad := &ResponseChunk{
				Kind:         StreamDelta,
				Identity:     StreamIdentity{PartIDs: []string{"tool"}},
				Content:      []ContentPart{ToolCallPart{ID: "a", Name: "fn", Args: `{}`}},
				StateUpdates: []PartState{update},
			}
			mutateStateUpdate(bad, mutation)
			stream := stateUpdateStream(first, bad, &ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls})
			// Act.
			for range stream.Events() {
			}
			// Assert: no content/usage/state half-commit, no successful assembly.
			status := stream.Status()
			require.Equal(t, StreamFailed, status.State)
			require.Error(t, status.Err)
			require.Len(t, status.Result.Content, 1)
			require.Equal(t, []PartState{original}, status.Result.ProviderState)
			require.Zero(t, status.Result.Usage.TotalTokens)
		})
	}
}

func mutateStateUpdate(frame *ResponseChunk, mutation string) {
	switch mutation {
	case "scope":
		frame.StateUpdates[0].Envelope.Scope.Endpoint = "https://other.invalid"
	case "codec":
		frame.StateUpdates[0].Envelope.Codec = "other"
	case "required":
		frame.StateUpdates[0].Envelope.Required = false
	case "format":
		frame.StateUpdates[0].Envelope.Format = "other"
	case "payload":
		frame.StateUpdates[0].Envelope.Payload = nil
	case "index":
		frame.StateUpdates[0].PartIndex = 1
	case "duplicate":
		frame.StateUpdates = append(frame.StateUpdates, frame.StateUpdates[0])
	case "text":
		frame.Identity.PartIDs[0] = "new-text"
		frame.Content = []ContentPart{TextPart{Text: "x"}}
	case "usage":
		frame.Usage = Usage{TotalTokens: -1, Known: []UsageCounter{UsageTotal}}
	case "identity":
		frame.Identity.PartIDs = nil
	case "expiry":
		expiry := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		frame.StateUpdates[0].Envelope.ExpiresAt = &expiry
	case "also-ordinary":
		frame.ProviderState = clonePartStates(frame.StateUpdates)
	}
}

func TestToolStateUpdateRequiredOmissionAndCanceledPartial(t *testing.T) {
	t.Parallel()
	// Arrange: required pending state arrives with an early provisional call.
	state := updateStateFixture(0, "pending-signature")
	stream := stateUpdateStream(&ResponseChunk{Kind: StreamDelta, Identity: StreamIdentity{PartIDs: []string{"tool"}},
		Content: []ContentPart{ToolCallPart{ID: "a", Name: "fn"}}, StateUpdates: []PartState{state}})
	// Act: early stop unwinds producer, retaining the last accepted envelope.
	for range stream.Events() {
		break
	}
	status := stream.Status()
	status.Result.Outcome = OutcomeIncomplete
	wire, err := LoggingResponse(status.Result)
	// Assert: no leaked state or false ability to continue omitted signature.
	require.NoError(t, err)
	require.Equal(t, StreamCanceled, status.State)
	require.Equal(t, []PartState{state}, status.Result.ProviderState)
	require.Empty(t, wire.ProviderState)
	require.True(t, wire.ContinuationUnavailable)
}

func TestIDlessStateUpdatesNeverCorrelateByName(t *testing.T) {
	t.Parallel()
	// Arrange: equal names and absent provider IDs, different source-proven refs.
	a, b := updateStateFixture(0, "pending-a"), updateStateFixture(1, "pending-b")
	closedB, closedA := updateStateFixture(0, "closed-b"), updateStateFixture(1, "closed-a")
	stream := stateUpdateStream(
		&ResponseChunk{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"local-a", "local-b"}},
			Content: []ContentPart{
				&ToolCallPart{Name: "fn"},
				&ToolCallPart{Name: "fn"},
			},
			StateUpdates: []PartState{a, b},
		},
		&ResponseChunk{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"local-b", "local-a"}},
			Content: []ContentPart{
				&ToolCallPart{Name: "fn", Args: `{"n":2}`},
				&ToolCallPart{Name: "fn", Args: `{"n":1}`},
			},
			StateUpdates: []PartState{closedB, closedA},
		},
		&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls},
	)
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert: pointer and value tool parts have the same validated semantics.
	status := stream.Status()
	require.Equal(t, StreamCompleted, status.State)
	require.Len(t, status.Result.Content, 2)
	require.Equal(t, 1, status.Result.ProviderState[0].PartIndex)
	require.Equal(t, 0, status.Result.ProviderState[1].PartIndex)
	require.Equal(t, "closed-b", string(status.Result.ProviderState[0].Envelope.Payload))
	require.Equal(t, "closed-a", string(status.Result.ProviderState[1].Envelope.Payload))
	wire, err := MarshalResponse(status.Result, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	restored, err := UnmarshalResponse(wire)
	require.NoError(t, err)
	require.Equal(t, status.Result.ProviderState, restored.ProviderState)
}

func TestToolStateCannotChangeWithoutExplicitUpdate(t *testing.T) {
	t.Parallel()
	// Arrange: state binds the first provisional part, not the assembled call.
	state := updateStateFixture(0, "pending")
	stream := stateUpdateStream(
		&ResponseChunk{
			Kind:          StreamDelta,
			Identity:      StreamIdentity{PartIDs: []string{"tool"}},
			Content:       []ContentPart{ToolCallPart{ID: "a", Name: "fn"}},
			ProviderState: []PartState{state},
		},
		&ResponseChunk{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"tool"}},
			Content:  []ContentPart{ToolCallPart{ID: "a", Name: "fn", Args: `{}`}},
		},
		&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeToolCalls},
	)
	// Act.
	for range stream.Events() {
	}
	// Assert: old exact-association invariant remains fail-closed.
	require.ErrorIs(t, stream.Status().Err, ErrProviderState)
	require.Equal(t, []PartState{state}, stream.Status().Result.ProviderState)
	stream = stateUpdateStream(
		&ResponseChunk{
			Kind:          StreamDelta,
			Identity:      StreamIdentity{PartIDs: []string{"tool"}},
			Content:       []ContentPart{ToolCallPart{ID: "a", Name: "fn"}},
			ProviderState: []PartState{state},
		},
		&ResponseChunk{
			Kind:          StreamDelta,
			Identity:      StreamIdentity{PartIDs: []string{"tool"}},
			Content:       []ContentPart{ToolCallPart{ID: "a", Name: "fn"}},
			ProviderState: []PartState{state},
		},
		&ResponseChunk{
			Kind:         StreamDelta,
			Identity:     StreamIdentity{PartIDs: []string{"tool"}},
			Content:      []ContentPart{ToolCallPart{ID: "a", Name: "fn", Args: `{}`}},
			StateUpdates: []PartState{state},
		},
	)
	for range stream.Events() {
	}
	require.ErrorIs(t, stream.Status().Err, ErrProviderState)
	require.Len(t, stream.Status().Result.ProviderState, 2)
}
