package prompty

import (
	"context"
	"errors"
	"fmt"
)

// WithRetry replays executor with scratchpad feedback for retryable typed errors.
func WithRetry[T any](
	ctx context.Context,
	exec *PromptExecution,
	maxRetries int,
	executor func(context.Context, *PromptExecution) (T, error),
) (T, error) {
	var zero T

	if executor == nil {
		return zero, fmt.Errorf("retry: executor is nil")
	}
	if exec == nil {
		return zero, fmt.Errorf("retry: execution is nil")
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	workExec := clonePromptExecution(exec)
	for attempt := 0; ; attempt++ {
		if err := retryContextErr(ctx); err != nil {
			return zero, err
		}

		result, err := executor(ctx, workExec)
		if err == nil {
			return result, nil
		}
		if attempt == maxRetries {
			return zero, err
		}

		var valErr *ValidationError
		if errors.As(err, &valErr) {
			workExec = appendRetryAssistantMessage(workExec, valErr.RawAssistantMessage)
			workExec = workExec.AddMessage(NewUserMessage(valErr.FeedbackPrompt))
			if ctxErr := retryContextErr(ctx); ctxErr != nil {
				return zero, ctxErr
			}
			continue
		}

		var toolErr *ToolCallError
		if errors.As(err, &toolErr) {
			workExec = appendRetryAssistantMessage(workExec, toolErr.RawAssistantMessage)
			workExec = workExec.AddMessage(newToolMessageWithContent(toolErr.ToolResults))
			if ctxErr := retryContextErr(ctx); ctxErr != nil {
				return zero, ctxErr
			}
			continue
		}

		return zero, err
	}
}

func retryContextErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("retry aborted due to context cancellation: %w", err)
	}
	return nil
}

func appendRetryAssistantMessage(exec *PromptExecution, msg *ChatMessage) *PromptExecution {
	if exec == nil || msg == nil {
		return exec
	}
	return exec.AddMessage(cloneChatMessage(*msg))
}
