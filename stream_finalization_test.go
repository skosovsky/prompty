package prompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func finalizationFixture(events []*ResponseChunk, failure error) *Stream {
	return NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			for _, event := range events {
				if !yield(event, nil) {
					return
				}
			}
			if failure != nil {
				yield(nil, failure)
			}
		}
	})
}

func TestStreamPartFinalizationReindexesAndRetainsLateFailure(t *testing.T) {
	t.Parallel()
	// Arrange: interleaved reasoning fragments, ordinary text and a correlated local call.
	envelope := ProviderEnvelope{Format: ProviderStateFormat,
		Scope: ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"},
		Codec: "opaque", Required: true, Payload: JSONDocument(`{"private":true}`)}
	finish := &ResponseChunk{Kind: StreamDelta, Finalizations: []StreamPartFinalization{
		{PartID: "reason", Content: ReasoningPart{Text: "ab"}, ProviderState: &envelope,
			Annotations: []PartAnnotation{{PartIndex: 0, Kind: "summary", Value: JSONDocument(`{"span":2}`)}}},
		{PartID: "tool", Content: ToolCallPart{ID: "call", Name: "lookup", Args: `{"n":1}`}},
	}}
	events := []*ResponseChunk{
		{Kind: StreamDelta, Identity: StreamIdentity{PartIDs: []string{"reason", "text", "tool"}},
			Content: []ContentPart{
				ReasoningPart{Text: "a"},
				TextPart{Text: "ordinary"},
				ToolCallPart{ID: "call", Name: "lookup", ArgsChunk: `{"n":`},
			},
			Annotations: []PartAnnotation{{PartIndex: 1, Kind: "citation", Value: JSONDocument(`{"url":"fixture"}`)}}},
		{Kind: StreamDelta, Identity: StreamIdentity{PartIDs: []string{"reason", "tool"}},
			Content: []ContentPart{
				ReasoningPart{Text: "b"},
				ToolCallPart{ID: "call", Name: "lookup", ArgsChunk: `1}`},
			}},
		finish,
	}
	failure := errors.New("late provider read failure")
	stream := finalizationFixture(events, failure)
	// Act: mutate yielded finalizations and Status copies; the handle owns its data.
	for event, err := range stream.Events() {
		if event.Kind == StreamTerminal {
			require.ErrorIs(t, err, failure)
			continue
		}
		require.NoError(t, err)
		if len(event.Finalizations) != 0 {
			event.Finalizations[0].ProviderState.Payload[0] = 'x'
		}
	}
	status := stream.Status()
	// Assert: finalization was applied before EOF failure; state survived but continuation was revoked.
	require.Equal(t, StreamFailed, status.State)
	require.Equal(t, []string{"reason", "text", "tool"}, status.Identity.PartIDs)
	require.Equal(
		t,
		[]ContentPart{
			ReasoningPart{Text: "ab"},
			TextPart{Text: "ordinary"},
			ToolCallPart{ID: "call", Name: "lookup", Args: `{"n":1}`},
		},
		status.Result.Content,
	)
	require.Equal(t, []PartState{{PartIndex: 0, Envelope: envelope}}, status.Result.ProviderState)
	require.Equal(t, 1, status.Result.Annotations[0].PartIndex)
	require.Equal(t, 0, status.Result.Annotations[1].PartIndex)
	require.True(t, status.Result.ContinuationUnavailable)
	status.Result.ProviderState[0].Envelope.Payload[0] = 'z'
	require.Equal(t, envelope.Payload, stream.Status().Result.ProviderState[0].Envelope.Payload)
}

func TestStreamFinalizationRejectsContradictionsAtomically(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unknown", "text change", "type change", "duplicate", "late delta", "after finish", "cache policy"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			events := contradictionFinalizationEvents(scenario)
			stream := finalizationFixture(events, nil)
			// Act.
			for _, err := range stream.Events() {
				if err != nil {
					require.ErrorIs(t, err, ErrStreamProtocol)
				}
			}
			// Assert: failed multi-update does not commit its successful first update.
			status := stream.Status()
			require.Equal(t, StreamFailed, status.State)
			require.ErrorIs(t, status.Err, ErrStreamProtocol)
			if scenario == "late delta" {
				require.Equal(t, []ContentPart{TextPart{Text: "ab"}}, status.Result.Content)
			} else {
				require.Equal(t, []ContentPart{TextPart{Text: "a"}, TextPart{Text: "b"}}, status.Result.Content)
			}
		})
	}
}

