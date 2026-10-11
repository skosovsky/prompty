package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func TestSignedOrderedPartsResponseExecutionRoundTrip(t *testing.T) {
	t.Parallel()
	// Arrange: nonadjacent texts must not be aggregated around signed parts/tools.
	a := New()
	signature := []byte{0, 1, 2, 255}
	parts := []*genai.Part{
		{Text: "first"},
		{Text: "visible summary", Thought: true, ThoughtSignature: signature},
		{
			FunctionCall: &genai.FunctionCall{
				ID:   "call",
				Name: "lookup",
				Args: map[string]any{"value": json.Number("1")},
			},
			ThoughtSignature: []byte{3, 4},
		},
		{Text: "last"},
	}
	raw := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{
		{
			Content:          &genai.Content{Parts: parts},
			FinishReason:     genai.FinishReasonStop,
			CitationMetadata: &genai.CitationMetadata{Citations: []*genai.Citation{{URI: "https://source.invalid"}}},
		},
	},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{TotalTokenCount: 5}}
	// Act.
	response, err := a.ParseResponse(raw)
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	var decoded prompty.ResponseWire
	require.NoError(t, json.Unmarshal(data, &decoded))
	restored, err := prompty.UnmarshalResponse(decoded)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	execution := prompty.SimplePrompt("fixture").AddMessage(message).Clone().Normalize()
	executionWire, err := prompty.MarshalExecution(execution, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	final, err := prompty.UnmarshalExecution(executionWire)
	require.NoError(t, err)
	request, err := a.Translate(final)
	require.NoError(t, err)
	// Assert.
	expectedParts, err := json.Marshal(parts)
	require.NoError(t, err)
	actualParts, err := json.Marshal(request.Contents[1].Parts)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedParts), string(actualParts))
	require.Equal(t, signature, request.Contents[1].Parts[1].ThoughtSignature)
	require.Len(t, restored.Content, 4)
	require.Len(t, restored.ProviderState, 2)
	require.Empty(t, restored.Annotations)
	require.Len(t, restored.MessageAnnotations, 1)
	require.Equal(t, prompty.OutcomeToolCalls, restored.Outcome)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, restored.Usage.Known)
	require.Zero(t, restored.Usage.PromptTokens)
	request.Contents[1].Parts[1].ThoughtSignature[0] = 42
	require.Equal(t, byte(0), signature[0])
	require.Equal(t, response.ProviderState, restored.ProviderState)
}

func TestSignedContentCannotBeEditedOrSentToForeignDestination(t *testing.T) {
	t.Parallel()
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "modified", true: "foreign"}[foreign], func(t *testing.T) {
			t.Parallel()
			// Arrange.
			a := New()
			response, err := a.ParseResponse(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Parts: []*genai.Part{{Text: "signed", ThoughtSignature: []byte("signature")}},
					},
					FinishReason: genai.FinishReasonStop,
				},
			}})
			require.NoError(t, err)
			message, err := response.AssistantMessage()
			require.NoError(t, err)
			execution := prompty.SimplePrompt("fixture").AddMessage(message)
			if foreign {
				execution.ModelOptions = &prompty.ModelOptions{Model: "foreign"}
			} else {
				execution.Messages[1].Content[0] = prompty.TextPart{Text: "edited"}
			}
			// Act.
			_, translationErr := a.Translate(execution)
			// Assert.
			if foreign {
				require.ErrorIs(t, translationErr, prompty.ErrStateDestination)
			} else {
				require.ErrorIs(t, translationErr, prompty.ErrProviderState)
			}
		})
	}
}

func TestGeminiRefusalIncompleteAndSDKUnknownZeroUsage(t *testing.T) {
	t.Parallel()
	// Arrange.
	a := New()
	refusal := &genai.GenerateContentResponse{
		Candidates:      []*genai.Candidate{{FinishReason: genai.FinishReasonSafety}},
		UsageMetadata:   &genai.GenerateContentResponseUsageMetadata{},
		SDKHTTPResponse: &genai.HTTPResponse{Body: `{"usageMetadata":{"totalTokenCount":0}}`},
	}
	partial := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: `{"valid":true}`}}},
				FinishReason: genai.FinishReasonMaxTokens,
			},
		},
	}
	// Act.
	refused, err := a.ParseResponse(refusal)
	require.NoError(t, err)
	incomplete, err := a.ParseResponse(partial)
	require.NoError(t, err)
	// Assert.
	require.Equal(t, prompty.OutcomeRefusal, refused.Outcome)
	require.Empty(t, refused.Content)
	require.Empty(t, refused.Usage.Known)
	require.Equal(t, prompty.OutcomeIncomplete, incomplete.Outcome)
	require.Equal(t, string(genai.FinishReasonMaxTokens), incomplete.FinishReason)
}
