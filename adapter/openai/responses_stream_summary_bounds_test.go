package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestResponsesCreateStreamRejectsUndeclaredSummaryIndices(t *testing.T) {
	t.Parallel()
	for _, declared := range []bool{false, true} {
		for _, kind := range []string{"text.delta", "text.done", "part.done"} {
			indices := []string{"-1", "0", "1", "9223372036854775807", "missing"}
			for _, index := range indices {
				if declared && index == "0" {
					continue
				}
				t.Run(fmt.Sprintf("declared=%t/%s/index=%s", declared, kind, index), func(t *testing.T) {
					t.Parallel()
					// Arrange: a reasoning item exists, but the adversarial index has no declared summary part.
					prefix := 3
					if declared {
						prefix = 5
					}
					events := mixedResponseSSEFixture(true)[:prefix]
					indexField := `,"summary_index":` + index
					if index == "missing" {
						indexField = ""
					}
					payload := `,"delta":"bad"`
					switch kind {
					case "text.done":
						payload = `,"text":"bad"`
					case "part.done":
						payload = `,"part":{"type":"summary_text","text":"bad"}`
					}
					events = append(
						events,
						`{"type":"response.reasoning_summary_`+kind+`","item_id":"rs_fixture","output_index":0`+indexField+payload+`}`,
					)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						writeResponseSSEFixture(w, events)
					}))
					defer server.Close()
					transport := &sseBodyTransport{base: server.Client().Transport, firstClosed: make(chan struct{})}
					server.Client().Transport = transport
					manager := localToolsFixtureAdapter(server)
					manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
					stream := manager.CreateStream(
						t.Context(),
						prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("fixture"), Store: new(false)},
					)
					// Act: malformed native data must follow the ordinary stream failure path, never panic.
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
					// Assert: a single terminal retains the accepted resource, usage and public prefix.
					require.ErrorIs(t, observed, prompty.ErrStreamProtocol)
					require.Equal(t, 1, terminals)
					require.Equal(t, prompty.StreamFailed, stream.Status().State)
					result := stream.RemoteResult()
					require.Equal(t, "resp_fixture", result.Handle.ProviderID)
					require.Equal(t, prompty.RemoteResponseRunning, result.Handle.Status)
					expected := []prompty.ContentPart{prompty.ReasoningPart{Text: ""}}
					if declared {
						expected = append(
							expected,
							prompty.ReasoningPart{Text: ""},
							prompty.ReasoningPart{Text: "plan"},
						)
					}
					require.Equal(t, expected, result.Response.Content)
					require.Equal(t, 10, result.Response.Usage.PromptTokens)
					require.True(t, result.Response.ContinuationUnavailable)
					require.EqualValues(t, 1, transport.calls.Load())
					select {
					case <-transport.firstClosed:
					default:
						t.Fatal("SDK body was not closed after protocol rejection")
					}
				})
			}
		}
	}
}
