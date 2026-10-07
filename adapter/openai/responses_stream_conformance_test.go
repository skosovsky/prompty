package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesCreateStreamTerminalOnlyStateWireAndExplicitReplay(t *testing.T) {
	t.Parallel()
	for _, terminalOnly := range []bool{false, true} {
		name := "item_done_state"
		if terminalOnly {
			name = "terminal_only_state"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange: the terminal payload carries state withheld from item.done.
			events := mixedResponseSSEFixture(true)
			if terminalOnly {
				events[6] = strings.ReplaceAll(events[6], `,"encrypted_content":"opaque"`, "")
			}
			requests := make(chan []byte, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests <- body
				if strings.Contains(string(body), `"stream":true`) {
					writeResponseSSEFixture(w, events)
				} else {
					writeResponseFixture(w, "completed", "next")
				}
			}))
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			execution := prompty.SimplePrompt("fixture")
			execution.Tools = responseToolDefinitions()
			execution.Requirements.Capabilities = []string{prompty.CapabilityProviderState}
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{
					Execution: withResponseHostedTools(execution, []prompty.HostedToolRequest{{Kind: HostedWebSearch}}),
					Store:     new(false),
				},
			)
			// Act: finish the native stream, round-trip state, then explicitly request continuation.
			for _, err := range stream.Events() {
				require.NoError(t, err)
			}
			response := stream.RemoteResult().Response
			wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			encoded, err := json.Marshal(wire)
			require.NoError(t, err)
			restored, err := prompty.DecodeResponseJSON(encoded)
			require.NoError(t, err)
			history, err := restored.AssistantMessage()
			require.NoError(t, err)
			next := prompty.NewExecution(
				[]prompty.ChatMessage{
					history,
					{
						Role: prompty.RoleTool,
						Content: []prompty.ContentPart{
							prompty.ToolResultPart{
								ToolCallID: "call_fixture",
								Content:    []prompty.ContentPart{prompty.TextPart{Text: "found"}},
							},
						},
					},
					{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "continue"}}},
				},
			)
			_, err = manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: next, Store: new(false)})
			// Assert: no provisional missing-state flag survives; every original native item is replayable.
			require.NoError(t, err)
			require.False(t, restored.ContinuationUnavailable)
			require.Len(t, restored.ProviderState, 2)
			require.Equal(t, response.Content, restored.Content)
			<-requests
			var request struct {
				Input []json.RawMessage `json:"input"`
			}
			require.NoError(t, json.Unmarshal(<-requests, &request))
			require.Len(t, request.Input, 6)
			original := []string{streamReasoningItem, streamMessageItem, responsesFunctionFixture, streamHostedItem}
			for index, item := range original {
				require.JSONEq(t, item, string(request.Input[index]))
			}
			require.Contains(t, string(request.Input[4]), `"call_id":"call_fixture"`)
			require.Contains(t, string(request.Input[5]), "continue")
		})
	}
}

func TestResponsesCreateStreamParentCancellationReleasesBody(t *testing.T) {
	t.Parallel()
	// Arrange: cancellation occurs after a real response ID, usage and encrypted state are observed.
	waitCtx, stopWaiting := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopWaiting()
	parent, cancel := context.WithCancel(waitCtx)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResponseSSEFixture(w, mixedResponseSSEFixture(true)[:7])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	transport := &sseBodyTransport{base: server.Client().Transport, firstClosed: make(chan struct{})}
	server.Client().Transport = transport
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
	execution := prompty.SimplePrompt("fixture")
	execution.Tools = responseToolDefinitions()
	stream := manager.CreateStream(
		parent,
		prompty.RemoteResponseRequest{
			Execution: withResponseHostedTools(execution, []prompty.HostedToolRequest{{Kind: HostedWebSearch}}),
			Store:     new(false),
		},
	)
	ready, done := make(chan struct{}), make(chan struct{})
	go consumeUntilSSEFinalization(stream, ready, done)
	awaitSSESignal(waitCtx, t, ready)
	// Act.
	cancel()
	awaitSSESignal(waitCtx, t, done)
	awaitSSESignal(waitCtx, t, transport.firstClosed)
	// Assert: local cancellation does not invent a remote cancellation operation/status.
	require.NoError(t, waitCtx.Err())
	require.Equal(t, prompty.StreamCanceled, stream.Status().State)
	result := stream.RemoteResult()
	require.Equal(t, "resp_fixture", result.Handle.ProviderID)
	require.Equal(t, prompty.RemoteResponseRunning, result.Handle.Status)
	require.Len(t, result.Response.ProviderState, 1)
	require.Equal(t, 10, result.Response.Usage.PromptTokens)
	require.True(t, result.Response.ContinuationUnavailable)
	require.EqualValues(t, 1, transport.calls.Load())
}

