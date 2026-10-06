package prompty

import (
	"context"
	"encoding/json"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMessageAnnotationResponseHistoryWireIsolation(t *testing.T) {
	t.Parallel()
	// Arrange: metadata belongs to the message, even with no content part.
	annotations := []MessageAnnotation{
		{Kind: "provider.sources", Value: JSONDocument(`{"n":9007199254740993,"zero":0}`)},
	}
	response := NewResponse(nil)
	response.MessageAnnotations = annotations
	// Act: persist response and reuse its assistant message in history.
	wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeResponseJSON(data)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	execution := SimplePrompt("fixture").AddMessage(message).Clone().Normalize()
	executionWire, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	executionData, err := json.Marshal(executionWire)
	require.NoError(t, err)
	history, err := DecodeExecutionJSON(executionData)
	require.NoError(t, err)
	// Assert: no fake content or numeric loss, no mutable aliases.
	require.Empty(t, restored.Content)
	require.Equal(t, annotations, restored.MessageAnnotations)
	require.Equal(t, annotations, history.Messages[1].MessageAnnotations)
	wire.MessageAnnotations[0].Value[0] = 'x'
	message.MessageAnnotations[0].Value[0] = 'y'
	history.Messages[1].MessageAnnotations[0].Value[0] = 'z'
	require.Equal(t, annotations, restored.MessageAnnotations)
	require.Equal(t, byte('{'), response.MessageAnnotations[0].Value[0])
}

func TestMessageAnnotationsOrderedAfterFinishAndAtomicFailure(t *testing.T) {
	t.Parallel()
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "invalid"}[invalid], func(t *testing.T) {
			t.Parallel()
			// Arrange: metadata-only updates follow provider finish.
			first := MessageAnnotation{Kind: "provider.grounding", Value: JSONDocument(`{"chunks":["a"]}`)}
			second := MessageAnnotation{Kind: "provider.grounding", Value: JSONDocument(`{"indices":[0]}`)}
			events := []*ResponseChunk{
				{
					Kind:     StreamDelta,
					Identity: StreamIdentity{PartIDs: []string{"text"}},
					Content:  []ContentPart{TextPart{Text: "early"}},
				},
				{Kind: StreamFinish, Outcome: OutcomeCompleted},
				{Kind: StreamUsage, MessageAnnotations: []MessageAnnotation{first}},
				{Kind: StreamUsage, MessageAnnotations: []MessageAnnotation{second}},
			}
			if invalid {
				events = append(
					events,
					&ResponseChunk{Kind: StreamUsage, Usage: Usage{TotalTokens: 7, Known: []UsageCounter{UsageTotal}},
						MessageAnnotations: []MessageAnnotation{{Kind: " ", Value: JSONDocument(`{}`)}}},
				)
			}
			stream := NewStream(
				context.Background(),
				StreamNative,
				func(context.Context) iter.Seq2[*ResponseChunk, error] {
					return func(yield func(*ResponseChunk, error) bool) {
						for _, event := range events {
							if !yield(event, nil) {
								return
							}
						}
					}
				},
			)
			// Act: caller mutates delivered copies, not retained state.
			terminals := readMessageAnnotationStream(t, stream, invalid)
			// Assert: order, partial retention and single terminal; bad update is atomic.
			status := stream.Status()
			require.Equal(t, 1, terminals)
			require.Equal(t, []MessageAnnotation{first, second}, status.Result.MessageAnnotations)
			require.Len(t, status.Result.Content, 1)
			require.Zero(t, status.Result.Usage.TotalTokens)
			if invalid {
				require.Equal(t, StreamFailed, status.State)
			} else {
				require.Equal(t, StreamCompleted, status.State)
			}
			status.Result.MessageAnnotations[0].Value[0] = 'x'
			require.Equal(t, []MessageAnnotation{first, second}, stream.Status().Result.MessageAnnotations)
		})
	}
}

func readMessageAnnotationStream(t *testing.T, stream *Stream, invalid bool) int {
	t.Helper()
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == StreamTerminal {
			terminals++
			if invalid {
				require.ErrorIs(t, err, ErrProviderState)
			} else {
				require.NoError(t, err)
			}
		}
		if len(event.MessageAnnotations) != 0 {
			event.MessageAnnotations[0].Value[0] = 'x'
		}
	}
	return terminals
}

func TestMessageAnnotationInvalidEnvelopeRejected(t *testing.T) {
	t.Parallel()
	for _, annotation := range []MessageAnnotation{
		{Kind: "", Value: JSONDocument(`{}`)}, {Kind: " \t", Value: JSONDocument(`{}`)},
		{Kind: "provider", Value: JSONDocument(`{`)}, {Kind: "provider"},
	} {
		t.Run(annotation.Kind+string(annotation.Value), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			response := NewResponse([]ContentPart{TextPart{Text: "text"}})
			response.MessageAnnotations = []MessageAnnotation{annotation}
			// Act and assert: sync metadata, history, continuation and wire gates.
			require.ErrorIs(t, ValidateResponseMetadata(response), ErrProviderState)
			_, err := response.AssistantMessage()
			require.ErrorIs(t, err, ErrProviderState)
			_, err = MarshalResponse(response, WirePolicy{State: StatePreserve})
			require.ErrorIs(t, err, ErrProviderState)
			message := ChatMessage{Role: RoleAssistant, MessageAnnotations: response.MessageAnnotations}
			require.ErrorIs(
				t,
				ValidateContinuation([]ChatMessage{message}, ProfileIdentity{}, time.Time{}),
				ErrProviderState,
			)
		})
	}
}
