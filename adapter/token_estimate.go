package adapter

import (
	"github.com/skosovsky/prompty"
)

// TokenEstimator estimates token usage for a canonical PromptExecution.
type TokenEstimator interface {
	EstimateTokens(exec *prompty.PromptExecution) (int, error)
}

// EstimateTokens requires a TokenEstimator (clear-break default).
func EstimateTokens(est TokenEstimator, exec *prompty.PromptExecution) (int, error) {
	return EstimateTokensStrict(est, exec)
}

// EstimateTokensStrict requires a non-nil TokenEstimator.
func EstimateTokensStrict(est TokenEstimator, exec *prompty.PromptExecution) (int, error) {
	if est == nil {
		return 0, ErrNoTokenEstimator
	}
	return est.EstimateTokens(exec)
}
