package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func nativeFixture(t *testing.T, lines []string, capabilities ...string) (prompty.Invoker, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, line := range lines {
			_, _ = fmt.Fprintln(w, line)
		}
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local SDK/NDJSON fixture, not model conformance",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	for _, capability := range capabilities {
		profile.Capabilities[capability] = prompty.CapabilitySupported
	}
	chat := api.NewClient(endpoint, server.Client())
	return adapter.NewClient(New(WithModel("fixture"), WithClient(chat),
		WithProfiles(server.URL, profile))), &calls
}

func TestNativeSnapshotsAndLateUsage(t *testing.T) {
	t.Parallel()
	// Arrange: tools are complete snapshots, not JSON argument deltas.
	client, calls := nativeFixture(t, []string{
		`{"message":{"role":"assistant","content":"early"},"done":false}`,
		`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"x":"one"}}},{"id":"b","function":{"name":"lookup","arguments":{"x":"two"}}}]},"done":false}`,
		`{"message":{"tool_calls":[{"id":"b","function":{"name":"lookup","arguments":{"x":"two"}}}]},"done":false}`,
		`{"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":2}`,
	})
	// Act.
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if frame.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	status := stream.Status()
	// Assert.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.OutcomeToolCalls, status.Result.Outcome)
	require.Equal(t, 5, status.Result.Usage.TotalTokens)
	require.Len(t, status.Result.Content, 3)
	require.Equal(t, "early", status.Result.Content[0].(prompty.TextPart).Text)
	require.JSONEq(t, `{"x":"one"}`, status.Result.Content[1].(prompty.ToolCallPart).Args)
	require.JSONEq(t, `{"x":"two"}`, status.Result.Content[2].(prompty.ToolCallPart).Args)
}

func TestNativeFaultsRetainPartial(t *testing.T) {
	t.Parallel()
	for name, final := range map[string]string{
		"EOF":                  "",
		"transport error":      `{"error":"fixture"}`,
		"malformed frame":      `{`,
		"conflicting snapshot": `{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"x":"two"}}}]},"done":true,"done_reason":"stop"}`,
		"after finish":         `{"done":true,"done_reason":"stop"}` + "\n" + `{"message":{"content":"late"},"done":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			client, calls := nativeFixture(t, []string{
				`{"message":{"content":"partial","tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"x":"one"}}}]},"done":false}`,
				final,
			})
			// Act.
			stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert.
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.NotEmpty(t, stream.Status().Result.Content)
			require.True(t, stream.Status().Result.ContinuationUnavailable)
			require.Equal(t, "partial", stream.Status().Result.Content[0].(prompty.TextPart).Text)
		})
	}
}

type nativeTransport func(*http.Request) (*http.Response, error)

func (f nativeTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type nativeBody struct {
	io.ReadCloser

	closed *atomic.Int32
}

func (b nativeBody) Close() error {
	b.closed.Add(1)
	return b.ReadCloser.Close()
}

func TestNativeBreakClosesBodyAndPreservesRequest(t *testing.T) {
	t.Parallel()
	// Arrange: observe the actual stream flag and response Close without networking.
	var calls, closed atomic.Int32
	transport := nativeTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		var body struct {
			Stream bool `json:"stream"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.True(t, body.Stream)
		payload := `{"message":{"content":"partial"},"done":false}` + "\n"
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{}, Request: req,
			Body: nativeBody{ReadCloser: io.NopCloser(strings.NewReader(payload)), closed: &closed},
		}, nil
	})
	endpoint, err := url.Parse("https://fixture.invalid")
	require.NoError(t, err)
	a := New(WithClient(api.NewClient(endpoint, &http.Client{Transport: transport})))
	streamOff := false
	request := &api.ChatRequest{Model: "fixture", Stream: &streamOff}
	// Act.
	frames := 0
	for frame, err := range a.StreamEvents(context.Background(), request) {
		require.NoError(t, err)
		require.NotEmpty(t, frame.Content)
		frames++
		break
	}
	// Assert: SDK callback stops immediately and closes the body, never replays.
	require.Equal(t, 1, frames)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, int32(1), closed.Load())
	require.Same(t, &streamOff, request.Stream)
	require.False(t, *request.Stream)
}

func TestNativeEarlyTextBeforeServerCompletion(t *testing.T) {
	t.Parallel()
	// Arrange: server will not finish until consumer cancellation closes the request.
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = fmt.Fprintln(w, `{"message":{"content":"early"},"done":false}`)
		w.(http.Flusher).Flush()
		<-req.Context().Done()
		close(closed)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "ollama", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "local blocked server",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(api.NewClient(endpoint, server.Client())),
		WithProfiles(server.URL, profile)))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Act: break on the first content frame, not on shared pre-dispatch start.
	stream := client.ExecuteStream(ctx, prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if len(frame.Content) > 0 {
			break
		}
	}
	// Assert.
	require.NoError(t, ctx.Err())
	require.Equal(t, prompty.StreamCanceled, stream.Status().State)
	require.Equal(t, "early", stream.Status().Result.Content[0].(prompty.TextPart).Text)
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("native transport did not close on break")
	}
}

func TestCompleteSnapshotAndNameResultMapping(t *testing.T) {
	t.Parallel()
	// Arrange: no provider ID/index is fabricated for a complete ID-less call.
	var raw api.ChatResponse
	payload := `{"message":{"tool_calls":[{"function":{"name":"lookup","arguments":{}}}]}}`
	require.NoError(t, json.Unmarshal([]byte(payload), &raw))
	a := New()
	// Act.
	parts, err := a.ParseStreamChunk(&raw)
	require.NoError(t, err)
	request, err := a.Translate(&prompty.PromptExecution{Messages: []prompty.ChatMessage{
		{Role: prompty.RoleTool, Content: []prompty.ContentPart{
			prompty.ToolResultPart{Name: "lookup", Content: []prompty.ContentPart{prompty.TextPart{Text: "result"}}},
		}},
	}})
	// Assert.
	require.NoError(t, err)
	call := parts[0].(prompty.ToolCallPart)
	require.Empty(t, call.ID)
	require.Empty(t, call.ArgsChunk)
	require.Equal(t, "{}", call.Args)
	require.Equal(t, "lookup", request.Messages[0].ToolName)
	require.Empty(t, request.Messages[0].ToolCallID)
}

func TestNativeSDKLargeIntegerCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: actual SDK HTTP decoding precedes the adapter callback.
	client, _ := nativeFixture(t, []string{
		`{"message":{"tool_calls":[{"id":"a","function":{"name":"lookup","arguments":{"id":9007199254740993}}}]},"done":false}`,
		`{"done":true,"done_reason":"stop"}`,
	})
	// Act.
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	// Assert: SDK-OLLAMA-NUM accepts the available rounded value, not exact wire.
	require.Equal(t, prompty.StreamCompleted, stream.Status().State)
	require.NoError(t, stream.Status().Err)
	require.Equal(t, `{"id":9007199254740992}`, stream.Status().Result.Content[0].(prompty.ToolCallPart).Args)
}
