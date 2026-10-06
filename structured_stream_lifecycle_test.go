package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

var errStructuredFixtureDomain = errors.New("fixture domain rejection")

func TestStructuredControlAndTerminalConformance(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, text string
		finish     bool
		canceled   bool
		items      int
		want       error
	}{
		{"completed", "{}", true, false, 1, nil},
		{"empty document", "", true, false, 0, ErrEmptyTextResponse},
		{"missing finish", "{}", false, false, 1, ErrStreamProtocol},
		{"truncated document", "{", true, false, 0, ErrStructuredStream},
		{"canceled before source", "{}", true, true, 0, context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: control-only events surround content without closing transport early.
			sourceCalls := 0
			invoker := &structuredLifecycleInvoker{source: func(context.Context) iter.Seq2[*ResponseChunk, error] {
				sourceCalls++
				return structuredControlFixtureEvents(test.text, test.finish)
			}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.canceled {
				cancel()
			}
			// Act.
			items, err := collectSeq(
				StreamStructuredOutput[struct{}](ctx, invoker, SimplePrompt("fixture"), StreamNative),
			)
			// Assert: provisional items do not turn missing/invalid completion into success.
			require.Len(t, items, test.items)
			if test.want == nil {
				require.NoError(t, err)
				require.Equal(t, StreamCompleted, invoker.stream.Status().State)
			} else {
				require.ErrorIs(t, err, test.want)
			}
			if test.canceled {
				require.Zero(t, sourceCalls)
				require.Equal(t, StreamCanceled, invoker.stream.Status().State)
			} else {
				require.Equal(t, 1, sourceCalls)
				require.Equal(t, 9, invoker.stream.Status().Result.Usage.TotalTokens)
				require.Equal(t, 1, invoker.observations)
				if test.want != nil {
					require.Equal(t, StreamFailed, invoker.stream.Status().State)
				}
			}
		})
	}
}

func structuredControlFixtureEvents(text string, finish bool) iter.Seq2[*ResponseChunk, error] {
	return func(yield func(*ResponseChunk, error) bool) {
		if !yield(&ResponseChunk{Kind: StreamStart}, nil) {
			return
		}
		if text != "" && !yield(&ResponseChunk{Kind: StreamDelta,
			Identity: StreamIdentity{PartIDs: []string{"text"}}, Content: []ContentPart{TextPart{Text: text}}}, nil) {
			return
		}
		if finish && !yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil) {
			return
		}
		yield(&ResponseChunk{Kind: StreamUsage, Usage: Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}}}, nil)
	}
}

type structuredDomainFixture struct {
	Value int `json:"value"`
}

func (structuredDomainFixture) Validate() error { return errStructuredFixtureDomain }

func TestStructuredLifecyclePreservesDomainValidationCause(t *testing.T) {
	t.Parallel()
	// Arrange: JSON and schema are valid, but the caller rejects the decoded value.
	invoker := structuredSingleDocumentInvoker(`{"value":1}`)
	// Act.
	items, err := collectSeq(StreamStructuredOutput[structuredDomainFixture](
		context.Background(), invoker, SimplePrompt("fixture"), StreamNative))
	// Assert: domain validation is neither a parse error nor cancellation.
	require.Empty(t, items)
	require.ErrorIs(t, err, errStructuredFixtureDomain)
	require.NotErrorIs(t, err, ErrStructuredStream)
	require.NotErrorIs(t, err, context.Canceled)
	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, StreamFailed, invoker.stream.Status().State)
	require.ErrorIs(t, invoker.stream.Status().Err, errStructuredFixtureDomain)
	require.Zero(t, invoker.finalized)
	require.Equal(t, 1, invoker.observations)
}

func TestStructuredLifecyclePreservesJSONSyntaxCause(t *testing.T) {
	t.Parallel()
	// Arrange: a balanced object still has invalid JSON syntax.
	invoker := structuredSingleDocumentInvoker(`{"value":}`)
	// Act.
	items, err := collectSeq(StreamStructuredOutput[structuredDomainFixture](
		context.Background(), invoker, SimplePrompt("fixture"), StreamNative))
	// Assert: retain both the portable parse category and the decoder's typed cause.
	require.Empty(t, items)
	require.ErrorIs(t, err, ErrStructuredStream)
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
	require.NotErrorIs(t, err, errStructuredFixtureDomain)
	require.Equal(t, StreamFailed, invoker.stream.Status().State)
	require.Zero(t, invoker.finalized)
	require.Equal(t, 1, invoker.observations)
}

func structuredSingleDocumentInvoker(document string) *structuredLifecycleInvoker {
	return &structuredLifecycleInvoker{source: func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			if !yield(
				&ResponseChunk{
					Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
					Kind:     StreamDelta,
					Content:  []ContentPart{TextPart{Text: document}},
				},
				nil,
			) {
				<-ctx.Done()
				return
			}
			yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil)
		}
	}}
}

type structuredLifecycleInvoker struct {
	source       func(context.Context) iter.Seq2[*ResponseChunk, error]
	stream       *Stream
	calls        int
	observations int
	finalized    int
}

func (*structuredLifecycleInvoker) Execute(context.Context, *PromptExecution) (*Response, error) {
	return nil, ErrStreamMode
}

