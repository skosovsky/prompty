package anthropic

import (
	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const charsPerToken = 3

// EstimateTokens implements adapter.TokenEstimator.
func (*Adapter) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	if exec == nil {
		return 0, adapter.ErrNilExecution
	}
	counter := &prompty.CharFallbackCounter{CharsPerToken: charsPerToken}
	total := 0
	for _, msg := range exec.Messages {
		n, err := counter.CountMessage(msg)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}
