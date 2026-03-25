package prompty

import (
	"context"
	"errors"
)

// Option configures a PromptExecution for convenience helpers.
type Option func(*PromptExecution)

// GenerateOption configures GenerateStructured.
type GenerateOption func(*generateStructuredConfig)

type generateStructuredConfig struct {
	retries int
}

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

	resp, err := invoker.Generate(ctx, exec)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("generate text: nil response")
	}
	return resp.Text(), nil
}

// WithRetries enables retry orchestration for GenerateStructured.
func WithRetries(n int) GenerateOption {
	return func(cfg *generateStructuredConfig) {
		if cfg == nil {
			return
		}
		if n < 0 {
			n = 0
		}
		cfg.retries = n
	}
}

// GenerateStructured builds a one-message PromptExecution and returns typed structured output.
func GenerateStructured[T any](ctx context.Context, invoker Invoker, prompt string, opts ...GenerateOption) (T, error) {
	var zero T

	if invoker == nil {
		return zero, errors.New("generate structured: invoker is nil")
	}

	cfg := generateStructuredConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	exec := NewExecution([]ChatMessage{NewUserMessage(prompt)})
	result, err := WithRetry(ctx, exec, cfg.retries, func(ctx context.Context, exec *PromptExecution) (*T, error) {
		return ExecuteWithStructuredOutput[T](ctx, invoker, exec)
	})
	if err != nil {
		return zero, err
	}
	if result == nil {
		return zero, errors.New("generate structured: nil result")
	}
	return *result, nil
}
