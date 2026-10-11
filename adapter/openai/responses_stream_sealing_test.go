package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestResponsesCreateStreamRejectsEventsAfterPartDone(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"output_text", "function_call_arguments", "reasoning_summary_text"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			// Arrange: an empty extra delta does not change the eventual text/JSON,
			// but violates the completed part's lifecycle and must fail immediately.
			events := mixedResponseSSEFixture(true)
			var late string
			for index, event := range events {
				if strings.Contains(event, `"type":"response.`+kind+`.delta"`) {
					switch kind {
					case "output_text":
						late = `{"type":"response.output_text.delta","output_index":1,"content_index":0,"item_id":"msg_fixture","delta":""}`
					case "function_call_arguments":
						late = `{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"fc_fixture","delta":""}`
					case "reasoning_summary_text":
						late = `{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"item_id":"rs_fixture","delta":""}`
					}
				}
				if strings.Contains(event, `"type":"response.`+kind+`.done"`) {
					prefix := append([]string(nil), events[:index+1]...)
					events = append(append(prefix, late), events[index+1:]...)
					break
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
			// Assert: terminal response is not reached despite identical final content.
			require.ErrorIs(t, observed, prompty.ErrStreamProtocol)
			require.Equal(t, prompty.RemoteResponseRunning, stream.RemoteResult().Handle.Status)
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
		})
	}
}
