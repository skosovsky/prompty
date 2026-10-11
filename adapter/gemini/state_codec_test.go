package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestGeminiSignedCodecRejectsLossyShapesBeforeDispatch(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, payload string }{
		{"duplicate signature", `{"text":"signed","thoughtSignature":"bG9zdA==","thoughtSignature":"c2lnbmF0dXJl"}`},
		{"duplicate text", `{"text":"private-lost","text":"signed","thoughtSignature":"c2lnbmF0dXJl"}`},
		{"escaped duplicate", `{"text":"private-lost","\u0074ext":"signed","thoughtSignature":"c2lnbmF0dXJl"}`},
		{"case text alias", `{"Text":"signed","thoughtSignature":"c2lnbmF0dXJl"}`},
		{"case signature alias", `{"text":"signed","ThoughtSignature":"c2lnbmF0dXJl"}`},
		{"nested case alias", `{"functionCall":{"Name":"lookup"},"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"nested duplicate", `{"functionCall":{"name":"private-lost","name":"lookup"},"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"argument duplicate", `{"functionCall":{"name":"lookup","args":{"x":1,"x":2}},"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"array nested duplicate", `{"functionCall":{"name":"lookup","args":{"items":[{"x":1,"x":2}]}},"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"unknown field", `{"text":"signed","private-extension":null,"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"unknown nested field", `{"functionCall":{"name":"lookup","private-extension":null},"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"discarded null", `{"text":"signed","fileData":null,"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"discarded zero", `{"text":"signed","thought":false,"thoughtSignature":"c2lnbmF0dXJl"}`},
		{"altered signature encoding", `{"text":"signed","thoughtSignature":"c2ln\nbmF0dXJl"}`},
		{"missing signature", `{"text":"signed"}`},
		{"null signature", `{"text":"signed","thoughtSignature":null}`},
		{"extra document", `{"text":"signed","thoughtSignature":"c2lnbmF0dXJl"}{}`},
		{"array root", `[{"text":"signed","thoughtSignature":"c2lnbmF0dXJl"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: a corrupt persisted envelope must not become a normalized SDK request.
			profile := geminiStateFixtureProfile()
			fixture := &schemaDispatchFixture{
				Adapter: New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile)),
			}
			message := prompty.ChatMessage{Role: prompty.RoleAssistant,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "signed"}},
				ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
					Format: prompty.ProviderStateFormat, Scope: profile.Identity, Codec: geminiPartCodec,
					Payload: []byte(test.payload), Required: true,
				}}}}
			// Act.
			_, err := adapter.NewClient(fixture).
				Execute(context.Background(), prompty.SimplePrompt("fixture").AddMessage(message))
			// Assert: typed safe failure, zero provider calls and no mutation/repair.
			require.ErrorIs(t, err, prompty.ErrProviderState)
			require.Zero(t, fixture.calls)
			require.Equal(t, test.payload, string(message.ProviderState[0].Envelope.Payload))
			require.NotContains(t, err.Error(), "private")
		})
	}
}

func TestGeminiSignedCodecPreservesCallerArgumentKeysAndExactNumbers(t *testing.T) {
	t.Parallel()
	// Arrange: argument keys are caller-owned, unlike case-sensitive SDK struct keys.
	payload := []byte(` {
		"thoughtSignature": "c2lnbmF0dXJl",
		"functionCall": {"name":"lookup", "args": {
			"Name":"caller", "name":"distinct", "nested":[{"Unicode":"☕"}],
			"big":9007199254740993, "huge":1e1000, "decimal":1.00, "nil":null
		}}
	} `)
	var state prompty.PartState
	state.Envelope.Payload = payload
	// Act.
	part, err := decodeGeminiState(state)
	require.NoError(t, err)
	canonical, err := canonicalGeminiPart(part)
	require.NoError(t, err)
	encoded, err := json.Marshal(part)
	require.NoError(t, err)
	// Assert: whitespace/key order may differ, all signed values/argument data remain exact.
	require.Equal(t, "caller", part.FunctionCall.Args["Name"])
	require.Equal(t, "distinct", part.FunctionCall.Args["name"])
	require.Equal(t, json.Number("9007199254740993"), part.FunctionCall.Args["big"])
	require.Equal(t, json.Number("1e1000"), part.FunctionCall.Args["huge"])
	require.Equal(t, json.Number("1.00"), part.FunctionCall.Args["decimal"])
	require.Equal(t, geminiJSONValue(payload), geminiJSONValue(encoded))
	require.IsType(t, prompty.ToolCallPart{}, canonical)
	require.Equal(t, []byte("signature"), part.ThoughtSignature)
	var restored genai.Part
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&restored))
	require.Equal(t, "lookup", restored.FunctionCall.Name)
}
