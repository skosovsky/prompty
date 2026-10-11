package prompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamScopedMetadataOnlyFailureRevokesContinuation(t *testing.T) {
	t.Parallel()
	for _, stop := range []string{"error", "break", "close"} {
		t.Run(stop, func(t *testing.T) {
			t.Parallel()
			assertScopedMetadataStreamFailure(t, stop)
		})
	}
}

func assertScopedMetadataStreamFailure(t *testing.T, stop string) {
	t.Helper()
	// Arrange: hosted-only output carries continuation without content parts.
	failure := errors.New("source failed")
	stream := NewStream(
		context.Background(),
		StreamNative,
		func(context.Context) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				if !yield(
					&ResponseChunk{
						Kind:               StreamDelta,
						MessageAnnotations: []MessageAnnotation{scopedAnnotationFixture()},
					},
					nil,
				) {
					return
				}
				yield(nil, failure)
			}
		},
	)
	// Act.
	for _, err := range stream.Events() {
		if stop == "break" {
			break
		}
		if stop == "close" {
			require.NoError(t, stream.Close())
			break
		}
		if err != nil {
			require.ErrorIs(t, err, failure)
		}
	}
	// Assert: evidence survives, but incomplete history cannot be replayed.
	result := stream.Status().Result
	require.Empty(t, result.Content)
	require.Empty(t, result.ProviderState)
	require.Len(t, result.MessageAnnotations, 1)
	require.True(t, result.ContinuationUnavailable)
}
