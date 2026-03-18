// Package contextx provides context window management middleware for prompty.
package contextx

import (
	"context"
	"iter"

	"github.com/skosovsky/prompty"
)

// trimConfig holds token budget and media penalty for counting.
type trimConfig struct {
	maxTokens    int
	counter      prompty.TokenCounter
	mediaPenalty int // tokens per MediaPart (LLMs charge ~250-1000 per image)
}

// TokenBudgetOption configures WithTokenBudget.
type TokenBudgetOption func(*trimConfig)

// WithMediaTokenPenalty sets token penalty per MediaPart (image, audio, etc).
// LLM providers charge ~250-1000 tokens per image. Default is 256.
func WithMediaTokenPenalty(n int) TokenBudgetOption {
	return func(c *trimConfig) { c.mediaPenalty = n }
}

// WithTokenBudget returns a Middleware that trims exec.Messages to fit within maxTokens
// before invoking next. Original exec is never mutated; a trimmed copy is passed to next.
// System/developer messages are never removed.
func WithTokenBudget(maxTokens int, counter prompty.TokenCounter, opts ...TokenBudgetOption) prompty.Middleware {
	cfg := &trimConfig{maxTokens: maxTokens, counter: counter, mediaPenalty: 256}
	for _, opt := range opts {
		opt(cfg)
	}
	return func(next prompty.Invoker) prompty.Invoker {
		return &tokenBudgetInvoker{next: next, cfg: cfg}
	}
}

type tokenBudgetInvoker struct {
	next prompty.Invoker
	cfg  *trimConfig
}

func (t *tokenBudgetInvoker) Generate(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	if exec == nil {
		return t.next.Generate(ctx, exec)
	}

	execCopy := exec.Clone()
	counter := wrapCounterWithMediaPenalty(t.cfg.counter, t.cfg.mediaPenalty)
	if err := execCopy.Truncate(t.cfg.maxTokens, counter); err != nil {
		return nil, err
	}
	return t.next.Generate(ctx, execCopy)
}

func (t *tokenBudgetInvoker) GenerateStream(ctx context.Context, exec *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
	if exec == nil {
		return t.next.GenerateStream(ctx, exec)
	}

	execCopy := exec.Clone()
	counter := wrapCounterWithMediaPenalty(t.cfg.counter, t.cfg.mediaPenalty)
	if err := execCopy.Truncate(t.cfg.maxTokens, counter); err != nil {
		return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, err) }
	}
	return t.next.GenerateStream(ctx, execCopy)
}

type mediaPenaltyCounter struct {
	prompty.TokenCounter
	penalty int
}

func (c *mediaPenaltyCounter) MediaTokenPenalty() int {
	return c.penalty
}

func wrapCounterWithMediaPenalty(counter prompty.TokenCounter, penalty int) prompty.TokenCounter {
	if counter == nil {
		return nil
	}
	return &mediaPenaltyCounter{TokenCounter: counter, penalty: penalty}
}