func TestStreamFinalizationCannotRebindExistingState(t *testing.T) {
	t.Parallel()
	// Arrange: immutable existing state-bound text may not be merged with another fragment.
	envelope := ProviderEnvelope{
		Format:  ProviderStateFormat,
		Scope:   ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"},
		Codec:   "opaque",
		Payload: JSONDocument(`{}`),
	}
	stream := finalizationFixture([]*ResponseChunk{
		{
			Kind:          StreamDelta,
			Identity:      StreamIdentity{PartIDs: []string{"text"}},
			Content:       []ContentPart{TextPart{Text: "a"}},
			ProviderState: []PartState{{PartIndex: 0, Envelope: envelope}},
		},
		{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"text"}},
			Content:  []ContentPart{TextPart{Text: "b"}},
		},
		{
			Kind: StreamDelta,
			Finalizations: []StreamPartFinalization{
				{PartID: "text", Content: TextPart{Text: "ab"}, ProviderState: &envelope},
			},
		},
	}, nil)
	// Act.
	for _, err := range stream.Events() {
		if err != nil {
			require.ErrorIs(t, err, ErrProviderState)
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, StreamFailed, status.State)
	require.Equal(t, []ContentPart{TextPart{Text: "a"}, TextPart{Text: "b"}}, status.Result.Content)
	require.Equal(t, []PartState{{PartIndex: 0, Envelope: envelope}}, status.Result.ProviderState)
}

func TestStreamFinalizationIDLessToolAndCompleteText(t *testing.T) {
	t.Parallel()
	// Arrange: logical references correlate a native ID-less call without inventing a provider ID.
	stream := finalizationFixture([]*ResponseChunk{
		{Kind: StreamDelta, Identity: StreamIdentity{PartIDs: []string{"text", "tool"}},
			Content: []ContentPart{TextPart{Text: "before"}, ToolCallPart{Name: "lookup", ArgsChunk: `{"n":`}}},
		{Kind: StreamDelta, Identity: StreamIdentity{PartIDs: []string{"tool", "text"}},
			Content: []ContentPart{ToolCallPart{Name: "lookup", ArgsChunk: `1}`}, TextPart{Text: " after"}}},
		{Kind: StreamDelta, Finalizations: []StreamPartFinalization{
			{PartID: "text", Content: TextPart{Text: "before after"}},
			{PartID: "tool", Content: ToolCallPart{Name: "lookup", Args: `{"n":1}`}},
		}},
		{Kind: StreamFinish, Outcome: OutcomeToolCalls},
	}, nil)
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, StreamCompleted, status.State)
	require.Equal(t, []string{"text", "tool"}, status.Identity.PartIDs)
	require.Equal(
		t,
		[]ContentPart{TextPart{Text: "before after"}, ToolCallPart{Name: "lookup", Args: `{"n":1}`}},
		status.Result.Content,
	)
	_, err := status.Result.AssistantMessage()
	require.NoError(t, err)
}

func contradictionFinalizationEvents(scenario string) []*ResponseChunk {
	update := StreamPartFinalization{PartID: "text", Content: TextPart{Text: "ab"}}
	events := []*ResponseChunk{
		{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"text"}},
			Content:  []ContentPart{TextPart{Text: "a"}},
		},
		{
			Kind:     StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"text"}},
			Content:  []ContentPart{TextPart{Text: "b"}},
		},
	}
	switch scenario {
	case "unknown":
		update.PartID = "missing"
	case "text change":
		update.Content = TextPart{Text: "changed"}
	case "type change":
		update.Content = ReasoningPart{Text: "ab"}
	case "cache policy":
		update.Content = TextPart{Text: "ab", CachePolicy: &CachePolicy{Mechanism: CacheBlockMarker}}
	}
	final := &ResponseChunk{Kind: StreamDelta, Finalizations: []StreamPartFinalization{update}}
	if scenario == "duplicate" {
		final.Finalizations = append(final.Finalizations, update)
	}
	if scenario == "after finish" {
		events = append(events, &ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted})
		final.Kind = StreamUsage
	}
	events = append(events, final)
	if scenario == "late delta" {
		events = append(
			events,
			&ResponseChunk{
				Kind:     StreamDelta,
				Identity: StreamIdentity{PartIDs: []string{"text"}},
				Content:  []ContentPart{TextPart{Text: "late"}},
			},
		)
	}
	return events
}
