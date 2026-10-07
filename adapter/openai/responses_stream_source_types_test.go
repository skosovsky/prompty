package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesCreateStreamRejectsCoercedSourceTypes(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"numeric delta", "null delta", "missing delta", "string sequence", "fraction sequence", "string output index", "fraction output index", "string content index", "fraction content index", "string summary index", "fraction summary index"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: coercible source values have internally matching done/terminal output.
			events := outputResponseSSEFixture("42", "completed", false)
			target := 3
			rawSequence := false
			switch scenario {
			case "numeric delta":
				events[target] = strings.ReplaceAll(events[target], `"delta":"42"`, `"delta":42`)
			case "null delta", "missing delta":
				events = outputResponseSSEFixture("", "completed", false)
				replacement := `"delta":null`
				if scenario == "missing delta" {
					replacement = `"optional_future":"value"`
				}
				events[target] = strings.ReplaceAll(events[target], `"delta":""`, replacement)
			case "string output index", "fraction output index":
				replacement := `"output_index":"0"`
				if scenario == "fraction output index" {
					replacement = `"output_index":0.5`
				}
				events[target] = strings.ReplaceAll(events[target], `"output_index":0`, replacement)
			case "string content index", "fraction content index":
				replacement := `"content_index":"0"`
				if scenario == "fraction content index" {
					replacement = `"content_index":0.5`
				}
				events[target] = strings.ReplaceAll(events[target], `"content_index":0`, replacement)
			case "string summary index", "fraction summary index":
				events = mixedResponseSSEFixture(true)
				target = 4
				replacement := `"summary_index":"0"`
				if scenario == "fraction summary index" {
					replacement = `"summary_index":0.5`
				}
				events[target] = strings.ReplaceAll(events[target], `"summary_index":0`, replacement)
			case "string sequence", "fraction sequence":
				rawSequence = true
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if !rawSequence {
					writeResponseSSEFixture(w, events)
					return
				}
				writeResponseSSEFixture(w, events[:target])
				sequence := `"3"`
				if scenario == "fraction sequence" {
					sequence = `3.5`
				}
				_, _ = io.WriteString(
					w,
					`data: {"sequence_number":`+sequence+`,`+strings.TrimPrefix(events[target], "{")+"\n\n",
				)
				writeResponseSSEFixtureFrom(w, events[target+1:], target+1)
			}))
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			exec = withResponseHostedTools(exec, []prompty.HostedToolRequest{{Kind: HostedWebSearch}})
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
			)
			// Act.
			var observed error
			terminals := 0
			require.NotPanics(t, func() {
				for frame, err := range stream.Events() {
					if frame != nil && frame.Kind == prompty.StreamTerminal {
						terminals++
					}
					if err != nil {
						observed = err
					}
				}
			})
			// Assert: native source types cannot be silently normalized into successful output.
			require.ErrorIs(t, observed, prompty.ErrStreamProtocol)
			require.Equal(t, 1, terminals)
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.Equal(t, "resp_fixture", stream.RemoteResult().Handle.ProviderID)
			require.True(t, stream.RemoteResult().Response.ContinuationUnavailable)
		})
	}
}
