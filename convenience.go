package prompty

import (
	"context"
	"fmt"
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
		return "", fmt.Errorf("generate text: invoker is nil")
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
		return "", fmt.Errorf("generate text: nil response")
	}
	return resp.Text(), nil
}