func TestResponsesCreateStreamMalformedUsageRetainsAcceptedPrefix(t *testing.T) {
	t.Parallel()
	for _, counter := range []string{`-1`, `"10"`, `1.5`, `null`, `9223372036854775808`} {
		t.Run(counter, func(t *testing.T) {
			t.Parallel()
			// Arrange: usage is observed after a verified reasoning part, before completion.
			events := mixedResponseSSEFixture(true)[:7]
			events = append(
				events,
				`{"type":"response.in_progress","response":{"id":"resp_fixture","model":"fixture","status":"in_progress","usage":{"input_tokens":`+counter+`,"output_tokens":3}}}`,
			)
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponseSSEFixture(w, events) }),
			)
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
			)
			// Act.
			var observed error
			for _, err := range stream.Events() {
				if err != nil {
					observed = err
				}
			}
			// Assert: invalid counters are never reported as a known zero; accepted state is retained.
			require.ErrorIs(t, observed, prompty.ErrUsage)
			result := stream.RemoteResult()
			require.Equal(t, "resp_fixture", result.Handle.ProviderID)
			require.Len(t, result.Response.ProviderState, 1)
			require.Equal(t, []prompty.ContentPart{prompty.ReasoningPart{Text: "plan"}}, result.Response.Content)
			require.True(t, result.Response.ContinuationUnavailable)
		})
	}
}

func TestResponsesCreateStreamRejectsMalformedLifecycleAfterAcceptedState(t *testing.T) {
	t.Parallel()
	for name, event := range map[string]string{
		"missing sequence":         `{"type":"response.in_progress","response":{"id":"resp_fixture","model":"fixture","status":"in_progress"}}`,
		"repeated sequence":        `{"sequence_number":6,"type":"response.in_progress","response":{"id":"resp_fixture","model":"fixture","status":"in_progress"}}`,
		"changed resource":         `{"sequence_number":7,"type":"response.in_progress","response":{"id":"changed","model":"fixture","status":"in_progress"}}`,
		"duplicate field":          `{"sequence_number":7,"type":"response.in_progress","response":{"id":"resp_fixture","id":"changed","model":"fixture","status":"in_progress"}}`,
		"terminal status mismatch": `{"sequence_number":7,"type":"response.completed","response":{"id":"resp_fixture","model":"fixture","status":"incomplete"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange: malformed native lifecycle data follows a valid state-bearing prefix.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeResponseSSEFixture(w, mixedResponseSSEFixture(true)[:7])
				_, _ = io.WriteString(w, "data: "+event+"\n\n")
			}))
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
			)
			// Act.
			var observed error
			terminals := 0
			for frame, err := range stream.Events() {
				if frame != nil && frame.Kind == prompty.StreamTerminal {
					terminals++
				}
				if err != nil {
					observed = err
				}
			}
			// Assert: rejection never overwrites the already accepted resource or private state.
			if name == "changed resource" {
				require.ErrorIs(t, observed, prompty.ErrRemoteResponseUnknown)
			} else {
				require.ErrorIs(t, observed, prompty.ErrStreamProtocol)
			}
			require.Equal(t, 1, terminals)
			result := stream.RemoteResult()
			require.Equal(t, "resp_fixture", result.Handle.ProviderID)
			require.Equal(t, prompty.RemoteResponseRunning, result.Handle.Status)
			require.Len(t, result.Response.ProviderState, 1)
			require.True(t, result.Response.ContinuationUnavailable)
		})
	}
}

func TestResponsesCreateStreamTerminalMalformedUsage(t *testing.T) {
	t.Parallel()
	// Arrange: malformed nested counters arrive only at terminal after verified output.
	events := outputResponseSSEFixture("answer", "completed", false)
	last := len(events) - 1
	events[last] = strings.TrimSuffix(
		events[last],
		"}}",
	) + `,"usage":{"input_tokens":2,"output_tokens_details":{"reasoning_tokens":"3"}}}}`
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponseSSEFixture(w, events) }),
	)
	defer server.Close()
	manager := responseFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
	stream := manager.CreateStream(
		t.Context(),
		prompty.RemoteResponseRequest{Execution: prompty.SimplePrompt("fixture"), Store: new(false)},
	)
	// Act.
	var observed error
	for _, err := range stream.Events() {
		if err != nil {
			observed = err
		}
	}
	// Assert: ordinary text and valid observed counters survive the usage failure.
	require.ErrorIs(t, observed, prompty.ErrUsage)
	result := stream.RemoteResult()
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "answer"}}, result.Response.Content)
	require.Equal(t, 2, result.Response.Usage.PromptTokens)
	require.NotContains(t, result.Response.Usage.Known, prompty.UsageReasoning)
	require.Equal(t, prompty.RemoteResponseCompleted, result.Handle.Status)
	require.Equal(t, prompty.StreamFailed, stream.Status().State)
}
