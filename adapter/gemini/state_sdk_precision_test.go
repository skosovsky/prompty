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

func TestSignedNumbersAreExactOnSDKWireOrRejectedBeforeDispatch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, args, wireArgs string
		supported            bool
	}{
		{"large exact integer", `{"big":9007199254740992}`, `{"big":9007199254740992}`, true},
		{"numeric spelling", `{"decimal":1.00,"exponent":1e2}`, `{"decimal":1.00,"exponent":1e2}`, true},
		{"nested caller keys", `{"Name":"caller","nested":[{"name":0.1,"exact":9007199254740994}]}`,
			`{"Name":"caller","nested":[{"name":0.1,"exact":9007199254740994}]}`, true},
		{"large preserved integer", `{"big":9007199254740993}`, `{"big":9007199254740993}`, true},
		{"preserved decimal", `{"nested":[{"decimal":0.10000000000000001}]}`, `{"nested":[{"decimal":0.10000000000000001}]}`, true},
		{"overflow", `{"huge":1e1000}`, "", false},
	} {
		for _, mode := range []string{"sync", "buffered"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				t.Parallel()
				checkSignedSDKNumber(t, tc.args, tc.wireArgs, tc.supported, mode)
			})
		}
	}
}

func checkSignedSDKNumber(t *testing.T, args, wireArgs string, supported bool, mode string) {
	t.Helper()
	// Arrange: canonical args and required envelope agree exactly before SDK encoding.
	var calls atomic.Int64
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failure", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`,
		)
	}))
	t.Cleanup(server.Close)
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	identity := prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"}
	profile := prompty.CapabilityProfile{
		Identity:   identity,
		Provenance: "local SDK transport fixture, not live evidence",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState:       prompty.CapabilitySupported,
			prompty.CapabilityToolNameCorrelation: prompty.CapabilitySupported,
		},
	}
	fixture := &sdkPrecisionTranslationFixture{Adapter: New(
		WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile),
	)}
	client := adapter.NewClient(fixture)
	payload := `{"functionCall":{"name":"lookup","args":` + args + `},"thoughtSignature":"c2lnbmF0dXJl"}`
	canonicalArgs, err := json.Marshal(geminiJSONValue([]byte(args)))
	require.NoError(t, err)
	message := prompty.ChatMessage{Role: prompty.RoleAssistant,
		Content: []prompty.ContentPart{prompty.ToolCallPart{Name: "lookup", Args: string(canonicalArgs)}},
		ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
			Format: prompty.ProviderStateFormat, Scope: identity, Codec: geminiPartCodec,
			Required: true, Payload: []byte(payload),
		}}}}
	exec := prompty.SimplePrompt("fixture").AddMessage(message)
	before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	// Act: use the real SDK/local HTTP, not just marshaling a translated DTO.
	if mode == "sync" {
		_, err = client.Execute(context.Background(), exec)
	} else {
		stream := client.ExecuteStream(context.Background(), exec, prompty.StreamBuffered)
		for range stream.Events() {
		}
		err = stream.Status().Err
	}
	// Assert: preserve exact values/signature or reject with no actual HTTP calls.
	if !supported {
		require.ErrorIs(t, err, prompty.ErrProviderState)
		require.Zero(t, calls.Load())
		require.Zero(t, fixture.translations.Load())
		require.NotContains(t, err.Error(), "signature")
		require.NotContains(t, err.Error(), args)
	} else {
		require.NoError(t, err)
		require.Equal(t, int64(1), calls.Load())
		require.Equal(t, int64(1), fixture.translations.Load())
		var request struct {
			Contents []struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"contents"`
		}
		require.NoError(t, json.Unmarshal(<-requests, &request))
		require.Len(t, request.Contents, 2)
		require.Len(t, request.Contents[1].Parts, 1)
		require.True(t, exactGeminiSDKJSONEqual([]byte(payload), request.Contents[1].Parts[0]))
		actual := geminiJSONValue(request.Contents[1].Parts[0]).(map[string]any)
		require.Equal(t, "c2lnbmF0dXJl", actual["thoughtSignature"])
		call := actual["functionCall"].(map[string]any)
		require.Equal(t, "lookup", call["name"])
		require.Equal(t, geminiJSONValue([]byte(wireArgs)), call["args"])
	}
	after, wireErr := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, wireErr)
	require.Equal(t, before, after)
}

type sdkPrecisionTranslationFixture struct {
	*Adapter

	translations atomic.Int64
}

func (a *sdkPrecisionTranslationFixture) Translate(exec *prompty.PromptExecution) (*Request, error) {
	a.translations.Add(1)
	return a.Adapter.Translate(exec)
}
