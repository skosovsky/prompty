package prompty

import (
	"context"
	"fmt"
)

// ToolValidator validates a tool call without coupling prompty to a concrete tool registry.
type ToolValidator interface {
	ValidateToolCall(name string, argsJSON string) error
}

// ExecuteWithToolValidation retries invalid tool calls by feeding tool validation errors back to the model.
func ExecuteWithToolValidation(
	ctx context.Context,
	invoker Invoker,
	exec *PromptExecution,
	validator ToolValidator,
	maxRetries int,
) (*PromptExecution, error) {
	if invoker == nil {
		return nil, fmt.Errorf("tool validation: invoker is nil")
	}
	if exec == nil {
		return nil, fmt.Errorf("tool validation: execution is nil")
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	workExec := clonePromptExecution(exec)
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return workExec, ctx.Err()
		}

		resp, err := invoker.Generate(ctx, workExec)
		if err != nil {
			return workExec, err
		}
		if resp == nil {
			return workExec, fmt.Errorf("tool validation: nil response")
		}

		workExec = workExec.AddMessage(newAssistantMessageWithContent(resp.Content))
		toolCalls := toolCallsFromContent(resp.Content)
		if len(toolCalls) == 0 {
			return workExec, nil
		}
		if validator == nil {
			return workExec, fmt.Errorf("tool validation: validator is nil")
		}

		callErrs := make([]error, len(toolCalls))
		hasInvalid := false
		for i, toolCall := range toolCalls {
			callErrs[i] = validator.ValidateToolCall(toolCall.Name, toolCall.Args)
			if callErrs[i] != nil {
				hasInvalid = true
				lastErr = callErrs[i]
			}
		}
		if !hasInvalid {
			return workExec, nil
		}

		workExec = appendToolValidationFeedback(workExec, toolCalls, callErrs)
	}

	return workExec, fmt.Errorf("tool validation: validation failed after %d retries: %w", maxRetries+1, lastErr)
}

func toolCallsFromContent(parts []ContentPart) []ToolCallPart {
	out := make([]ToolCallPart, 0)
	for _, part := range parts {
		switch x := part.(type) {
		case ToolCallPart:
			if x.Args == "" {
				x.Args = x.ArgsChunk
			}
			out = append(out, x)
		case *ToolCallPart:
			if x != nil {
				cp := *x
				if cp.Args == "" {
					cp.Args = cp.ArgsChunk
				}
				out = append(out, cp)
			}
		}
	}
	return out
}

func appendToolValidationFeedback(exec *PromptExecution, toolCalls []ToolCallPart, callErrs []error) *PromptExecution {
	for i, toolCall := range toolCalls {
		msg := "Tool call was not executed because the tool batch must be regenerated after validation errors in sibling calls."
		if callErrs[i] != nil {
			msg = callErrs[i].Error()
		}
		exec = exec.AddMessage(newToolResultMessage(toolCall.ID, toolCall.Name, msg, true))
	}
	return exec
}
