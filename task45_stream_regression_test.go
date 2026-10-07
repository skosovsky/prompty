package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStructuredEmptyFragmentsAreNoOps(t *testing.T) {
	t.Parallel()
	for _, mode := range []StreamMode{StreamNative, StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: empty first, middle and final fragments share the logical text part.
			invoker := &scriptedInvoker{
				generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
					frames := []*ResponseChunk{}
					for _, text := range []string{"", `{"answer":`, "", `"ok"}`, ""} {
						frames = append(
							frames,
							identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: text}}),
						)
					}
					frames = append(frames, &ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted})
					return identityFixtureSource(frames...)(context.Background())
				},
			}
			// Act.
			items, err := collectSeq(
				StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("fixture"), mode),
			)
			// Assert.
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, "ok", items[0].Answer)
		})
	}
	// Arrange: a document made only of empty fragments is still empty at finish.
	invoker := &scriptedInvoker{
		generateStream: func(ctx context.Context, _ *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			return identityFixtureSource(
				identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{}}),
				&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted},
			)(
				ctx,
			)
		},
	}
	// Act / Assert.
	_, err := collectSeq(
		StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("fixture"), StreamNative),
	)
	require.ErrorIs(t, err, ErrEmptyTextResponse)
	var nilText *TextPart
	for _, parts := range [][]ContentPart{{nilText}, {nil}, {MediaPart{MIMEType: "image/png", Data: []byte{1}}}} {
		_, err = strictTextPartsFromContent(parts)
		require.Error(t, err)
	}
}

func TestStreamTrailingMetadataCannotRewriteFinish(t *testing.T) {
	t.Parallel()
	for _, outcome := range []Outcome{OutcomeCompleted, OutcomeIncomplete, OutcomeRefusal, OutcomePaused, OutcomeToolCalls} {
		for _, bad := range []*ResponseChunk{
			{Kind: StreamUsage, Outcome: Outcome("invalid"), Usage: Usage{TotalTokens: 99, Known: []UsageCounter{UsageTotal}}},
			{Kind: StreamUsage, Outcome: OutcomeCompleted, FinishReason: "changed", Usage: Usage{TotalTokens: 99, Known: []UsageCounter{UsageTotal}}},
		} {
			t.Run(string(outcome)+":"+string(bad.Outcome)+":"+bad.FinishReason, func(t *testing.T) {
				t.Parallel()
				// Arrange: the good late frame establishes usage before the contradictory one.
				captureCalls := 0
				stream := NewStream(context.Background(), StreamNative, identityFixtureSource(
					identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: "prefix"}}),
					&ResponseChunk{Kind: StreamFinish, Outcome: outcome, FinishReason: "accepted"},
					&ResponseChunk{
						Kind:         StreamUsage,
						Outcome:      outcome,
						FinishReason: "accepted",
						Usage:        Usage{TotalTokens: 7, Known: []UsageCounter{UsageTotal}},
					},
					bad,
				), WithStreamCaptureObserver(func(_ context.Context, status StreamStatus, err error) (*CaptureReport, error) {
					captureCalls++
					require.ErrorIs(t, err, ErrStreamProtocol)
					require.Equal(t, outcome, status.Result.Outcome)
					report := validFixtureCaptureReport()
					return &report, nil
				}))
				// Act.
				terminals := 0
				for frame, err := range stream.Events() {
					if frame.Kind == StreamTerminal {
						terminals++
						require.ErrorIs(t, err, ErrStreamProtocol)
					}
				}
				result := stream.Status()
				// Assert: the bad frame contributes nothing, and finish remains evidence.
				require.Equal(t, StreamFailed, result.State)
				require.Equal(t, outcome, result.Result.Outcome)
				require.Equal(t, "accepted", result.Result.FinishReason)
				require.Equal(t, 7, result.Result.Usage.TotalTokens)
				require.True(t, result.Result.ContinuationUnavailable)
				require.Equal(t, "prefix", result.Result.Content[0].(TextPart).Text)
				require.Equal(t, 1, terminals)
				require.Equal(t, 1, captureCalls)
			})
		}
	}
}

