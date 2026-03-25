// Package contextx provides context window management middleware for prompty.
package contextx

import (
	"context"
	"iter"

	"github.com/skosovsky/prompty"
)

// WithTokenBudget returns a Middleware that trims exec.Messages to fit within maxTokens
// before invoking next. Original exec is never mutated; a trimmed copy is passed to next.
// System/developer messages are never removed.
func WithTokenBudget(maxTokens int, counter prompty.TokenCounter) prompty.Middleware {
	return func(next prompty.Invoker) prompty.Invoker {
		return &tokenBudgetInvoker{next: next, maxTokens: maxTokens, counter: counter}
	}
}

type tokenBudgetInvoker struct {
	next      prompty.Invoker
	maxTokens int
	counter   prompty.TokenCounter
}

func (t *tokenBudgetInvoker) Generate(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	if exec == nil {
		return t.next.Generate(ctx, exec)
	}

	execCopy, err := exec.Truncated(t.maxTokens, t.counter, prompty.DropOldestStrategy{})
	if err != nil {
		return nil, err
	}
	return t.next.Generate(ctx, execCopy)
}

func (t *tokenBudgetInvoker) GenerateStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	if exec == nil {
		return t.next.GenerateStream(ctx, exec)
	}

	execCopy, err := exec.Truncated(t.maxTokens, t.counter, prompty.DropOldestStrategy{})
	if err != nil {
		return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, err) }
	}
	return t.next.GenerateStream(ctx, execCopy)
}
