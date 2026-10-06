package prompty

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func identityFixtureFrame(ids []string, parts []ContentPart) *ResponseChunk {
	var frame ResponseChunk
	frame.Kind = StreamDelta
	frame.Identity.PartIDs = slices.Clone(ids)
	frame.Content = parts
	return &frame
}

func identityFixtureSource(frames ...*ResponseChunk) func(context.Context) iter.Seq2[*ResponseChunk, error] {
	return func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			for _, frame := range frames {
				if !yield(frame, nil) {
					return
				}
			}
		}
	}
}

func TestStreamIdentitySurvivesFramesAssemblyObserversAndConsumerMutation(t *testing.T) {
	t.Parallel()
	// Arrange: logical text identity spans fragments; interleaved calls collapse at terminal.
	frames := []*ResponseChunk{
		{Kind: StreamStart},
		identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: "hello"}}),
		identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: " world"}}),
		identityFixtureFrame([]string{"call:a", "call:b"}, []ContentPart{
			ToolCallPart{ID: "a", Name: "fn", ArgsChunk: `{"x":`},
			ToolCallPart{ID: "b", Name: "fn", ArgsChunk: `{"x":`},
		}),
		identityFixtureFrame([]string{"call:b", "call:a"}, []ContentPart{
			ToolCallPart{ID: "b", ArgsChunk: `2}`}, ToolCallPart{ID: "a", ArgsChunk: `1}`},
		}),
		{Kind: StreamFinish, Outcome: OutcomeToolCalls},
		{Kind: StreamUsage, Usage: Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}}},
	}
	var observed []StreamIdentity
	var terminalObserved StreamIdentity
	stream := NewStream(context.Background(), StreamNative, identityFixtureSource(frames...),
		WithStreamFrameObserver(func(_ context.Context, frame *ResponseChunk) error {
			observed = append(observed, cloneStreamIdentity(frame.Identity))
			if len(frame.Identity.PartIDs) != 0 {
				frame.Identity.PartIDs[0] = "observer mutation"
			}
			return nil
		}), WithStreamCaptureObserver(func(_ context.Context, status StreamStatus, err error) (*CaptureReport, error) {
			require.NoError(t, err)
			terminalObserved = cloneStreamIdentity(status.Identity)
			status.Identity.PartIDs[0] = "terminal observer mutation"
			report := validFixtureCaptureReport()
			return &report, nil
		}))
	messageID := stream.Status().Identity.MessageID
	require.NotEmpty(t, messageID)
	require.Empty(t, stream.Status().Identity.PartIDs)
	// Act: mutation of delivered identity cannot poison the accumulated binding.
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		require.Equal(t, messageID, frame.Identity.MessageID)
		if frame.Kind == StreamTerminal {
			terminals++
			require.Equal(t, terminalObserved, frame.Identity)
		}
		if len(frame.Identity.PartIDs) != 0 {
			frame.Identity.PartIDs[0] = "consumer mutation"
		}
	}
	status := stream.Status()
	// Assert: original fragments remain distinct, while tools retain their logical references after collapse.
	require.Equal(t, StreamCompleted, status.State)
	require.Equal(t, 1, terminals)
	require.Equal(t, []string{"text", "text", "call:a", "call:b"}, status.Identity.PartIDs)
	require.Equal(t, terminalObserved, status.Identity)
	require.Len(t, observed, len(frames))
	require.Equal(t, observed[1].PartIDs, observed[2].PartIDs)
	require.Len(t, status.Result.Content, 4)
	require.JSONEq(t, `{"x":1}`, status.Result.Content[2].(ToolCallPart).Args)
	require.JSONEq(t, `{"x":2}`, status.Result.Content[3].(ToolCallPart).Args)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	status.Identity.PartIDs[0] = "status mutation"
	require.Equal(t, "text", stream.Status().Identity.PartIDs[0])
	for _, sourceFrame := range frames {
		require.Empty(t, sourceFrame.Identity.MessageID, "source-owned frames were not changed")
	}
}

