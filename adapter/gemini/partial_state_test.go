package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestSignedPartialActualVertexSDKWireContinuation(t *testing.T) {
	t.Parallel()
	for _, numeric := range []bool{false, true} {
		t.Run(map[bool]string{false: "string", true: "ordinary numeric"}[numeric], func(t *testing.T) {
			t.Parallel()
			checkSignedPartialVertexSDKWireContinuation(t, numeric)
		})
	}
}

func checkSignedPartialVertexSDKWireContinuation(t *testing.T, numeric bool) {
	t.Helper()
	// Arrange: unsigned start, signed progress and unsigned completion stay distinct.
	parts := []string{
		`{"functionCall":{"id":"a","name":"lookup","willContinue":true}}`,
		`{"functionCall":{"id":"b","name":"lookup","willContinue":true}}`,
		`{"functionCall":{"id":"a","partialArgs":[{"jsonPath":"$.n","stringValue":"one"}],"willContinue":true},"thoughtSignature":"c2ln"}`,
		`{"functionCall":{"id":"b","partialArgs":[{"jsonPath":"$.n","stringValue":"two"}]}}`,
		`{"functionCall":{"id":"a","partialArgs":[{"jsonPath":"$.s","stringValue":"ok"}]}}`,
	}
	expectedArgs := `{"n":"one","s":"ok"}`
	if numeric {
		parts[2] = `{"functionCall":{"id":"a","partialArgs":[{"jsonPath":"$.n","stringValue":"one"},{"jsonPath":"$.zero","numberValue":0},{"jsonPath":"$.fraction","numberValue":0.25}],"willContinue":true},"thoughtSignature":"AAH/"}`
		expectedArgs = `{"n":"one","s":"ok","zero":0,"fraction":0.25}`
	}
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- body
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range parts {
				_, _ = io.WriteString(
					w,
					"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":["+part+"]}}]}\n\n",
				)
			}
			_, _ = io.WriteString(w, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"usageMetadata\":{\"totalTokenCount\":7}}\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`,
		)
	}))
	defer server.Close()
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendVertexAI,
		HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta1"}})
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local signed partial Vertex SDK fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
			prompty.CapabilityProviderState:    prompty.CapabilitySupported,
			prompty.CapabilityTools:            prompty.CapabilitySupported,
			CapabilityIncrementalToolArguments: prompty.CapabilitySupported,
		},
	}
	provider := New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile))
	client := adapter.NewClient(provider)
	// Act: source updates bind exact provisional then complete arguments.
	stream := client.ExecuteStream(t.Context(), partialExecution(), prompty.StreamNative)
	updates := 0
	for frame, streamErr := range stream.Events() {
		require.NoError(t, streamErr)
		updates += len(frame.StateUpdates)
	}
	status := stream.Status()
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, 2, updates)
	require.Len(t, status.Result.ProviderState, 1)
	require.Equal(t, 7, status.Result.Usage.TotalTokens)
	require.JSONEq(t, expectedArgs, status.Result.Content[0].(prompty.ToolCallPart).Args)
	wire, err := prompty.MarshalResponse(status.Result, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(wire)
	require.NoError(t, err)
	exec := prompty.SimplePrompt("continue").AddMessage(prompty.ChatMessage{Role: prompty.RoleAssistant,
		Content: restored.Content, ProviderState: restored.ProviderState, Annotations: restored.Annotations})
	executionWire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	exec, err = prompty.UnmarshalExecution(executionWire)
	require.NoError(t, err)
	response, err := client.Execute(t.Context(), exec)
	// Assert: the actual SDK request replays all five source parts in order.
	require.NoError(t, err)
	require.Equal(t, "done", response.Content[0].(prompty.TextPart).Text)
	<-requests
	assertVertexSDKSourceParts(t, <-requests, parts)
	for _, annotation := range restored.Annotations {
		if annotation.Kind == geminiSourcePartAnnotation {
			require.NotContains(t, string(annotation.Value), "thoughtSignature")
		}
	}
	// Incompatible caller/legacy exact state must not be rounded to bypass admission.
	if numeric {
		for _, exact := range []string{"9007199254740993", "0.10000000000000001"} {
			legacy := exec.Clone()
			message := &legacy.Messages[len(legacy.Messages)-1]
			message.ProviderState[0].Envelope.Payload = []byte(
				strings.ReplaceAll(string(message.ProviderState[0].Envelope.Payload), "0.25", exact),
			)
			call := message.Content[0].(prompty.ToolCallPart)
			call.Args = strings.ReplaceAll(call.Args, "0.25", exact)
			message.Content[0] = call
			// Act/Assert: classified migration guidance without payload leakage, before SDK IO.
			_, stateErr := client.Execute(t.Context(), legacy)
			require.ErrorIs(t, stateErr, prompty.ErrProviderState)
			require.ErrorContains(t, stateErr, "saved representation is incompatible with SDK encoding")
			require.ErrorContains(t, stateErr, "obtain fresh provider state")
			require.NotContains(t, stateErr.Error(), exact)
			require.NotContains(t, stateErr.Error(), "AAH/")
			require.Empty(t, requests)
		}
	}
	// Missing or duplicated source provenance cannot be repaired by canonical order.
	for _, duplicate := range []bool{false, true} {
		invalid := exec.Clone()
		message := &invalid.Messages[len(invalid.Messages)-1]
		if duplicate {
			message.Annotations = append(message.Annotations, message.Annotations[len(message.Annotations)-1])
		} else {
			message.Annotations = nil
		}
		_, err = client.Execute(t.Context(), invalid)
		require.ErrorIs(t, err, prompty.ErrProviderState)
		require.Empty(t, requests)
	}
	// Edited canonical data must fail before another transport request.
	exec.Messages[len(exec.Messages)-1].Content[0] = prompty.ToolCallPart{
		ID:   "a",
		Name: "lookup",
		Args: `{"n":"two","s":"ok"}`,
	}
	_, err = client.Execute(t.Context(), exec)
	require.ErrorIs(t, err, prompty.ErrProviderState)
	require.Empty(t, requests)
}

