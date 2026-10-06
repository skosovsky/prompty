package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func TestGeminiUsageCopiesProviderTotalWithoutAddingBreakdowns(t *testing.T) {
	t.Parallel()
	// Arrange: provider total is authoritative, not a reconstructed sum.
	raw := &genai.GenerateContentResponse{UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount: 10, CandidatesTokenCount: 4, TotalTokenCount: 19,
		CachedContentTokenCount: 3, ThoughtsTokenCount: 5,
	}}
	// Act.
	usage := geminiUsage(raw)
	// Assert.
	require.NoError(t, usage.Validate())
	require.Equal(t, prompty.Usage{
		PromptTokens: 10, CompletionTokens: 4, TotalTokens: 19, PromptTokensCached: 3,
		CompletionTokensReasoning: 5, Mode: prompty.UsageCumulative,
		Known: []prompty.UsageCounter{prompty.UsagePrompt, prompty.UsageCompletion, prompty.UsageTotal,
			prompty.UsageCacheRead, prompty.UsageReasoning},
	}, usage)
}

func TestGeminiUsagePresenceRequiresMatchingNumericHTTPField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		body  string
		known bool
	}{
		{"zero", `{"usageMetadata":{"totalTokenCount":0}}`, true},
		{"missing", `{"usageMetadata":{}}`, false},
		{"null", `{"usageMetadata":{"totalTokenCount":null}}`, false},
		{"string", `{"usageMetadata":{"totalTokenCount":"0"}}`, false},
		{"boolean", `{"usageMetadata":{"totalTokenCount":false}}`, false},
		{"object", `{"usageMetadata":{"totalTokenCount":{}}}`, false},
		{"inconsistent", `{"usageMetadata":{"totalTokenCount":5}}`, false},
		{"malformed", `{"usageMetadata":{"totalTokenCount":0}} broken`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK scalar zero cannot prove presence by itself.
			raw := &genai.GenerateContentResponse{
				UsageMetadata:   &genai.GenerateContentResponseUsageMetadata{},
				SDKHTTPResponse: &genai.HTTPResponse{Body: tc.body},
			}
			// Act.
			usage := geminiUsage(raw)
			// Assert.
			require.NoError(t, usage.Validate())
			if tc.known {
				require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, usage.Known)
				return
			}
			require.Empty(t, usage.Known)
			require.Equal(t, prompty.UsageUnknown, usage.Availability())
		})
	}
}
