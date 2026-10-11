package gemini

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func TestMixedExactSourceBindingWithUnrelatedFloat(t *testing.T) {
	for _, signed := range []bool{false, true} {
		raw := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "call",
									Name: "lookup",
									Args: map[string]any{"number": int64(9007199254740992), "ordinary": float64(1)},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
			SDKHTTPResponse: &genai.HTTPResponse{
				Body: `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call","name":"lookup","args":{"number":9007199254740993,"ordinary":1}}}]},"finishReason":"STOP"}]}`,
			},
		}
		if signed {
			raw.Candidates[0].Content.Parts[0].ThoughtSignature = []byte("signature")
			raw.SDKHTTPResponse.Body = strings.Replace(
				raw.SDKHTTPResponse.Body,
				`"functionCall":`,
				`"thoughtSignature":"c2lnbmF0dXJl","functionCall":`,
				1,
			)
		}
		response, err := New().ParseResponse(raw)
		require.ErrorIs(t, err, prompty.ErrProviderState)
		require.Nil(t, response)
	}
}
