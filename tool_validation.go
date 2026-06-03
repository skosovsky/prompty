package prompty

import (
	"context"
	"errors"
)

// ErrToolInvokerRequired indicates tool orchestration requires a typed ToolInvoker.
var ErrToolInvokerRequired = errors.New("tool validation: validator must implement ToolInvoker")

// ToolValidator validates a tool call without coupling prompty to a concrete tool registry.
type ToolValidator interface {
	ValidateToolCall(name string, argsJSON string) error
}

// ExecuteWithToolValidation performs one model call, validates tool args, and invokes handlers.
// Clear-break: validator must implement ToolInvoker (use TypedToolRegistry or custom ToolInvoker).
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
	if validator == nil {
		return nil, errors.New("tool validation: tool invoker is nil")
	}
	toolInvoker, ok := validator.(ToolInvoker)
	if !ok {
		return nil, ErrToolInvokerRequired
	}
	return ExecuteWithTypedTools(ctx, invoker, exec, toolInvoker)
}

func toolCallsFromContent(parts []ContentPart) ([]ToolCallPart, error) {
	var err error
	parts, err = GlueToolCallArgChunks(parts)
	if err != nil {
		return nil, err
	}
	out := make([]ToolCallPart, 0)
	for _, part := range parts {
		switch x := part.(type) {
		case ToolCallPart:
			args, err := resolvedToolCallArgs(x)
			if err != nil {
				return nil, err
			}
			x.Args = args
			x.ArgsChunk = ""
			out = append(out, x)
		case *ToolCallPart:
			if x == nil {
				continue
			}
			cp := *x
			args, err := resolvedToolCallArgs(cp)
			if err != nil {
				return nil, err
			}
			cp.Args = args
			cp.ArgsChunk = ""
			out = append(out, cp)
		}
	}
	return out, nil
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
