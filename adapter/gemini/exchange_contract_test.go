package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestDirectUnsignedNumericRepresentations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		value    any
		accepted bool
	}{
		{"integer", int64(9007199254740993), true},
		{"unsigned integer", uint64(9007199254740993), true},
		{"exact decimal", json.Number("0.10000000000000001"), true},
		{"SDK float64", float64(1), false},
		{"typed nested float", map[string][]float64{"values": {1}}, false},
		{"nested SDK float", []any{map[string]any{"n": float64(2)}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: no original source accompanies a caller-supplied SDK DTO.
			raw := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{
					{
						Text: "known prefix",
					}, {FunctionCall: &genai.FunctionCall{ID: "call", Name: "lookup", Args: map[string]any{"number": test.value}}},
				}}, FinishReason: genai.FinishReasonStop}},
			}
			// Act.
			response, err := New().ParseResponse(raw)
			// Assert.
			if test.accepted {
				require.NoError(t, err)
				require.Len(t, response.Content, 2)
			} else {
				require.ErrorIs(t, err, prompty.ErrProviderState)
				require.NotNil(t, response)
				require.Len(t, response.Content, 1)
				require.True(t, response.ContinuationUnavailable)
				require.Empty(t, response.Outcome)
				require.Empty(t, response.FinishReason)
				require.Equal(t, "known prefix", response.Content[0].(prompty.TextPart).Text)
			}
		})
	}
}

func TestGeminiHistoryCorrelationFailsWithoutGuessing(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		calls   []prompty.ContentPart
		results []prompty.ContentPart
	}{
		{"mixed name and ID", []prompty.ContentPart{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}, prompty.ToolCallPart{Name: "lookup", Args: "{}"}}, nil},
		{"mixed ID and name", []prompty.ContentPart{prompty.ToolCallPart{Name: "lookup", Args: "{}"}, prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}}, nil},
		{"duplicate call ID", []prompty.ContentPart{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}, prompty.ToolCallPart{ID: "a", Name: "other", Args: "{}"}}, nil},
		{"ambiguous name only", []prompty.ContentPart{prompty.ToolCallPart{Name: "lookup", Args: "{}"}, prompty.ToolCallPart{Name: "lookup", Args: "{}"}}, nil},
		{"unknown result ID", []prompty.ContentPart{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}}, []prompty.ContentPart{prompty.ToolResultPart{ToolCallID: "b", Name: "lookup"}}},
		{"changed result name", []prompty.ContentPart{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}}, []prompty.ContentPart{prompty.ToolResultPart{ToolCallID: "a", Name: "other"}}},
		{"duplicate result", []prompty.ContentPart{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}}, []prompty.ContentPart{prompty.ToolResultPart{ToolCallID: "a", Name: "lookup"}, prompty.ToolResultPart{ToolCallID: "a", Name: "lookup"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution := prompty.SimplePrompt("fixture").
				AddMessage(prompty.ChatMessage{Role: prompty.RoleAssistant, Content: test.calls})
			if test.results != nil {
				execution = execution.AddMessage(prompty.ChatMessage{Role: prompty.RoleTool, Content: test.results})
			}
			// Act.
			request, err := New().Translate(execution)
			// Assert.
			require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
			require.Nil(t, request)
		})
	}
}

func TestDirectNumericInspectionDoesNotImposeNestingPolicyOrRecurseIntoCycles(t *testing.T) {
	t.Parallel()
	for _, cyclic := range []bool{false, true} {
		t.Run(map[bool]string{false: "deep exact integer", true: "cycle"}[cyclic], func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var value any = int64(9007199254740993)
			for range 200 {
				value = map[string]any{"nested": value}
			}
			args := map[string]any{"value": value}
			if cyclic {
				args["cycle"] = args
			}
			raw := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []*genai.Part{
								{FunctionCall: &genai.FunctionCall{ID: "call", Name: "lookup", Args: args}},
							},
						},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}
			// Act.
			response, err := New().ParseResponse(raw)
			// Assert.
			if cyclic {
				require.ErrorIs(t, err, adapter.ErrMalformedArgs)
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.Len(t, response.Content, 1)
				encoded, marshalErr := json.Marshal(args)
				require.NoError(t, marshalErr)
				require.Equal(t, string(encoded), response.Content[0].(prompty.ToolCallPart).Args)
			}
		})
	}
}

func TestCallerExactSDKDTOAndSourceMustAgree(t *testing.T) {
	t.Parallel()
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "rounded integer conflict"}[mismatch], func(t *testing.T) {
			t.Parallel()
			// Arrange: typed integers cannot be justified by SDK float normalization.
			number := int64(9007199254740993)
			if mismatch {
				number--
			}
			raw := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []*genai.Part{
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "call",
										Name: "lookup",
										Args: map[string]any{"number": number},
									},
								},
							},
						},
						FinishReason: genai.FinishReasonStop,
					},
				},
				SDKHTTPResponse: &genai.HTTPResponse{
					Body: `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call","name":"lookup","args":{"number":9007199254740993}}}]},"finishReason":"STOP"}]}`,
				},
			}
			// Act.
			response, err := New().ParseResponse(raw)
			// Assert.
			if mismatch {
				require.ErrorIs(t, err, prompty.ErrProviderState)
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.Len(t, response.Content, 1)
				require.Equal(t, `{"number":9007199254740993}`, response.Content[0].(prompty.ToolCallPart).Args)
			}
		})
	}
}
