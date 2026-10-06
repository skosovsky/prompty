// Package contextx provides context window management middleware for prompty.
package contextx

import (
	"context"

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

	if scopeErr := prompty.ValidateExecutionToolScope(exec); scopeErr != nil {
		return nil, scopeErr
	}
	messages, err := exttruncate.DropOldest(exec.Messages, t.maxTokens, t.counter)
	if err != nil {
		return nil, err
	}
	execCopy := exec.WithMessages(messages)
	if scopeErr := prompty.ValidateExecutionToolScope(execCopy); scopeErr != nil {
		return nil, scopeErr
	}
	return t.next.Execute(ctx, execCopy)
}

func (t *tokenBudgetInvoker) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	if exec == nil {
		return t.next.ExecuteStream(ctx, exec, mode)
	}

	if scopeErr := prompty.ValidateExecutionToolScope(exec); scopeErr != nil {
		return prompty.ErrorStream(ctx, mode, scopeErr)
	}
	messages, err := exttruncate.DropOldest(exec.Messages, t.maxTokens, t.counter)
	if err != nil {
		return prompty.ErrorStream(ctx, mode, err)
	}
	execCopy := exec.WithMessages(messages)
	if scopeErr := prompty.ValidateExecutionToolScope(execCopy); scopeErr != nil {
		return prompty.ErrorStream(ctx, mode, scopeErr)
	}
	return t.next.ExecuteStream(ctx, execCopy, mode)
}
