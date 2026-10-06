package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type geminiPartShapeCase struct {
	name    string
	payload string
	want    error
}

func geminiInvalidPartShapes() []geminiPartShapeCase {
	return []geminiPartShapeCase{
		{"text and call", `{"text":"private","functionCall":{"name":"lookup","args":{}}}`, adapter.ErrInvalidResponse},
		{
			"text and inline",
			`{"text":"private","inlineData":{"mimeType":"image/png","data":"AQI="}}`,
			adapter.ErrInvalidResponse,
		},
		{
			"two media variants",
			`{"inlineData":{"data":"AQI="},"fileData":{"fileUri":"https://fixture.invalid"}}`,
			adapter.ErrInvalidResponse,
		},
		{"thought and call", `{"thought":true,"functionCall":{"name":"lookup","args":{}}}`, adapter.ErrInvalidResponse},
		{"text and code", `{"text":"private","executableCode":{"code":"private"}}`, adapter.ErrUnsupportedContentType},
		{
			"text and code result",
			`{"text":"private","codeExecutionResult":{"output":"private"}}`,
			adapter.ErrUnsupportedContentType,
		},
		{
			"text and function response",
			`{"text":"private","functionResponse":{"name":"lookup","response":{}}}`,
			adapter.ErrUnsupportedContentType,
		},
		{"media resolution", `{"text":"private","mediaResolution":{}}`, adapter.ErrUnsupportedContentType},
		{
			"video metadata",
			`{"fileData":{"fileUri":"https://fixture.invalid"},"videoMetadata":{}}`,
			adapter.ErrUnsupportedContentType,
		},
		{
			"inline display name",
			`{"inlineData":{"data":"AQI=","displayName":"private"}}`,
			adapter.ErrUnsupportedContentType,
		},
		{
			"file display name",
			`{"fileData":{"fileUri":"https://fixture.invalid","displayName":"private"}}`,
			adapter.ErrUnsupportedContentType,
		},
		{
			"partial arguments",
			`{"functionCall":{"name":"lookup","args":{},"partialArgs":[{"jsonPath":"$.x","stringValue":"private"}]}}`,
			adapter.ErrUnsupportedContentType,
		},
		{
			"continuing call",
			`{"functionCall":{"name":"lookup","args":{},"willContinue":true}}`,
			adapter.ErrUnsupportedContentType,
		},
		{
			"explicit noncontinuing call",
			`{"functionCall":{"name":"lookup","args":{},"willContinue":false}}`,
			adapter.ErrUnsupportedContentType,
		},
	}
}

func TestGeminiMixedAndUnsupportedPartsFailClosed(t *testing.T) {
	t.Parallel()
	for _, test := range geminiInvalidPartShapes() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, signed := range []bool{false, true} {
				// Arrange: SDK shapes must not be normalized by switch priority.
				var part genai.Part
				require.NoError(t, json.Unmarshal([]byte(test.payload), &part))
				if signed {
					part.ThoughtSignature = []byte("signature")
				}
				raw := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
					Content:      &genai.Content{Parts: []*genai.Part{{Text: "accepted prefix"}, &part}},
					FinishReason: genai.FinishReasonStop,
				}}}
				// Act.
				response, err := New().ParseResponse(raw)
				// Assert: an invalid sync response is not a successful shortened response.
				require.Nil(t, response)
				require.ErrorIs(t, err, test.want)
				require.NotContains(t, err.Error(), "private")
			}
		})
	}
}

