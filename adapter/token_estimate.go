package adapter

import (
	"github.com/skosovsky/prompty"
)

// Provider-specific heuristic ratios until native tokenizer APIs are wired.
const (
	OpenAICharsPerToken    = 4
	AnthropicCharsPerToken = 3
	GeminiCharsPerToken    = 4
	OllamaCharsPerToken    = 4
)

// TokenEstimator estimates token usage for a canonical PromptExecution.
type TokenEstimator interface {
	EstimateTokens(exec *prompty.PromptExecution) (int, error)
}

// CharFallbackEstimator implements TokenEstimator using prompty.CharFallbackCounter.
type CharFallbackEstimator struct {
	CharsPerToken int
}

// EstimateTokens implements TokenEstimator.
func (e CharFallbackEstimator) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	if e.CharsPerToken <= 0 {
		return 0, ErrNoTokenEstimator
	}
	return estimateTokensFromExecution(exec, e.CharsPerToken)
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

// ClientWithEstimator exposes token estimation on top of prompty.Invoker.
type ClientWithEstimator struct {
	Invoker   prompty.Invoker
	Estimator TokenEstimator
}

// NewClientWithEstimator wraps an invoker and token estimator.
func NewClientWithEstimator(inv prompty.Invoker, est TokenEstimator) *ClientWithEstimator {
	return &ClientWithEstimator{Invoker: inv, Estimator: est}
}

// EstimateTokens returns token estimate for the execution via the wrapped estimator.
func (c *ClientWithEstimator) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	if c == nil {
		return 0, ErrNoTokenEstimator
	}
	return EstimateTokens(c.Estimator, exec)
}
