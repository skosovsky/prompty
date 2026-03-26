package prompty

import (
	"context"
	"errors"
)

// ToolValidator validates a tool call without coupling prompty to a concrete tool registry.
type ToolValidator interface {
	ValidateToolCall(name string, argsJSON string) error
}

// ExecuteWithToolValidation performs one model call and validates tool call arguments.
func ExecuteWithToolValidation(
	ctx context.Context,
	invoker Invoker,
	exec *PromptExecution,
	validator ToolValidator,
) (*PromptExecution, error) {
	if invoker == nil {
		return nil, errors.New("tool validation: invoker is nil")
	}
	if exec == nil {
		return nil, errors.New("tool validation: execution is nil")
	}

	workExec := clonePromptExecution(exec)
	resp, err := invoker.Execute(ctx, workExec)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("tool validation: nil response")
	}

	assistantMsg := newAssistantMessageWithContent(resp.Content)
	toolCalls := toolCallsFromContent(resp.Content)
	if len(toolCalls) == 0 {
		return workExec.AddMessage(assistantMsg), nil
	}
	if validator == nil {
		return workExec, errors.New("tool validation: validator is nil")
	}

	callErrs := make([]error, len(toolCalls))
	invalidErrs := make([]error, 0, len(toolCalls))
	for i, toolCall := range toolCalls {
		callErrs[i] = validator.ValidateToolCall(toolCall.Name, toolCall.Args)
		if callErrs[i] != nil {
			invalidErrs = append(invalidErrs, callErrs[i])
		}
	}
	if len(invalidErrs) == 0 {
		return workExec.AddMessage(assistantMsg), nil
	}

	return workExec, &ToolCallError{
		RawAssistantMessage: &assistantMsg,
		ToolResults:         toolValidationResults(toolCalls, callErrs),
		Err:                 errors.Join(invalidErrs...),
	}
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

func toolValidationResults(toolCalls []ToolCallPart, callErrs []error) []ContentPart {
	results := make([]ContentPart, 0, len(toolCalls))
	for i, toolCall := range toolCalls {
		msg := "Tool call was not executed because the tool batch must be regenerated after validation errors in sibling calls."
		if callErrs[i] != nil {
			msg = callErrs[i].Error()
		}
		results = append(results, newToolResultPart(toolCall.ID, toolCall.Name, msg, true))
	}
	return results
}
