package openai

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func forNativeFixtureModels(t *testing.T, run func(*testing.T, string)) {
	t.Helper()
	for _, model := range []string{openai.ChatModelGPT4o, "fixture"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			run(t, model)
		})
	}
}

// This inventory is local SDK conformance, not a catalogue of live model support.
//
//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestNativeFixtureProfilesTerminalConformance(t *testing.T) {
	t.Parallel()
	for _, model := range []string{openai.ChatModelGPT4o, "fixture"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			for _, scenario := range []string{"completed", "read-failure", "early-eof"} {
				t.Run(scenario, func(t *testing.T) {
					t.Parallel()
					// Arrange: identical native contracts, distinct effective model bindings.
					reader, wantState, wantErr := nativeConformanceReader(scenario)
					client, transport := cleanupFixtureClientModel(reader, model)
					stream := client.ExecuteStream(
						context.Background(),
						prompty.SimplePrompt("fixture"),
						prompty.StreamNative,
					)
					require.Zero(t, transport.calls.Load())
					messageID := stream.Status().Identity.MessageID
					// Act.
					terminals := 0
					for event, err := range stream.Events() {
						if event.Kind == prompty.StreamTerminal {
							terminals++
							require.Equal(t, messageID, event.Identity.MessageID)
							if wantErr != nil {
								require.ErrorIs(t, err, wantErr)
							} else {
								require.NoError(t, err)
							}
						}
					}
					// Assert: terminal state and accepted prefix survive the real SDK boundary.
					status := stream.Status()
					require.Equal(t, model, status.Result.Diagnostics.Profile.Model)
					require.Equal(t, "https://fixture.invalid", status.Result.Diagnostics.Profile.Endpoint)
					require.Equal(t, wantState, status.State)
					require.Equal(t, 1, terminals)
					require.Equal(t, int32(1), transport.calls.Load())
					require.Positive(t, transport.body.closed.Load())
					text, err := status.Result.StrictText()
					require.NoError(t, err)
					require.Equal(t, "onetwo", text)
					require.Equal(t, []string{"content:0", "content:0"}, status.Identity.PartIDs)
					if scenario == "completed" {
						require.Equal(t, 9, status.Result.Usage.TotalTokens)
					}
				})
			}
		})
	}
}

func nativeConformanceReader(scenario string) (io.Reader, prompty.StreamState, error) {
	if scenario == "read-failure" {
		return io.MultiReader(
			strings.NewReader(cleanupFixtureSSE),
			cleanupErrorReader{},
		), prompty.StreamFailed, adapter.ErrInvalidResponse
	}
	if scenario == "early-eof" {
		return strings.NewReader(cleanupFixtureSSE), prompty.StreamFailed, prompty.ErrStreamProtocol
	}
	return strings.NewReader(
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n" + cleanupFixtureSSE +
			"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: {\"choices\":[],\"usage\":{\"total_tokens\":9}}\n\ndata: [DONE]\n\n",
	), prompty.StreamCompleted, nil
}

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestNativeFixtureProfilesStopConformance(t *testing.T) {
	t.Parallel()
	for _, model := range []string{openai.ChatModelGPT4o, "fixture"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			for _, stop := range []string{"break", "cancel", "close"} {
				t.Run(stop, func(t *testing.T) {
					t.Parallel()
					// Arrange.
					client, transport := cleanupFixtureClientModel(strings.NewReader(cleanupFixtureSSE), model)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					stream := client.ExecuteStream(ctx, prompty.SimplePrompt("fixture"), prompty.StreamNative)
					require.Zero(t, transport.calls.Load())
					// Act: stop immediately after the first accepted content delta.
					for event, err := range stream.Events() {
						if len(event.Content) == 0 {
							continue
						}
						require.NoError(t, err)
						if stop == "break" {
							break
						}
						if stop == "cancel" {
							cancel()
						} else {
							require.NoError(t, stream.Close())
						}
					}
					// Assert.
					status := stream.Status()
					require.Equal(t, model, status.Result.Diagnostics.Profile.Model)
					require.Equal(t, prompty.StreamCanceled, status.State)
					require.ErrorIs(t, status.Err, context.Canceled)
					require.Len(t, status.Result.Content, 1)
					require.Equal(t, []string{"content:0"}, status.Identity.PartIDs)
					require.Equal(t, int32(1), transport.calls.Load())
					require.Positive(t, transport.body.closed.Load())
				})
			}
		})
	}
}
