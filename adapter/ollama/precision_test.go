package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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

func TestSDKUnverifiedFloatsFailClosed(t *testing.T) {
	t.Parallel()
	// Arrange: genuine SDK response loses precision before the adapter receives it.
	var raw api.ChatResponse
	require.NoError(
		t,
		json.Unmarshal(
			[]byte(
				`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"id":9007199254740993}}}]},"done":true,"done_reason":"stop"}`,
			),
			&raw,
		),
	)
	value, exists := raw.Message.ToolCalls[0].Function.Arguments.Get("id")
	require.True(t, exists)
	numeric, valid := value.(float64)
	require.True(t, valid)
	require.Equal(t, math.Float64bits(float64(9007199254740992)), math.Float64bits(numeric))
	// Act/Assert: neither sync nor chunk parsing advertises rounded args as exact.
	_, err := New().ParseResponse(&raw)
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
	_, err = New().ParseStreamChunk(&raw)
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
}

func TestSDKPrecisionFailurePreventsToolHandler(t *testing.T) {
	t.Parallel()
	// Arrange: the real SDK converts the received integer into an unverified float.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprintln(
			w,
			`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"id":9007199254740993}}}]},"done":true,"done_reason":"stop"}`,
		)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "SDK precision rejection fixture",
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
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
	require.Equal(t, int32(1), calls.Load())
	require.Zero(t, invoked)
}

func TestSDKPrecisionFailureRetainsKnownUsageWithoutContent(t *testing.T) {
	t.Parallel()
	// Arrange: real SDK numeric failure and known usage in the same frame.
	client, _ := nativeFixture(
		t,
		[]string{
			`{"message":{"tool_calls":[{"function":{"name":"lookup","arguments":{"id":9007199254740993}}}]},"prompt_eval_count":7,"done":true,"done_reason":"stop"}`,
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
