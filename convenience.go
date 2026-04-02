package prompty

import (
	"context"
	"errors"
)

// Option configures a PromptExecution for convenience helpers.
type Option func(*PromptExecution)

// SimpleChat builds a typical execution with one system instruction and one user message.
func SimpleChat(system, user string) *PromptExecution {
	return NewExecution([]ChatMessage{
		NewSystemMessage(system),
		NewUserMessage(user),
	})
}

// SimplePrompt builds an execution with only one user message.
func SimplePrompt(user string) *PromptExecution {
	return NewExecution([]ChatMessage{
		NewUserMessage(user),
	})
}

// GenerateText is intended for prototyping, scripts, and simple tasks.
// For production, use prompty-gen and typed facade executions to maintain observability.
func GenerateText(ctx context.Context, invoker Invoker, prompt string, opts ...Option) (string, error) {
	if invoker == nil {
		return "", errors.New("generate text: invoker is nil")
	}

	exec := NewExecution([]ChatMessage{NewUserMessage(prompt)})
	for _, opt := range opts {
		if opt != nil {
			opt(exec)
		}
	}

	resp, err := invoker.Execute(ctx, exec)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("generate text: nil response")
	}
	return resp.Text(), nil
}

// GenerateStructured builds a one-message PromptExecution and returns typed structured output in a single model call.
// For self-correction across attempts, use [NewStructuredExecutor] and an outer orchestrator (timeouts, retries).
func GenerateStructured[T any](ctx context.Context, invoker Invoker, prompt string) (T, error) {
	var zero T

	if invoker == nil {
		return zero, errors.New("generate structured: invoker is nil")
	}

	exec := NewExecution([]ChatMessage{NewUserMessage(prompt)})
	result, err := ExecuteWithStructuredOutput[T](ctx, invoker, exec)
	if err != nil {
		return zero, err
	}
	if result == nil {
		return zero, errors.New("generate structured: nil result")
	}
	return *result, nil
}
