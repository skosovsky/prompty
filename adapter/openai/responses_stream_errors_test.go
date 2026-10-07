package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestResponsesCreateStreamRejectsBeforeIO(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"background", "unsupported stream", "unknown profile"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeResponseSSEFixture(w, mixedResponseSSEFixture(true))
			}))
			defer server.Close()
			manager := responseFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			input := prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("fixture"), Store: new(false)}
			want := adapter.ErrUnsupportedCombination
			switch scenario {
			case "background":
				input.Background = true
			case "unsupported stream":
				manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilityUnsupported
				want = adapter.ErrUnsupportedCapability
			case "unknown profile":
				manager.base.profiles = nil
				want = adapter.ErrUnknownProfile
			}
			// Act.
			stream := manager.CreateStream(t.Context(), input)
			var observed error
			for _, err := range stream.Events() {
				if err != nil {
					observed = err
				}
			}
			// Assert.
			require.ErrorIs(t, observed, want)
			require.Zero(t, calls.Load())
			require.Nil(t, stream.RemoteResult().Handle)
		})
	}
}

func TestResponsesCreateStreamRejectsContradictorySSE(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"summary done", "text done", "citation", "late summary", "invalid original arguments", "unknown event"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: malicious provider events cannot replace accepted fragments/state.
			events := mixedResponseSSEFixture(true)
			want := prompty.ErrStreamProtocol
			for index, event := range events {
				switch scenario {
				case "summary done":
					if strings.Contains(event, `"type":"response.reasoning_summary_text.done"`) {
						events[index] = strings.ReplaceAll(event, `"plan"`, `"changed"`)
					}
				case "text done":
					if strings.Contains(event, `"type":"response.output_text.done"`) {
						events[index] = strings.ReplaceAll(event, `"answer"`, `"changed"`)
					}
				case "citation":
					if strings.Contains(event, `"type":"response.output_text.annotation.added"`) {
						events[index] = strings.ReplaceAll(event, "https://example.org", "https://changed.example")
					}
				case "late summary":
					if strings.Contains(event, `"type":"response.completed"`) {
						events[index] = strings.ReplaceAll(event, `"plan"`, `"changed"`)
					}
				case "invalid original arguments":
					events[index] = strings.ReplaceAll(event, "9007199254740993", "1")
					want = prompty.ErrSchemaMismatch
				case "unknown event":
					if strings.Contains(event, `"type":"response.output_text.done"`) {
						events[index] = strings.ReplaceAll(
							event,
							"response.output_text.done",
							"response.unknown_required_event",
						)
					}
				}
			}
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponseSSEFixture(w, events) }),
			)
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			// Act.
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: withResponseHostedTools(exec,
					[]prompty.HostedToolRequest{{Kind: HostedWebSearch}}), Store: new(false)},
			)
			var observed error
			for _, err := range stream.Events() {
				if err != nil {
					observed = err
				}
			}
			// Assert: the resource and observed prefix remain available after failure.
			require.ErrorIs(t, observed, want)
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.Equal(t, "resp_fixture", stream.RemoteResult().Handle.ProviderID)
			require.NotEmpty(t, stream.RemoteResult().Response.Content)
			require.True(t, stream.RemoteResult().Response.ContinuationUnavailable)
		})
	}
}
