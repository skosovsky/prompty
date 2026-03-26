// Package contextx provides context window management middleware for prompty.
package contextx

import (
	"context"
	"iter"

	"github.com/skosovsky/prompty"
	exttruncate "github.com/skosovsky/prompty/ext/truncate"
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

func (t *tokenBudgetInvoker) Execute(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	if exec == nil {
		return t.next.Execute(ctx, exec)
	}

	messages, err := exttruncate.DropOldest(exec.Messages, t.maxTokens, t.counter)
	if err != nil {
		return nil, err
	}
	execCopy := exec.WithMessages(messages)
	return t.next.Execute(ctx, execCopy)
}

func (t *tokenBudgetInvoker) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	if exec == nil {
		return t.next.ExecuteStream(ctx, exec)
	}

	messages, err := exttruncate.DropOldest(exec.Messages, t.maxTokens, t.counter)
	if err != nil {
		return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, err) }
	}
	execCopy := exec.WithMessages(messages)
	return t.next.ExecuteStream(ctx, execCopy)
}
