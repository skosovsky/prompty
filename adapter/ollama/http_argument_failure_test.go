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

func TestSDKDuplicateKeysCharacterization(t *testing.T) {
	t.Parallel()
	for _, args := range []string{`{"n":9007199254740993,"n":2}`, `{"nested":{"n":1,"\u006e":2}}`} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			t.Run(args+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange: SDK normalizes duplicate keys before adapter mapping.
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
				// Assert: available values survive; original duplicates are irrecoverable.
				require.NoError(t, err)
				require.NotNil(t, response)
				require.Len(t, response.Content, 3)
				require.Equal(t, "known reasoning", response.Content[0].(prompty.ReasoningPart).Text)
				require.Equal(t, "known text", response.Content[1].(prompty.TextPart).Text)
				expected := `{"n":2}`
				if args == `{"nested":{"n":1,"\u006e":2}}` {
					expected = `{"nested":{"n":2}}`
				}
				require.Equal(t, expected, response.Content[2].(prompty.ToolCallPart).Args)
				require.Equal(t, prompty.OutcomeToolCalls, response.Outcome)
			})
		}
	}
}
