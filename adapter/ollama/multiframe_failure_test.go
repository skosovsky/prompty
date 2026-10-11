package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestLaterMalformedHTTPFrameDisablesContinuation(t *testing.T) {
	t.Parallel()
	// Arrange: a valid frame precedes an invalid call with no known parts of its own.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(
			w,
			`{"message":{"role":"assistant","thinking":"known reasoning","content":"known text","tool_calls":[{"id":"valid","function":{"name":"lookup","arguments":{}}}]},"done":false}`+"\n"+`{"message":{"role":"assistant","tool_calls":[{"function":{"name":"bad","arguments":[]}}]},"done":true,"done_reason":"stop"}`+"\n",
		)
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport, err := NewHTTPChatClient(endpoint, server.Client(), 4096)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local fixture",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(transport), WithProfiles(server.URL, profile)))
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	status := stream.Status()
	// Assert: the known valid call is retained, but the incomplete exchange cannot be replayed.
	require.ErrorIs(t, status.Err, adapter.ErrMalformedArgs)
	require.NotNil(t, status.Result)
	require.Len(t, status.Result.Content, 3)
	require.True(t, status.Result.ContinuationUnavailable)
	require.Empty(t, status.Result.Outcome)
	message, err := status.Result.AssistantMessage()
	require.NoError(t, err)
	require.True(t, message.ContinuationUnavailable)
	_, err = client.Execute(t.Context(), prompty.SimplePrompt("next").AddMessage(message))
	require.Error(t, err)
}