func TestStreamSourceShapesAndMetadataControls(t *testing.T) {
	t.Parallel()
	for _, bad := range []*ResponseChunk{
		{Kind: StreamStart, Content: []ContentPart{TextPart{Text: "bad"}}, Identity: StreamIdentity{PartIDs: []string{"text"}}},
		{Kind: StreamUsage, Content: []ContentPart{TextPart{Text: "bad"}}, Identity: StreamIdentity{PartIDs: []string{"text"}}},
		{Kind: StreamUsage, ProviderState: []PartState{{PartIndex: 0}}},
		{Kind: StreamUsage, StateUpdates: []PartState{{PartIndex: 0}}},
		{Kind: StreamUsage, Finalizations: []StreamPartFinalization{{PartID: "text"}}},
		{Kind: StreamUsage, Annotations: []PartAnnotation{{PartIndex: 0}}},
		{Kind: StreamDelta, Outcome: Outcome("invalid")},
		{Kind: StreamStart, Outcome: Outcome("invalid")},
		{Kind: StreamDelta, Result: NewResponse(nil)},
		{Kind: StreamTerminal},
	} {
		// Arrange / Act: invalid frames cannot install revocation or other metadata.
		bad.ContinuationUnavailable = true
		_, err := checkStreamEvent(bad, false, "", "")
		// Assert.
		require.ErrorIs(t, err, ErrStreamProtocol)
	}
	// Arrange: Start is optional/repeatable, revocation monotonic, annotations can be late.
	annotation := scopedAnnotationFixture()
	stream := NewStream(context.Background(), StreamNative, identityFixtureSource(
		&ResponseChunk{Kind: StreamStart, ContinuationUnavailable: true}, &ResponseChunk{Kind: StreamStart},
		&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted, FinishReason: "stop"},
		&ResponseChunk{Kind: StreamUsage, MessageAnnotations: []MessageAnnotation{annotation}},
		&ResponseChunk{Kind: StreamUsage, Outcome: OutcomeCompleted, FinishReason: "stop"},
	))
	// Act.
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if frame.Kind == StreamTerminal {
			terminals++
		}
	}
	// Assert.
	require.Equal(t, 1, terminals)
	require.Equal(t, StreamCompleted, stream.Status().State)
	require.True(t, stream.Status().Result.ContinuationUnavailable)
	require.Equal(t, []MessageAnnotation{annotation}, stream.Status().Result.MessageAnnotations)
}

func TestFinishClearsProvisionalReason(t *testing.T) {
	t.Parallel()
	// Arrange: an empty accepted finish reason is still authoritative.
	stream := NewStream(context.Background(), StreamNative, identityFixtureSource(
		&ResponseChunk{Kind: StreamDelta, FinishReason: "provisional"},
		&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted},
		&ResponseChunk{Kind: StreamUsage, FinishReason: "provisional"},
	))
	// Act.
	for range stream.Events() {
	}
	// Assert: the rejected late frame cannot revive the provisional reason.
	require.ErrorIs(t, stream.Status().Err, ErrStreamProtocol)
	require.Empty(t, stream.Status().Result.FinishReason)
}

