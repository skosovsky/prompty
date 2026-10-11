package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestActualSDKIngressPreservesSignedNumbersBeforeContinuation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, args  string
		canContinue bool
	}{
		{"rounded integer", `{"big":9007199254740993}`, true},
		{"rounded decimal", `{"decimal":0.10000000000000001}`, true},
		{"exact integer", `{"big":9007199254740992}`, true},
		{"empty args", `{}`, true},
		{"nested caller data", `{"Name":"caller","nested":[{"decimal":0.1}]}`, true},
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered} {
			t.Run(tc.name+"/"+string(mode), func(t *testing.T) {
				t.Parallel()
				checkActualSignedIngress(t, tc.args, tc.canContinue, mode)
			})
		}
	}
}

func checkActualSignedIngress(t *testing.T, args string, canContinue bool, mode prompty.StreamMode) {
	t.Helper()
	// Arrange: the actual SDK would round the first response before Part encoding.
	var calls atomic.Int64
	requests := make(chan []byte, 2)
	signed := `{"functionCall":{"name":"lookup","args":` + args + `},"thoughtSignature":"c2lnbmF0dXJl"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read error", http.StatusBadRequest)
			return
		}
		requests <- body
		count := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		part := signed
		if count > 1 {
			part = `{"text":"ok"}`
		}
		_, _ = io.WriteString(
			w,
			`{"candidates":[{"content":{"role":"model","parts":[`+part+`]},"finishReason":"STOP"}]}`,
		)
	}))
	t.Cleanup(server.Close)
	httpClient := server.Client()
	originalTransport := httpClient.Transport
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	identity := prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"}
	profile := prompty.CapabilityProfile{
		Identity:   identity,
		Provenance: "local SDK ingress fixture, not live evidence",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState:       prompty.CapabilitySupported,
			prompty.CapabilityToolNameCorrelation: prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	// Act: parse → both wires → checked continuation, not a constructed SDK DTO.
	response, err := checkedIngressInvoke(context.Background(), client, prompty.SimplePrompt("fixture"), mode)
	require.NoError(t, err)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, geminiJSONValue([]byte(signed)), geminiJSONValue(response.ProviderState[0].Envelope.Payload))
	part := response.Content[0].(prompty.ToolCallPart)
	require.Equal(t, geminiJSONValue([]byte(args)), geminiJSONValue([]byte(part.Args)))
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(wire)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	exec := prompty.SimplePrompt("fixture").AddMessage(message)
	executionWire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	rebound, err := prompty.UnmarshalExecution(executionWire)
	require.NoError(t, err)
	_, err = checkedIngressInvoke(context.Background(), client, rebound, mode)
	// Assert: exact original state is persisted; no damaged signature is replayed.
	if canContinue {
		require.NoError(t, err)
		require.Equal(t, int64(2), calls.Load())
		<-requests // First request contains no signed continuation.
		var request struct {
			Contents []struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"contents"`
		}
		require.NoError(t, json.Unmarshal(<-requests, &request))
		require.Equal(t, geminiJSONValue([]byte(signed)), geminiJSONValue(request.Contents[1].Parts[0]))
	} else {
		require.ErrorIs(t, err, prompty.ErrProviderState)
		require.Equal(t, int64(1), calls.Load())
	}
	require.Same(t, originalTransport, httpClient.Transport)
	require.Same(t, httpClient, sdk.ClientConfig().HTTPClient)
}

func checkedIngressInvoke(
	ctx context.Context,
	client prompty.Invoker,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) (*prompty.Response, error) {
	if mode == "" {
		return client.Execute(ctx, exec)
	}
	stream := client.ExecuteStream(ctx, exec, mode)
	for range stream.Events() {
	}
	return stream.Status().Result, stream.Status().Err
}

func TestRawSignedFloatDTOWithoutOriginalSourceFailsClosed(t *testing.T) {
	t.Parallel()
	// Arrange: a float-normalized DTO is not proof of the provider's signed number.
	raw := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{
		Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "lookup", Args: map[string]any{"big": float64(1)}},
				ThoughtSignature: []byte("signature")},
		},
	}, FinishReason: genai.FinishReasonStop}}}
	// Act.
	response, err := New().ParseResponse(raw)
	// Assert: no falsely continuable original envelope escapes.
	require.Nil(t, response)
	require.ErrorIs(t, err, prompty.ErrProviderState)
}