func assertVertexSDKSourceParts(t *testing.T, body []byte, expected []string) {
	t.Helper()
	var request struct {
		Contents []struct {
			Role  string            `json:"role"`
			Parts []json.RawMessage `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	var actual []json.RawMessage
	for _, content := range request.Contents {
		if content.Role == "model" {
			actual = content.Parts
		}
	}
	require.Len(t, actual, 5)
	for index := range expected {
		require.Equal(t, geminiJSONValue([]byte(expected[index])), geminiJSONValue(actual[index]))
	}
}

func TestSignedPartialCancelRetainsLatestBoundSequence(t *testing.T) {
	t.Parallel()
	// Arrange: the only observed signed fragment is still provisional.
	client, calls := geminiNativeFixture(
		t,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","willContinue":true},"thoughtSignature":"c2ln"}]}}]}`,
		`{"candidates":[{"finishReason":"STOP"}]}`,
	)
	// Act: consumer stops before completion.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if len(frame.Content) == 0 {
			continue
		}
		require.Len(t, frame.StateUpdates, 1)
		break
	}
	status := stream.Status()
	// Assert: cancellation cannot make an unfinished signed sequence continuable.
	require.Equal(t, prompty.StreamCanceled, status.State)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, status.Result.ProviderState, 1)
	_, canonical, complete, err := decodePartialState(status.Result.ProviderState[0])
	require.NoError(t, err)
	require.False(t, complete)
	require.Equal(t, status.Result.Content[0], canonical)
	message := prompty.ChatMessage{
		Role:          prompty.RoleAssistant,
		Content:       status.Result.Content,
		ProviderState: status.Result.ProviderState,
	}
	require.ErrorIs(t, validateGeminiPayloads([]prompty.ChatMessage{message}), prompty.ErrProviderState)
}

func TestSignedPartialDoesNotSilentlyReorderInterleavedParts(t *testing.T) {
	t.Parallel()
	// Arrange: another call appears inside the signed source sequence.
	client, calls := geminiNativeFixture(
		t,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"a","name":"lookup","willContinue":true},"thoughtSignature":"c2ln"}]}}]}`,
		functionEvent(`{"id":"b","name":"other","args":{}}`),
		functionEvent(`{"id":"a","partialArgs":[{"jsonPath":"$.n","stringValue":"one"}]}`),
		`{"candidates":[{"finishReason":"STOP"}]}`,
	)
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	status := stream.Status()
	// Assert: canonical order stays call-based, while outgoing source order is global.
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.NoError(t, status.Err)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, status.Result.Content, 2)
	require.JSONEq(t, `{"n":"one"}`, status.Result.Content[0].(prompty.ToolCallPart).Args)
	require.Len(t, status.Result.ProviderState, 1)
	message := prompty.ChatMessage{Role: prompty.RoleAssistant, Content: status.Result.Content,
		ProviderState: status.Result.ProviderState, Annotations: status.Result.Annotations}
	parts, err := orderedPartialMessage(message)
	require.NoError(t, err)
	require.Len(t, parts, 3)
	require.Equal(t, "a", parts[0].FunctionCall.ID)
	require.Equal(t, "b", parts[1].FunctionCall.ID)
	require.Equal(t, "a", parts[2].FunctionCall.ID)
	require.Equal(t, []byte("sig"), parts[0].ThoughtSignature)
}
