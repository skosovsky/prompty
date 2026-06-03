package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ToolInvoker validates and executes typed tool calls.
type ToolInvoker interface {
	ToolValidator
	InvokeTool(name string, argsJSON string) (json.RawMessage, error)
}

// ExecuteWithTypedTools performs one model call, validates tool args, and invokes handlers.
func ExecuteWithTypedTools(
	ctx context.Context,
	invoker Invoker,
	exec *PromptExecution,
	tools ToolInvoker,
) (*PromptExecution, error) {
	if invoker == nil {
		return nil, errors.New("typed tools: invoker is nil")
	}
	if exec == nil {
		return nil, errors.New("typed tools: execution is nil")
	}
	if tools == nil {
		return nil, errors.New("typed tools: tool invoker is nil")
	}

	workExec := clonePromptExecution(exec)
	resp, err := invoker.Execute(ctx, workExec)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("typed tools: nil response")
	}

	assistantMsg := newAssistantMessageWithContent(resp.Content)
	toolCalls, err := toolCallsFromContent(resp.Content)
	if err != nil {
		return nil, err
	}
	if len(toolCalls) == 0 {
		return workExec.AddMessage(assistantMsg), nil
	}

	callErrs := make([]error, len(toolCalls))
	invalidErrs := make([]error, 0, len(toolCalls))
	for i, toolCall := range toolCalls {
		callErrs[i] = tools.ValidateToolCall(toolCall.Name, toolCall.Args)
		if callErrs[i] != nil {
			invalidErrs = append(invalidErrs, callErrs[i])
		}
	}
	if len(invalidErrs) > 0 {
		return workExec, &ToolCallError{
			RawAssistantMessage: &assistantMsg,
			ToolResults:         toolValidationResults(toolCalls, callErrs),
			Err:                 errors.Join(invalidErrs...),
		}
	}

	updated := workExec.AddMessage(assistantMsg)
	invokeErrs := make([]error, len(toolCalls))
	for i, toolCall := range toolCalls {
		result, invokeErr := tools.InvokeTool(toolCall.Name, toolCall.Args)
		var resultPart ContentPart
		if invokeErr != nil {
			invokeErrs[i] = invokeErr
			resultPart = newToolResultPart(toolCall.ID, toolCall.Name, invokeErr.Error(), true)
		} else {
			text := toolResultText(result)
			resultPart = newToolResultPart(toolCall.ID, toolCall.Name, text, false)
		}
		updated = updated.AddMessage(newToolMessageWithContent([]ContentPart{resultPart}))
	}
	if joined := errors.Join(invokeErrs...); joined != nil {
		return updated, fmt.Errorf("typed tools: invoke failed: %w", joined)
	}
	return updated, nil
}

func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
