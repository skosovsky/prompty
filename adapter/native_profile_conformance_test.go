package adapter

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

var errNativeProfileFixture = errors.New("native profile fixture read failure")

type nativeProfileFixture struct {
	*checkedFixture

	scenario string
	released bool
}

func (f *nativeProfileFixture) StreamEvents(
	context.Context, *prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		f.calls++
		defer func() { f.released = true }()
		for _, text := range []string{"one", "two"} {
			var frame prompty.ResponseChunk
			frame.Kind = prompty.StreamDelta
			frame.Identity.PartIDs = []string{"text"}
			frame.Content = []prompty.ContentPart{prompty.TextPart{Text: text}}
			if !yield(&frame, nil) {
				return
			}
		}
		if f.scenario == "read-failure" {
			yield(nil, errNativeProfileFixture)
			return
		}
		if f.scenario == "early-eof" {
			return
		}
		if !yield(&prompty.ResponseChunk{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeCompleted}, nil) {
			return
		}
		yield(&prompty.ResponseChunk{Kind: prompty.StreamUsage,
			Usage: prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}}}, nil)
	}
}

func TestSyntheticNativeProfileLifecycleConformance(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"completed", "read-failure", "early-eof", "break", "cancel", "close"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: caller-owned synthetic knowledge, checked native port, no provider SDK.
			fixture := &nativeProfileFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
				scenario: scenario, released: false}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := NewClient(fixture).ExecuteStream(ctx, prompty.SimplePrompt("fixture"), prompty.StreamNative)
			require.Zero(t, fixture.calls)
			require.Zero(t, fixture.translations)
			// Act.
			terminals := consumeNativeProfileFixture(t, stream, scenario, cancel)
			// Assert: this fixture tests the common lifecycle, not live tool/model capabilities.
			status := stream.Status()
			require.Equal(t, "known", status.Result.Diagnostics.Profile.Model)
			require.Equal(t, 1, fixture.calls)
			require.Equal(t, 1, fixture.translations)
			require.True(t, fixture.released)
			if scenario == "break" {
				require.Zero(t, terminals, "consumer false cannot be followed by terminal yield")
			} else {
				require.Equal(t, 1, terminals)
			}
			switch scenario {
			case "completed":
				require.Equal(t, prompty.StreamCompleted, status.State)
				require.NoError(t, status.Err)
				require.Equal(t, 9, status.Result.Usage.TotalTokens)
			case "read-failure":
				require.Equal(t, prompty.StreamFailed, status.State)
				require.ErrorIs(t, status.Err, errNativeProfileFixture)
			case "early-eof":
				require.Equal(t, prompty.StreamFailed, status.State)
				require.ErrorIs(t, status.Err, prompty.ErrStreamProtocol)
			default:
				require.Equal(t, prompty.StreamCanceled, status.State)
				require.ErrorIs(t, status.Err, context.Canceled)
				require.Len(t, status.Result.Content, 1)
			}
			for _, id := range status.Identity.PartIDs {
				require.Equal(t, "text", id)
			}
			require.NotEmpty(t, status.Result.Content)
			require.Len(t, status.Identity.PartIDs, len(status.Result.Content))
		})
	}
}

func consumeNativeProfileFixture(t *testing.T, stream *prompty.Stream, scenario string, cancel context.CancelFunc) int {
	t.Helper()
	terminals := 0
	for event := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
		}
		if len(event.Content) == 0 {
			continue
		}
		switch scenario {
		case "cancel":
			cancel()
		case "close":
			require.NoError(t, stream.Close())
		}
		if scenario == "break" {
			break
		}
	}
	return terminals
}
