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

func TestSDKOrdinarySignedContinuationRoundTrip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, args string
	}{
		{"ordinary numbers and refs", `{"message_ref":1,"patient_ref":"P1","service_ref":"S1","appointment_ref":"A1","zero":0,"fraction":0.25,"nested":[{"n":2}]}`},
		{"empty args", `{}`},
		{"nested caller data", `{"Name":"caller","nested":[{"decimal":0.1}]}`},
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			t.Run(tc.name+"/"+string(mode), func(t *testing.T) {
				t.Parallel()
				checkActualSignedIngress(t, tc.args, mode)
			})
		}
	}
}

func checkActualSignedIngress(t *testing.T, args string, mode prompty.StreamMode) {
	t.Helper()
	// Arrange: genuine SDK response with ordinary values and a fixture signature.
	var calls atomic.Int64
	requests := make(chan []byte, 2)
	signed := `{"functionCall":{"id":"call-a","name":"lookup","args":` + args + `},"thoughtSignature":"AAH/"}`
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
		reply := `{"candidates":[{"content":{"role":"model","parts":[` + part + `]},"finishReason":"STOP"}]}`
		if mode == prompty.StreamNative {
			reply = "data: " + reply + "\n\n"
		}
		_, _ = io.WriteString(w, reply)
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
			prompty.CapabilityNativeStream:        prompty.CapabilitySupported,
			prompty.CapabilityToolNameCorrelation: prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	// Act: parse → both wires → checked continuation, not a constructed SDK DTO.
	response, err := checkedIngressInvoke(context.Background(), client, prompty.SimplePrompt("fixture"), mode)
	require.NoError(t, err)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(
		t,
		geminiJSONValue([]byte(sdkCanonicalArgs(signed))),
		geminiJSONValue(response.ProviderState[0].Envelope.Payload),
	)
	part := response.Content[0].(prompty.ToolCallPart)
	require.Equal(t, geminiJSONValue([]byte(sdkCanonicalArgs(args))), geminiJSONValue([]byte(part.Args)))
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
	// Assert: SDK state, signature association, order and args survive persistence.
	// This proves transfer only, not cryptographic verification or live acceptance.
	require.NoError(t, err)
	require.Equal(t, int64(2), calls.Load())
	<-requests // First request contains no signed continuation.
	var request struct {
		Contents []struct {
			Parts []json.RawMessage `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	expectedState := sdkCanonicalArgs(signed)
	if args == "{}" {
		expectedState = `{"functionCall":{"id":"call-a","name":"lookup"},"thoughtSignature":"AAH/"}`
	}
	require.Equal(t, geminiJSONValue([]byte(expectedState)), geminiJSONValue(request.Contents[1].Parts[0]))
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

func TestRawSignedFiniteFloatDTOIsSDKRepresentation(t *testing.T) {
	t.Parallel()
	// Arrange: finite SDK values are valid even without original wire evidence.
	raw := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{
		Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "lookup", Args: map[string]any{"big": float64(1)}},
				ThoughtSignature: []byte("signature")},
		},
	}, FinishReason: genai.FinishReasonStop}}}
	// Act.
	response, err := New().ParseResponse(raw)
	// Assert: canonical state describes the SDK value.
	require.NoError(t, err)
	require.Len(t, response.ProviderState, 1)
	require.Equal(t, `{"big":1}`, response.Content[0].(prompty.ToolCallPart).Args)
}

func TestSDKSignedNumericLimitationsCharacterization(t *testing.T) {
	t.Parallel()
	for _, args := range []string{`{"big":9007199254740993}`, `{"decimal":1.234567890123456789}`, `{"nested":[{"negative":-9007199254740993,"fraction":0.10000000000000001,"exponent":1.234567890123456789e2}]}`} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamNative} {
			t.Run(args+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange/Act/Assert: SDK-limited signed representation persists and replays;
				// this differs from incompatible caller/legacy exact state tested separately.
				checkActualSignedIngress(t, args, mode)
			})
		}
	}
}
