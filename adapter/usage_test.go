package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type invalidUsageFixture struct{ *checkedFixture }

type partialParseUsageFixture struct{ *checkedFixture }

func TestParserUsageErrorCaptureRetainsKnownTerminal(t *testing.T) {
	t.Parallel()
	for _, buffered := range []bool{false, true} {
		name := "sync"
		if buffered {
			name = "buffered"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			fixture := &partialParseUsageFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()}}
			var records []prompty.CaptureRecord
			ports := captureBindingFixture{records: &records}
			exec := prompty.SimplePrompt("fixture")
			exec.Capture = &prompty.CaptureBinding{Sink: ports, Projector: ports}
			exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic}
			client := NewClient(fixture)
			// Act.
			var response *prompty.Response
			if buffered {
				stream := client.ExecuteStream(context.Background(), exec, prompty.StreamBuffered)
				for range stream.Events() {
				}
				require.Equal(t, prompty.StreamFailed, stream.Status().State)
				response = stream.Status().Result
			} else {
				var err error
				response, err = client.Execute(context.Background(), exec)
				require.ErrorIs(t, err, prompty.ErrUsage)
			}
			// Assert: one failed terminal carries the known provider outcome and usage.
			terminals := 0
			for _, record := range records {
				if record.Stage != prompty.CaptureTerminal {
					continue
				}
				terminals++
				require.Equal(t, prompty.OutcomeCompleted, record.Projection.Metadata.Outcome)
				require.Equal(t, 1, record.Projection.Metadata.Usage.CompletionTokens)
				var terminal map[string]any
				require.NoError(t, json.Unmarshal(record.Projection.Payload, &terminal))
				require.Equal(t, "parse", terminal["error_kind"])
				require.NotNil(t, terminal["response"])
			}
			require.Equal(t, 1, terminals)
			require.True(t, response.Diagnostics.Capture.Complete)
			require.Equal(t, 1, fixture.calls)
		})
	}
}

func (*partialParseUsageFixture) ParseResponse(response *prompty.Response) (*prompty.Response, error) {
	response.Usage = prompty.Usage{CompletionTokens: 1, Known: []prompty.UsageCounter{prompty.UsageCompletion}}
	return response, prompty.ErrUsage
}

func TestParserUsageErrorRetainsKnownResponseWithoutReplay(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &partialParseUsageFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()}}
	exec := prompty.SimplePrompt("fixture")
	// Act.
	response, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrUsage)
	require.NotNil(t, response)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "ok"}}, response.Content)
	require.Equal(t, 1, response.Usage.CompletionTokens)
	require.Equal(t, fixtureDescriptor().Profiles[0].Identity, response.Diagnostics.Profile)
	require.Equal(t, 1, fixture.calls)
	require.Equal(t, 1, fixture.translations)
}

func TestBufferedParserUsageErrorRetainsKnownResponseWithoutReplay(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &partialParseUsageFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()}}
	stream := NewClient(
		fixture,
	).ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamBuffered)
	require.Zero(t, fixture.calls)
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, prompty.ErrUsage)
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.Equal(t, 1, terminals)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "ok"}}, status.Result.Content)
	require.Equal(t, 1, status.Result.Usage.CompletionTokens)
	require.Equal(t, fixtureDescriptor().Profiles[0].Identity, status.Result.Diagnostics.Profile)
	require.Equal(t, 1, fixture.calls)
	require.Equal(t, 1, fixture.translations)
}

func (*invalidUsageFixture) ParseResponse(response *prompty.Response) (*prompty.Response, error) {
	response.Usage = prompty.Usage{TotalTokens: -1, Known: []prompty.UsageCounter{prompty.UsageTotal}}
	return response, nil
}

func TestSyncInvalidUsagePreservesKnownResponseWithoutReplay(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &invalidUsageFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()}}
	exec := prompty.SimplePrompt("fixture")
	// Act.
	response, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert: metadata failure cannot claim not-executed or erase known content.
	require.ErrorIs(t, err, prompty.ErrUsage)
	require.NotNil(t, response)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "ok"}}, response.Content)
	require.Equal(t, fixtureDescriptor().Profiles[0].Identity, response.Diagnostics.Profile)
	require.Equal(t, 1, fixture.calls)
	require.Equal(t, 1, fixture.translations)
}
