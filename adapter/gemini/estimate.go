package gemini

import (
	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// EstimateTokens implements adapter.TokenEstimator.
func (*Adapter) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	return adapter.CharFallbackEstimator{CharsPerToken: adapter.GeminiCharsPerToken}.EstimateTokens(exec)
}