func TestStructuredFinalOutcomeOverridesProvisionalMetadata(t *testing.T) {
	t.Parallel()
	for _, kind := range []StreamEventKind{StreamStart, StreamDelta, StreamUsage} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			// Arrange: provisional incomplete metadata is not an authoritative finish.
			invoker := &scriptedInvoker{
				generateStream: func(ctx context.Context, _ *PromptExecution) iter.Seq2[*ResponseChunk, error] {
					return identityFixtureSource(
						&ResponseChunk{Kind: kind, Outcome: OutcomeIncomplete},
						identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: `{"answer":"ok"}`}}),
						&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted},
					)(ctx)
				},
			}
			// Act / Assert.
			items, err := collectSeq(
				StreamStructuredOutput[valueSchemaResult](t.Context(), invoker, SimplePrompt("fixture"), StreamNative),
			)
			require.NoError(t, err)
			require.Equal(t, []valueSchemaResult{{Answer: "ok"}}, items)
		})
	}
	for _, outcome := range []Outcome{OutcomeIncomplete, OutcomeRefusal, OutcomePaused, OutcomeToolCalls} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			// Arrange: valid provisional JSON cannot promote an authoritative noncompleted finish.
			invoker := &scriptedInvoker{
				generateStream: func(ctx context.Context, _ *PromptExecution) iter.Seq2[*ResponseChunk, error] {
					return identityFixtureSource(
						identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: `{"answer":"ok"}`}}),
						&ResponseChunk{Kind: StreamFinish, Outcome: outcome},
						&ResponseChunk{Kind: StreamUsage, Outcome: OutcomeCompleted},
					)(ctx)
				},
			}
			// Act / Assert: metadata promotion fails the same handle's lifecycle.
			_, err := collectSeq(
				StreamStructuredOutput[valueSchemaResult](t.Context(), invoker, SimplePrompt("fixture"), StreamNative),
			)
			require.ErrorIs(t, err, ErrStreamProtocol)
		})
	}
}

func TestInvalidSourceFrameInstallsNoMetadata(t *testing.T) {
	t.Parallel()
	for _, bad := range []*ResponseChunk{
		{Kind: StreamStart, Content: []ContentPart{TextPart{Text: "bad"}}, Identity: StreamIdentity{PartIDs: []string{"bad"}}},
		{Kind: StreamUsage, Content: []ContentPart{TextPart{Text: "bad"}}, Identity: StreamIdentity{PartIDs: []string{"bad"}}},
		{Kind: StreamUsage, StateUpdates: []PartState{{PartIndex: 0}}},
		{Kind: StreamUsage, ProviderState: []PartState{{PartIndex: 0}}},
		{Kind: StreamUsage, Annotations: []PartAnnotation{{PartIndex: 0}}},
		{Kind: StreamUsage, Finalizations: []StreamPartFinalization{{PartID: "text"}}},
		{Kind: StreamStart, Outcome: Outcome("invalid")},
		{Kind: StreamDelta, Outcome: Outcome("invalid")},
		{Kind: StreamUsage, Outcome: Outcome("invalid")},
		{Kind: StreamTerminal},
		{Kind: StreamDelta, Result: NewResponse(nil)},
	} {
		// Arrange: the invalid frame has enticing metadata which must remain unobserved.
		bad.Model = "unaccepted"
		bad.Usage = Usage{TotalTokens: 99, Known: []UsageCounter{UsageTotal}}
		observations := 0
		good := identityFixtureFrame([]string{"text"}, []ContentPart{TextPart{Text: "prefix"}})
		good.Model = "accepted"
		stream := NewStream(
			t.Context(),
			StreamNative,
			identityFixtureSource(good, bad),
			WithStreamFrameObserver(func(context.Context, *ResponseChunk) error { observations++; return nil }),
		)
		// Act.
		terminals := 0
		for frame, err := range stream.Events() {
			if frame.Kind == StreamTerminal {
				terminals++
				require.ErrorIs(t, err, ErrStreamProtocol)
			}
		}
		// Assert: only the previously accepted prefix/metadata remain.
		require.Equal(t, "accepted", stream.Status().Result.Model)
		require.Zero(t, stream.Status().Result.Usage.TotalTokens)
		require.Equal(t, []ContentPart{TextPart{Text: "prefix"}}, stream.Status().Result.Content)
		require.Equal(t, 1, observations)
		require.Equal(t, 1, terminals)
	}
}
