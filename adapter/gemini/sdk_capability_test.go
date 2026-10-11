package gemini

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func capabilityFixture(t *testing.T, args string, mode prompty.StreamMode) prompty.Invoker {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"a","name":"lookup","args":` + args + `}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7}}`
		if mode == prompty.StreamNative {
			payload = "data: " + payload + "\n\n"
		}
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(server.Close)
	sdk, err := genai.NewClient(
		t.Context(),
		&genai.ClientConfig{
			APIKey:      "fixture",
			Backend:     genai.BackendGeminiAPI,
			HTTPClient:  server.Client(),
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
		},
	)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local capability contract fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityTools:        prompty.CapabilitySupported,
			prompty.CapabilityNativeStream: prompty.CapabilitySupported,
		},
	}
	return adapter.NewClient(
		New(WithNativeJSONSchema(), WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)),
	)
}

func capabilityInvoke(
	t *testing.T,
	client prompty.Invoker,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) (*prompty.Response, error) {
	t.Helper()
	if mode == "" {
		return client.Execute(t.Context(), exec)
	}
	stream := client.ExecuteStream(t.Context(), exec, mode)
	for range stream.Events() {
	}
	return stream.Status().Result, stream.Status().Err
}

func TestSDKOrdinaryNumbersAndOriginalSchema(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, tc := range []struct {
			name, args string
			invalid    bool
		}{
			{"ordinary", `{"message_ref":1,"patient_ref":"P1","zero":0,"fraction":0.25,"nested":[{"n":2}]}`, false},
			{"required", `{"patient_ref":"P1"}`, true},
			{"type", `{"message_ref":"wrong","patient_ref":"P1"}`, true},
			{"range", `{"message_ref":-1,"patient_ref":"P1"}`, true},
			{"additional", `{"message_ref":1,"patient_ref":"P1","extra":true}`, true},
		} {
			t.Run(tc.name+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange: actual public SDK decoding; the original schema validates available values.
				client := capabilityFixture(t, tc.args, mode)
				exec := prompty.SimplePrompt("fixture")
				exec.Tools = []prompty.ToolDefinition{
					{
						Name: "lookup",
						Parameters: prompty.JSONDocument(
							`{"type":"object","properties":{"message_ref":{"type":"integer","minimum":0},"patient_ref":{"type":"string"},"zero":{"type":"integer"},"fraction":{"type":"number"},"nested":{"type":"array","items":{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"],"additionalProperties":false}}},"required":["message_ref","patient_ref"],"additionalProperties":false}`,
						),
					},
				}
				// Act.
				result, err := capabilityInvoke(t, client, exec, mode)
				// Assert: validation is independent of the accepted SDK numeric limitation.
				if tc.invalid {
					require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
					return
				}
				require.NoError(t, err)
				require.Equal(t, prompty.OutcomeToolCalls, result.Outcome)
				require.JSONEq(t, tc.args, result.Content[0].(prompty.ToolCallPart).Args)
				require.Equal(t, 7, result.Usage.PromptTokens)
			})
		}
	}
}
func TestSDKNumericLimitationsCharacterization(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, input, canonical string }{
		{"large integer", `{"n":9007199254740993}`, `{"n":9007199254740992}`},
		{"long decimal", `{"n":1.234567890123456789}`, `{"n":1.2345678901234567}`},
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamNative} {
			t.Run(tc.name+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange: accepted dependency limit, separate from ordinary and domain validation.
				client := capabilityFixture(t, tc.input, mode)
				// Act.
				result, err := capabilityInvoke(t, client, prompty.SimplePrompt("fixture"), mode)
				// Assert: canonical JSON is the available SDK value, never restored wire precision.
				require.NoError(t, err)
				require.Equal(t, tc.canonical, result.Content[0].(prompty.ToolCallPart).Args)
				require.NotEqual(t, tc.input, result.Content[0].(prompty.ToolCallPart).Args)
			})
		}
	}
}
func TestInvalidSDKNumericGraphs(t *testing.T) {
	t.Parallel()
	for _, value := range []any{math.NaN(), math.Inf(1), math.Inf(-1), make(chan int)} {
		// Arrange: supplemental DTOs cover values JSON wire cannot express.
		raw := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{
								FunctionCall: &genai.FunctionCall{
									Name: "lookup",
									Args: map[string]any{"nested": []any{value}},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
		}
		// Act.
		_, err := New().ParseResponse(raw)
		// Assert: marshal rejection remains classified, without leaking the value.
		require.ErrorIs(t, err, adapter.ErrMalformedArgs)
	}
}

func sdkCanonicalArgs(args string) string {
	var value map[string]any
	if json.Unmarshal([]byte(args), &value) != nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func TestSDKDecodedNumericValuesCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: public SDK DTO decoder, complementary to actual HTTP sync/native fixtures.
	var raw genai.GenerateContentResponse
	// Act.
	require.NoError(
		t,
		json.Unmarshal(
			[]byte(
				`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"big":9007199254740993,"decimal":1.234567890123456789}}}]}}]}`,
			),
			&raw,
		),
	)
	values := raw.Candidates[0].Content.Parts[0].FunctionCall.Args
	// Assert: these are the SDK values before adapter serialization; precision cannot return.
	bigValue, bigOK := values["big"].(float64)
	decimalValue, decimalOK := values["decimal"].(float64)
	require.True(t, bigOK)
	require.True(t, decimalOK)
	require.Equal(t, math.Float64bits(float64(9007199254740992)), math.Float64bits(bigValue))
	require.Equal(t, math.Float64bits(1.2345678901234567), math.Float64bits(decimalValue))
}
