package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestSDKSchemaFailurePreventsToolHandler(t *testing.T) {
	t.Parallel()
	// Arrange: SDK-decoded arguments violate the original tool schema.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprintln(
			w,
			`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"id":"wrong"}}}]},"done":true,"done_reason":"stop"}`,
		)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "SDK schema rejection fixture",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityTools: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(api.NewClient(endpoint, server.Client())),
		WithProfiles(server.URL, profile)))
	registry := prompty.NewTypedToolRegistry()
	invoked := 0
	tool, err := prompty.NewTypedTool("lookup", func(_ context.Context, _ struct {
		ID int64 `json:"id"`
	}) (string, error) {
		invoked++
		return "must not execute", nil
	})
	require.NoError(t, err)
	require.NoError(t, prompty.RegisterTool(registry, tool))
	execution := prompty.SimplePrompt("fixture")
	execution.Tools = []prompty.ToolDefinition{{Name: "lookup",
		Parameters: prompty.JSONDocument(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`),
	}}
	// Act.
	_, err = prompty.ExecuteWithTypedTools(context.Background(), client, execution, registry)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
	require.Equal(t, int32(1), calls.Load())
	require.Zero(t, invoked)
}

func TestSDKInvalidCallRetainsKnownUsageWithoutContent(t *testing.T) {
	t.Parallel()
	// Arrange: real SDK invalid call and known usage in the same frame.
	client, _ := nativeFixture(
		t,
		[]string{
			`{"message":{"tool_calls":[{"function":{"name":"","arguments":{"id":1}}}]},"prompt_eval_count":7,"done":true,"done_reason":"stop"}`,
		},
	)
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	// Assert: no executable content/success, but the known billed input is retained.
	result := stream.Status().Result
	require.ErrorIs(t, stream.Status().Err, adapter.ErrMalformedArgs)
	require.Empty(t, result.Content)
	require.Empty(t, result.Outcome)
	require.Equal(t, 7, result.Usage.PromptTokens)
	require.Contains(t, result.Usage.Known, prompty.UsagePrompt)
	require.True(t, result.ContinuationUnavailable)
}
