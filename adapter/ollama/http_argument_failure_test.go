package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ollama/ollama/api"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestHTTPArgumentFailureRetainsKnownPrefix(t *testing.T) {
	t.Parallel()
	for _, args := range []string{`{"n":9007199254740993,"n":2}`, `{"nested":{"n":1,"\u006e":2}}`} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			t.Run(args+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange: ordinary parts precede a rejected native call in the same HTTP frame.
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(
						w,
						`{"message":{"role":"assistant","thinking":"known reasoning","content":"known text","tool_calls":[{"function":{"name":"lookup","arguments":`+args+`}}]},"done":true,"done_reason":"stop"}`+"\n",
					)
				}))
				t.Cleanup(server.Close)
				endpoint, err := url.Parse(server.URL)
				require.NoError(t, err)
				transport := api.NewClient(endpoint, server.Client())
				profile := prompty.CapabilityProfile{
					Identity:   prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
					Provenance: "local HTTP fixture",
					Capabilities: map[string]prompty.CapabilitySupport{
						prompty.CapabilityNativeStream: prompty.CapabilitySupported,
					},
				}
				client := adapter.NewClient(
					New(WithModel("fixture"), WithClient(transport), WithProfiles(server.URL, profile)),
				)
				// Act.
				var response *prompty.Response
				if mode == "" {
					response, err = client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
				} else {
					stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), mode)
					for range stream.Events() {
					}
					response, err = stream.Status().Result, stream.Status().Err
				}
				// Assert: no valid tool call or successful finish is invented.
				require.ErrorIs(t, err, adapter.ErrMalformedArgs)
				require.NotNil(t, response)
				require.Len(t, response.Content, 2)
				require.Equal(t, "known reasoning", response.Content[0].(prompty.ReasoningPart).Text)
				require.Equal(t, "known text", response.Content[1].(prompty.TextPart).Text)
				require.Empty(t, response.Outcome)
				require.True(t, response.ContinuationUnavailable)
			})
		}
	}
}
