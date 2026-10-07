package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestPausedThinkingWireAndExplicitSDKContinuation(t *testing.T) {
	t.Parallel()
	// Arrange: a completed HTTP operation pauses model work without losing signed state.
	requests := make(chan []byte, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(request.Body)
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.ReplaceAll(orderedThinkingFixture, `"end_turn"`, `"pause_turn"`))
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	identity := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL,
		prompty.CapabilityProfile{Identity: identity, Provenance: "actual SDK fixture, not live conformance",
			Capabilities: map[string]prompty.CapabilitySupport{
				prompty.CapabilityProviderState: prompty.CapabilitySupported,
			}})))
	exec := prompty.SimplePrompt("fixture")
	// Act: parse/wire restore and then one explicitly invoked continuation.
	response, err := client.Execute(t.Context(), exec)
	require.NoError(t, err)
	<-requests
	require.EqualValues(t, 1, calls.Load())
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	document, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(document)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	exec.Messages = append(exec.Messages, message)
	_, err = client.Execute(t.Context(), exec)
	// Assert: pause is preserved, signed blocks replay unchanged, and no hidden resume occurred.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomePaused, restored.Outcome)
	require.False(t, restored.ContinuationUnavailable)
	require.Len(t, restored.ProviderState, 2)
	require.EqualValues(t, 2, calls.Load())
	var request struct {
		Messages []struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.Equal(t, "assistant", request.Messages[1].Role)
	require.Contains(t, string(request.Messages[1].Content[1]), `"signature":"opaque-signature=="`)
	require.Contains(t, string(request.Messages[1].Content[3]), `"data":"opaque-redacted=="`)
}

func TestNativePausedThinkingRemainsResumable(t *testing.T) {
	t.Parallel()
	// Arrange: signed native state followed by an explicit pause marker.
	client, calls := nativeFixture(t, []string{
		messageStartFixture,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"pause_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	})
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	status := stream.Status()
	// Assert: transport completed; model did not complete its turn or authorize a handler.
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.OutcomePaused, status.Result.Outcome)
	require.False(t, status.Result.ContinuationUnavailable)
	require.Len(t, status.Result.ProviderState, 1)
	_, err := status.Result.AssistantMessage()
	require.NoError(t, err)
}