func (i *structuredLifecycleInvoker) ExecuteStream(ctx context.Context, _ *PromptExecution, mode StreamMode) *Stream {
	i.calls++
	i.stream = NewStream(ctx, mode, i.source, WithStreamFinalizer(func(context.Context, *Response) error {
		i.finalized++
		return nil
	}), WithStreamCaptureObserver(func(context.Context, StreamStatus, error) (*CaptureReport, error) {
		i.observations++
		report := validFixtureCaptureReport()
		return &report, nil
	}))
	return i.stream
}

func TestStructuredArrayLifecycleRejectsInvalidSeparators(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		text  string
		items int
		valid bool
	}{
		{"[]", 0, true}, {"[{}]", 1, true}, {"[{},{}]", 2, true},
		{" [ \n{} \t, \r{} ] ", 2, true},
		{"[,{}]", 0, false}, {"[{},,{}]", 1, false}, {"[{},]", 1, false}, {"[{}{}]", 1, false},
		{"[{}", 1, false},
	} {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()
			// Arrange: every separator/whitespace byte has its own delta.
			cleaned := make(chan struct{})
			invoker := &structuredLifecycleInvoker{source: func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
				return func(yield func(*ResponseChunk, error) bool) {
					defer close(cleaned)
					for _, ch := range test.text {
						if !yield(
							&ResponseChunk{
								Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
								Kind:     StreamDelta,
								Content:  []ContentPart{TextPart{Text: string(ch)}},
							},
							nil,
						) {
							<-ctx.Done() // validation must release the producer before iterator unwinds
							return
						}
					}
					if !yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil) {
						return
					}
					yield(
						&ResponseChunk{
							Kind:  StreamUsage,
							Usage: Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}},
						},
						nil,
					)
				}
			}}
			// Act.
			items, err := collectSeq(StreamStructuredOutput[struct{}](
				context.Background(), invoker, SimplePrompt("fixture"), StreamNative))
			status := invoker.stream.Status()
			// Assert: prior provisional items remain visible but cannot hide document failure.
			<-cleaned
			require.Len(t, items, test.items)
			require.Equal(t, 1, invoker.calls)
			require.Equal(t, 1, invoker.observations)
			assertStructuredLifecycleResult(t, test.valid, err, status, invoker.finalized)
		})
	}
}

func assertStructuredLifecycleResult(t *testing.T, valid bool, err error, status StreamStatus, finalized int) {
	t.Helper()
	if valid {
		require.NoError(t, err)
		require.Equal(t, StreamCompleted, status.State)
		require.Equal(t, 1, finalized)
		require.Equal(t, 9, status.Result.Usage.TotalTokens)
	} else {
		require.Error(t, err)
		require.Equal(t, StreamFailed, status.State)
		require.Error(t, status.Err)
		require.ErrorIs(t, status.Err, ErrStructuredStream)
		require.NotErrorIs(t, status.Err, context.Canceled)
		require.NotEmpty(t, status.Result.Content)
		require.Zero(t, finalized)
	}
}

func TestStructuredLifecycleRefusalPreservesTrailingUsage(t *testing.T) {
	t.Parallel()
	// Arrange: refusal text is not a JSON document and must not be classified as parse failure.
	invoker := &structuredLifecycleInvoker{source: func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			if !yield(
				&ResponseChunk{
					Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
					Kind:     StreamDelta,
					Outcome:  OutcomeRefusal,
					Content:  []ContentPart{TextPart{Text: "refused"}},
				},
				nil,
			) {
				return
			}
			if !yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeRefusal}, nil) {
				return
			}
			yield(
				&ResponseChunk{Kind: StreamUsage, Usage: Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}}},
				nil,
			)
		}
	}}
	// Act.
	items, err := collectSeq(
		StreamStructuredOutput[struct{}](context.Background(), invoker, SimplePrompt("fixture"), StreamNative),
	)
	// Assert.
	require.Empty(t, items)
	require.ErrorIs(t, err, ErrResponseOutcome)
	status := invoker.stream.Status()
	require.Equal(t, StreamFailed, status.State)
	require.Equal(t, OutcomeRefusal, status.Result.Outcome)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Equal(t, 1, invoker.observations)
	require.Equal(t, 1, invoker.calls)
}

func TestStructuredLifecycleConsumerBreakRemainsCanceled(t *testing.T) {
	t.Parallel()
	// Arrange: two provisional items in a single received frame.
	cleaned := make(chan struct{})
	invoker := &structuredLifecycleInvoker{source: func(ctx context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			defer close(cleaned)
			yield(
				&ResponseChunk{
					Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
					Kind:     StreamDelta,
					Content:  []ContentPart{TextPart{Text: "[{},{}]"}},
				},
				nil,
			)
			<-ctx.Done()
		}
	}}
	// Act.
	items := 0
	for _, err := range StreamStructuredOutput[struct{}](context.Background(), invoker, SimplePrompt("fixture"), StreamNative) {
		require.NoError(t, err)
		items++
		break
	}
	// Assert: voluntary stop is neither failed parsing nor a second item/terminal yield.
	<-cleaned
	require.Equal(t, 1, items)
	require.Equal(t, StreamCanceled, invoker.stream.Status().State)
	require.ErrorIs(t, invoker.stream.Status().Err, context.Canceled)
	require.Equal(t, 1, invoker.observations, "consumer stop still closes capture")
	require.Zero(t, invoker.finalized)
}
