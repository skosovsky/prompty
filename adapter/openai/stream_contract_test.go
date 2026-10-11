package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestNativeInterleavedToolDeltasPreserveIdentity(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeInterleavedToolConformance)
}

func nativeInterleavedToolConformance(t *testing.T, model string) {
	t.Helper()
	// Arrange: two calls to the same function, identity present only initially.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"lookup","arguments":"{\"x\":"}},{"index":1,"id":"b","type":"function","function":{"name":"lookup","arguments":"{\"x\":"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}},{"index":0,"function":{"arguments":"9007199254740993}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
			"[DONE]",
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider: "openai",
			Endpoint: server.URL,
			Model:    model,
		},
		Provenance:   "local SSE conformance fixture",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithModel(model), WithClient(&sdk), WithProfiles(server.URL, profile)))
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	status := stream.Status()
	// Assert.
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.OutcomeToolCalls, status.Result.Outcome)
	require.Equal(t, 5, status.Result.Usage.TotalTokens)
	require.Len(t, status.Result.Content, 2)
	first := status.Result.Content[0].(prompty.ToolCallPart)
	second := status.Result.Content[1].(prompty.ToolCallPart)
	require.Equal(t, "a", first.ID)
	require.Equal(t, "b", second.ID)
	require.Equal(t, "lookup", second.Name)
	require.Equal(t, `{"x":9007199254740993}`, first.Args)
	require.JSONEq(t, `{"x":2}`, second.Args)
	require.NotEmpty(t, status.Identity.MessageID)
	require.Equal(t, []string{"tool:0", "tool:1"}, status.Identity.PartIDs)
}

func TestUsagePresenceDistinguishesMissingAndKnownZero(t *testing.T) {
	t.Parallel()
	// Arrange.
	var usage openai.CompletionUsage
	require.NoError(t, usage.UnmarshalJSON([]byte(`{"total_tokens":0}`)))
	// Act.
	got, err := usageFromOpenAI(usage)
	absent, absentErr := usageFromOpenAI(openai.CompletionUsage{})
	// Assert.
	require.NoError(t, err)
	require.NoError(t, absentErr)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, got.Known)
	require.Empty(t, absent.Known)
}

func TestParallelNativeStreamsFromOneClientKeepCorrelationLocal(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeParallelToolConformance)
}

func nativeParallelToolConformance(t *testing.T, model string) {
	t.Helper()
	// Arrange: both requests use index zero and the same provider ID, but different functions/args.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(
			w,
			"data: "+`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"same","function":{"name":"lookup-%d","arguments":"{\"x\":"}}]}}]}`+"\n\n",
			n,
		)
		_, _ = fmt.Fprintf(
			w,
			"data: "+`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"%d}"}}]}}]}`+"\n\n",
			n,
		)
		_, _ = fmt.Fprint(w, "data: "+`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	var profile prompty.CapabilityProfile
	profile.Identity = prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: model}
	profile.Provenance = "parallel actual SDK SSE fixture"
	profile.Capabilities = map[string]prompty.CapabilitySupport{
		prompty.CapabilityNativeStream: prompty.CapabilitySupported,
	}
	client := adapter.NewClient(New(WithModel(model), WithClient(&sdk), WithProfiles(server.URL, profile)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	accepted := make(chan struct{}, 2)
	release := make(chan struct{})
	statuses := make([]prompty.StreamStatus, 2)
	var workers sync.WaitGroup
	// Act: block both consumers after initial identity, then allow both continuation deltas.
	for i := range statuses {
		workers.Go(func() {
			stream := client.ExecuteStream(ctx, prompty.SimplePrompt("fixture"), prompty.StreamNative)
			initial := false
			for event := range stream.Events() {
				if initial || event == nil || len(event.Content) == 0 {
					continue
				}
				initial = true
				accepted <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			statuses[i] = stream.Status()
		})
	}
	for range statuses {
		select {
		case <-accepted:
		case <-ctx.Done():
		}
	}
	close(release)
	workers.Wait()
	// Assert: neither request inherits the other's name/args or mutates its retained result.
	require.NoError(t, ctx.Err())
	require.Equal(t, int32(2), requests.Load())
	seen := make(map[string]string)
	require.NotEqual(t, statuses[0].Identity.MessageID, statuses[1].Identity.MessageID)
	for _, status := range statuses {
		require.NoError(t, status.Err)
		require.Equal(t, prompty.StreamCompleted, status.State)
		require.Equal(t, []string{"tool:0"}, status.Identity.PartIDs)
		require.Len(t, status.Result.Content, 1)
		call := status.Result.Content[0].(prompty.ToolCallPart)
		require.Equal(t, "same", call.ID)
		seen[call.Name] = call.Args
	}
	require.Equal(t, map[string]string{"lookup-1": `{"x":1}`, "lookup-2": `{"x":2}`}, seen)
}
