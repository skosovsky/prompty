package prompty

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseObservedModelWireAndClone(t *testing.T) {
	t.Parallel()
	// Arrange: selected profile differs from independently observed model.
	response := NewResponse([]ContentPart{TextPart{Text: "answer"}})
	response.Model = "observed-version"
	response.Diagnostics.Profile = ProfileIdentity{
		Provider: "fixture",
		Endpoint: "https://fixture.invalid",
		Model:    "requested",
	}
	// Act.
	wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	document, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeResponseJSON(document)
	require.NoError(t, err)
	clone := CloneResponse(response)
	clone.Content[0] = TextPart{Text: "changed"}
	clone.Model = "changed"
	// Assert: wire/ownership retains observed metadata without profile substitution.
	require.Equal(t, "observed-version", restored.Model)
	require.Equal(t, "requested", restored.Diagnostics.Profile.Model)
	require.Equal(t, "observed-version", response.Model)
	require.Equal(t, "answer", response.Content[0].(TextPart).Text)
	require.Nil(t, CloneResponse(nil))
}

func TestStreamObservationModelAndInterruptionCause(t *testing.T) {
	t.Parallel()
	for _, stop := range []StreamStopReason{"", StreamStopConsumer, StreamStopClosed, StreamStopCanceled} {
		t.Run(string(stop), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			stream := NewStream(ctx, StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
				return func(yield func(*ResponseChunk, error) bool) {
					if !yield(
						&ResponseChunk{
							Kind:     StreamDelta,
							Identity: StreamIdentity{PartIDs: []string{"text:0"}},
							Model:    "observed",
							Content:  []ContentPart{TextPart{Text: "known"}},
						},
						nil,
					) {
						return
					}
					yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil)
				}
			})
			// Act.
			for range stream.Events() {
				switch stop {
				case StreamStopConsumer:
				case StreamStopClosed:
					require.NoError(t, stream.Close())
				case StreamStopCanceled:
					cancel()
				default:
					continue
				}
				break
			}
			before := stream.Status()
			require.NoError(t, stream.Close())
			// Assert: interruption category and metadata survive, completed status immutable.
			require.Equal(t, stop, before.StopReason)
			require.Equal(t, "observed", before.Result.Model)
			require.Equal(t, before.StopReason, stream.Status().StopReason)
			if stop == "" {
				require.Equal(t, StreamCompleted, before.State)
			} else {
				require.Equal(t, StreamCanceled, before.State)
			}
		})
	}
}