func TestGeminiInvalidSignedPartsNeverDispatch(t *testing.T) {
	t.Parallel()
	for _, test := range geminiInvalidPartShapes() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: restore a signed part whose canonical projection would lose data.
			profile := geminiStateFixtureProfile()
			fixture := &schemaDispatchFixture{
				Adapter: New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile)),
			}
			var part genai.Part
			require.NoError(t, json.Unmarshal([]byte(test.payload), &part))
			part.ThoughtSignature = []byte("signature")
			payload, err := json.Marshal(&part)
			require.NoError(t, err)
			message := prompty.ChatMessage{Role: prompty.RoleAssistant,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "private"}},
				ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
					Format: prompty.ProviderStateFormat, Scope: profile.Identity, Codec: geminiPartCodec,
					Payload: payload, Required: true,
				}}}}
			// Act.
			_, executionErr := adapter.NewClient(fixture).
				Execute(context.Background(), prompty.SimplePrompt("fixture").AddMessage(message))
			// Assert: failure occurs before the SDK dispatch, with no fallback/replay.
			require.ErrorIs(t, executionErr, test.want)
			require.Zero(t, fixture.calls)
			require.NotContains(t, executionErr.Error(), "private")
		})
	}
}

func geminiStateFixtureProfile() prompty.CapabilityProfile {
	return prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider: "gemini",
			Endpoint: "https://fixture.invalid",
			Model:    "fixture",
		},
		Provenance: "local signed-part fidelity fixture, not live model support",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState: prompty.CapabilitySupported,
		},
	}
}

func TestGeminiSignedEnvelopeCannotBecomeOptional(t *testing.T) {
	t.Parallel()
	// Arrange.
	profile := geminiStateFixtureProfile()
	fixture := &schemaDispatchFixture{
		Adapter: New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile)),
	}
	response, err := fixture.ParseResponseForProfile(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content:      &genai.Content{Parts: []*genai.Part{{Text: "signed", ThoughtSignature: []byte("signature")}}},
		FinishReason: genai.FinishReasonStop,
	}}}, profile.Identity)
	require.NoError(t, err)
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	message.ProviderState[0].Envelope.Required = false
	// Act.
	_, executionErr := adapter.NewClient(fixture).
		Execute(context.Background(), prompty.SimplePrompt("fixture").AddMessage(message))
	// Assert.
	require.ErrorIs(t, executionErr, prompty.ErrProviderState)
	require.Zero(t, fixture.calls)
}

func TestGeminiOrderedMediaAndSignatureOnlyRoundTrip(t *testing.T) {
	t.Parallel()
	// Arrange: all supported variants remain separate, including a signature-only part.
	profile := geminiStateFixtureProfile()
	a := New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile))
	parts := []*genai.Part{
		{Text: "first"},
		{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte{1, 2, 3}}},
		{Text: "summary", Thought: true, ThoughtSignature: []byte{4, 5}},
		{FileData: &genai.FileData{MIMEType: "audio/wav", FileURI: "https://fixture.invalid/media"}},
		{ThoughtSignature: []byte{6, 7}},
		{Text: "last"},
	}
	raw := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Parts: parts}, FinishReason: genai.FinishReasonStop,
	}}}
	// Act.
	response, err := a.ParseResponseForProfile(raw, profile.Identity)
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(wire)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	execution := prompty.SimplePrompt("fixture").AddMessage(message).Clone().Normalize()
	executionWire, err := prompty.MarshalExecution(execution, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	recovered, err := prompty.UnmarshalExecution(executionWire)
	require.NoError(t, err)
	request, err := a.Translate(recovered)
	require.NoError(t, err)
	// Assert: SDK continuation preserves original ordering/data/signatures without aliases.
	expected, err := json.Marshal(parts)
	require.NoError(t, err)
	actual, err := json.Marshal(request.Contents[1].Parts)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
	require.Len(t, restored.Content, len(parts))
	require.Equal(t, []int{2, 4}, []int{restored.ProviderState[0].PartIndex, restored.ProviderState[1].PartIndex})
	require.Equal(t, prompty.ReasoningPart{}, restored.Content[4])
	request.Contents[1].Parts[1].InlineData.Data[0] = 99
	request.Contents[1].Parts[2].ThoughtSignature[0] = 99
	require.Equal(t, byte(1), parts[1].InlineData.Data[0])
	require.Equal(t, byte(4), parts[2].ThoughtSignature[0])
	require.Equal(t, byte(1), restored.Content[1].(prompty.MediaPart).Data[0])
}
