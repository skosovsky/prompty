package otelprompty

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type identityInvokerFixture struct {
	stream   *prompty.Stream
	response *prompty.Response
	err      error
}

func (f identityInvokerFixture) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	return f.response, f.err
}

func (f identityInvokerFixture) ExecuteStream(
	context.Context,
	*prompty.PromptExecution,
	prompty.StreamMode,
) *prompty.Stream {
	return f.stream
}

func TestTracingKeepsOriginalHandleIdentityAndFinalizer(t *testing.T) {
	t.Parallel()
	// Arrange: an original finalizer fails after known content/usage has been received.
	sentinel := errors.New("original finalizer failure")
	called, finalized := 0, false
	stream := prompty.NewStream(context.Background(), prompty.StreamNative,
		func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				called++
				yield(&prompty.ResponseChunk{
					Identity: prompty.StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
					Kind:     prompty.StreamFinish, Outcome: prompty.OutcomeCompleted,
					Content: []prompty.ContentPart{prompty.TextPart{Text: "known"}},
					Usage:   prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}},
				}, nil)
			}
		}, prompty.WithStreamFinalizer(func(context.Context, *prompty.Response) error {
			finalized = true
			return sentinel
		}))
	messageID := stream.Status().Identity.MessageID
	base := identityInvokerFixture{stream: stream, response: nil, err: nil}
	wrapped := WithTracing()(
		base,
	).ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	require.Same(t, stream, wrapped)
	require.Zero(t, called)
	// Act.
	terminals := 0
	for frame, err := range wrapped.Events() {
		require.Equal(t, messageID, frame.Identity.MessageID)
		if frame.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, sentinel)
		}
	}
	// Assert.
	require.True(t, finalized)
	require.Equal(t, 1, called)
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.StreamFailed, wrapped.Status().State)
	require.Equal(t, []string{"text"}, wrapped.Status().Identity.PartIDs)
	require.Equal(t, 9, wrapped.Status().Result.Usage.TotalTokens)
}

func TestTracingRetainsKnownSyncResponseOnFailure(t *testing.T) {
	t.Parallel()
	// Arrange: schema/metadata validation may return the known response together with an error.
	sentinel := errors.New("known response validation failure")
	known := prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "known"}})
	base := identityInvokerFixture{stream: nil, response: known, err: sentinel}
	// Act.
	response, err := WithTracing()(base).Execute(context.Background(), prompty.SimplePrompt("fixture"))
	// Assert.
	require.ErrorIs(t, err, sentinel)
	require.Same(t, known, response)
}
