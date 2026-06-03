package openai

import (
	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// EstimateTokens implements adapter.TokenEstimator with OpenAI-oriented char/token ratio.
func (*Adapter) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	return adapter.CharFallbackEstimator{CharsPerToken: adapter.OpenAICharsPerToken}.EstimateTokens(exec)
}
