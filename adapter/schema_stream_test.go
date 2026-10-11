package adapter

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type nativeLoweringFixture struct {
	*loweringFixture

	value string
}

func (f *nativeLoweringFixture) StreamEvents(
	context.Context,
	*prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		f.calls++
		for _, event := range []*prompty.ResponseChunk{
			{Identity: prompty.StreamIdentity{MessageID: "", PartIDs: []string{"text"}}, Kind: prompty.StreamDelta, Content: []prompty.ContentPart{prompty.TextPart{Text: f.value}}},
			{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeCompleted, FinishReason: "stop"},
			{Kind: prompty.StreamUsage, Usage: prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}}},
		} {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func TestNativeSchemaFinalizationRejectsOriginalMismatchAndRetainsUsage(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &nativeLoweringFixture{
		loweringFixture: &loweringFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()}}, value: "1",
	}
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`)}
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode:              prompty.SchemaLowering,
		AllowedTransforms: []string{"fixture.drop-minimum"},
	}
	stream := NewClient(fixture).ExecuteStream(context.Background(), exec, prompty.StreamNative)
	require.Zero(t, fixture.calls)
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Len(t, status.Result.Diagnostics.Schemas, 1)
	require.Equal(t, 1, terminals)
	require.Equal(t, 1, fixture.calls)
}

func TestBufferedValidationErrorRetainsKnownResponse(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &loweringFixture{
		checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
		response:       prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "1"}}),
	}
	fixture.response.Usage = prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}}
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`)}
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode:              prompty.SchemaLowering,
		AllowedTransforms: []string{"fixture.drop-minimum"},
	}
	stream := NewClient(fixture).ExecuteStream(context.Background(), exec, prompty.StreamBuffered)
	// Act.
	terminals := 0
	for event := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.ErrorIs(t, status.Err, prompty.ErrSchemaMismatch)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Equal(t, prompty.OutcomeCompleted, status.Result.Outcome)
	require.Len(t, status.Result.Content, 1)
	require.Equal(t, 1, fixture.calls)
	require.Equal(t, 1, terminals)
}

type bufferedMetadataFixture struct {
	*checkedFixture

	response *prompty.Response
}

func (f *bufferedMetadataFixture) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	f.calls++
	return f.response, nil
}

func TestBufferedStreamIsLazySingleUseAndPreservesFinishMetadata(t *testing.T) {
	t.Parallel()
	// Arrange: explicit buffered execution retains provider metadata rather than pretending native support.
	fixture := &bufferedMetadataFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
		response: prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "fixture"}})}
	fixture.response.FinishReason = "stop"
	fixture.response.Usage = prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}}
	stream := NewClient(
		fixture,
	).ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamBuffered)
	require.Zero(t, fixture.calls)
	require.Zero(t, fixture.translations)
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	status := stream.Status()
	for _, err := range stream.Events() {
		require.ErrorIs(t, err, prompty.ErrStreamConsumed)
	}
	// Assert: no implicit dispatch at creation or repeat at second consumption.
	require.Equal(t, prompty.StreamBuffered, status.Mode)
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, "stop", status.Result.FinishReason)
	require.Equal(t, prompty.OutcomeCompleted, status.Result.Outcome)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, status.Result.Usage.Known)
	require.Equal(t, 1, terminals)
	require.Equal(t, 1, fixture.calls)
	require.Equal(t, 1, fixture.translations)
}
