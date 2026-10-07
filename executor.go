package prompty

import (
	"context"
	"errors"
)

// NewStructuredExecutor returns a stateful closure that runs one structured-output attempt per call.
// On *ValidationError or *ToolCallError it mutates internal scratchpad state (assistant + follow-up message)
// before returning the same error, so an outer orchestrator (e.g. retry with RetryIf) can invoke the closure again.
// The signature matches common infra patterns such as func(context.Context) (T, error) without importing them.
//
// This optional helper owns one serial attempt chain. Do not call it concurrently.
// The host controls bounds, retries, delay and eventual reconciliation; each call
// performs one attempt and there is no internal loop or worker.
//
// If RawAssistantMessage on the error is nil, only the follow-up message (user feedback or tool results) is appended.
func NewStructuredExecutor[T any](invoker Invoker, exec *PromptExecution) func(context.Context) (*T, error) {
	if exec == nil {
		return func(context.Context) (*T, error) {
			return nil, errors.New("structured executor: execution is nil")
		}
	}
	if invoker == nil {
		return func(context.Context) (*T, error) {
			return nil, errors.New("structured executor: invoker is nil")
		}
	}

	workExec := clonePromptExecution(exec)

	return func(ctx context.Context) (*T, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		result, err := ExecuteWithStructuredOutput[T](ctx, invoker, workExec)
		if err == nil {
			return result, nil
		}

		if valErr, ok := errors.AsType[*ValidationError](err); ok {
			workExec = appendAssistantMessageForSelfCorrection(workExec, valErr.RawAssistantMessage)
			workExec = workExec.AddMessage(NewUserMessage(valErr.FeedbackPrompt))
			return nil, err
		}

		if toolErr, ok := errors.AsType[*ToolCallError](err); ok {
			workExec = appendAssistantMessageForSelfCorrection(workExec, toolErr.RawAssistantMessage)
			workExec = workExec.AddMessage(newToolMessageWithContent(toolErr.ToolResults))
			return nil, err
		}

		return nil, err
	}
}

func appendAssistantMessageForSelfCorrection(exec *PromptExecution, msg *ChatMessage) *PromptExecution {
	if exec == nil || msg == nil {
		return exec
	}
	return exec.AddMessage(cloneChatMessage(*msg))
}