func TestStreamIdentityRejectsMalformedFramesAtomically(t *testing.T) {
	t.Parallel()
	foreign := identityFixtureFrame([]string{"keep"}, []ContentPart{TextPart{Text: "private"}})
	foreign.Identity.MessageID = "private foreign identity"
	for name, bad := range map[string]*ResponseChunk{
		"missing refs":  identityFixtureFrame(nil, []ContentPart{TextPart{Text: "private"}}),
		"extra refs":    identityFixtureFrame([]string{"keep", "extra"}, []ContentPart{TextPart{Text: "private"}}),
		"blank ref":     identityFixtureFrame([]string{" "}, []ContentPart{TextPart{Text: "private"}}),
		"duplicate ref": identityFixtureFrame([]string{"same", "same"}, []ContentPart{TextPart{}, TextPart{}}),
		"foreign":       foreign,
		"kind conflict": identityFixtureFrame([]string{"keep"}, []ContentPart{ReasoningPart{Text: "private"}}),
		"nil part":      identityFixtureFrame([]string{"new"}, []ContentPart{nil}),
		"typed nil":     identityFixtureFrame([]string{"new"}, []ContentPart{(*TextPart)(nil)}),
		"control refs":  identityFixtureFrame([]string{"ghost"}, nil),
		"partial update": identityFixtureFrame([]string{"new", "keep"}, []ContentPart{
			TextPart{Text: "private"}, ReasoningPart{Text: "private"},
		}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange: bad frame must not commit its refs, content or usage after a valid prefix.
			first := identityFixtureFrame([]string{"keep"}, []ContentPart{TextPart{Text: "accepted"}})
			first.Usage = Usage{TotalTokens: 2, Known: []UsageCounter{UsageTotal}}
			bad.Usage = Usage{TotalTokens: 100, Known: []UsageCounter{UsageTotal}}
			stream := NewStream(context.Background(), StreamNative, identityFixtureSource(first, bad))
			messageID := stream.Status().Identity.MessageID
			// Act.
			terminals := 0
			for frame, err := range stream.Events() {
				if frame.Kind == StreamTerminal {
					terminals++
					require.ErrorIs(t, err, ErrStreamProtocol)
					require.NotContains(t, err.Error(), "private")
				}
			}
			// Assert.
			status := stream.Status()
			require.Equal(t, StreamFailed, status.State)
			require.Equal(t, messageID, status.Identity.MessageID)
			require.Equal(t, []string{"keep"}, status.Identity.PartIDs)
			require.Equal(t, []ContentPart{TextPart{Text: "accepted"}}, status.Result.Content)
			require.Equal(t, 2, status.Result.Usage.TotalTokens)
			require.NotContains(t, stream.partBindings, "new")
			require.Equal(t, 1, terminals)
		})
	}
}

func TestStreamIdentityRejectsToolRebindingAndAliasing(t *testing.T) {
	t.Parallel()
	for name, bad := range map[string]*ResponseChunk{
		"changed provider ID": identityFixtureFrame([]string{"call"}, []ContentPart{ToolCallPart{ID: "b", ArgsChunk: `}`}}),
		"second logical ref":  identityFixtureFrame([]string{"other"}, []ContentPart{ToolCallPart{ID: "a", ArgsChunk: `}`}}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			first := identityFixtureFrame(
				[]string{"call"},
				[]ContentPart{ToolCallPart{ID: "a", Name: "fn", ArgsChunk: `{`}},
			)
			stream := NewStream(context.Background(), StreamNative, identityFixtureSource(first, bad))
			// Act.
			for frame, err := range stream.Events() {
				if frame.Kind == StreamTerminal {
					require.ErrorIs(t, err, ErrStreamProtocol)
				}
			}
			// Assert.
			require.Equal(t, StreamFailed, stream.Status().State)
			require.Equal(t, []string{"call"}, stream.Status().Identity.PartIDs)
			require.Equal(t, first.Content, stream.Status().Result.Content)
		})
	}
}

func TestStreamFrameObserverFailureRetainsIdentityAndReleasesSource(t *testing.T) {
	t.Parallel()
	// Arrange: observation failure must unwind a source waiting for cancellation.
	sentinel := errors.New("frame observer failure")
	closed := false
	stream := NewStream(context.Background(), StreamNative, func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			yield(identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: "accepted"}}), nil)
			<-ctx.Done()
			closed = true
		}
	}, WithStreamFrameObserver(func(context.Context, *ResponseChunk) error { return sentinel }))
	// Act.
	data, terminals := 0, 0
	for frame, err := range stream.Events() {
		if frame.Kind == StreamTerminal {
			terminals++
			require.ErrorIs(t, err, sentinel)
		} else {
			data++
			require.NoError(t, err)
		}
	}
	// Assert.
	require.True(t, closed)
	require.Equal(t, 1, data)
	require.Equal(t, 1, terminals)
	require.Equal(t, StreamFailed, stream.Status().State)
	require.Equal(t, []string{"text"}, stream.Status().Identity.PartIDs)
	require.Equal(t, []ContentPart{TextPart{Text: "accepted"}}, stream.Status().Result.Content)
}

func TestStreamIdentityRemainsReadableAfterConsumerStop(t *testing.T) {
	t.Parallel()
	// Arrange.
	stream := NewStream(context.Background(), StreamNative,
		identityFixtureSource(identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: "partial"}})))
	messageID := stream.Status().Identity.MessageID
	// Act.
	for frame := range stream.Events() {
		require.Equal(t, messageID, frame.Identity.MessageID)
		break
	}
	require.NoError(t, stream.Close())
	// Assert.
	require.Equal(t, StreamCanceled, stream.Status().State)
	require.Equal(t, messageID, stream.Status().Identity.MessageID)
	require.Equal(t, []string{"text"}, stream.Status().Identity.PartIDs)
}
