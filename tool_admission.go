package prompty

import (
	"context"
	"errors"
)

// AdmitToolCalls validates an entire response batch without provider dispatch or
// handler invocation. It returns owned complete calls, or no calls on error.
// The execution's original schemas and scope remain the admission boundary.
// The validator must be pure; authorization and lifecycle policy belong to caller.
// Inputs may not be mutated concurrently. See docs/host-tool-dispatch.md.
func AdmitToolCalls(
	ctx context.Context,
	exec *PromptExecution,
	response *Response,
	validator ToolValidator,
) ([]ToolCallPart, error) {
	if exec == nil || validator == nil {
		return nil, errors.New("tool admission: execution and validator are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	work := exec.Clone()
	snapshot := cloneResponse(response)
	if err := ValidateExecutionToolScope(work); err != nil {
		return nil, err
	}
	schemas, err := compileToolSchemas(work)
	if err != nil {
		return nil, err
	}
	return admitToolCalls(ctx, work, snapshot, validator, schemas)
}

func admitToolCalls(
	ctx context.Context,
	exec *PromptExecution,
	response *Response,
	validator ToolValidator,
	schemas map[string]ValueValidator,
) ([]ToolCallPart, error) {
	if err := errors.Join(ctx.Err(), ValidateExecutionToolScope(exec)); err != nil {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: err}
	}
	if err := ValidateResponseMetadata(response); err != nil {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: err}
	}
	if response.ContinuationUnavailable {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: ErrStateUnavailable}
	}
	if response.Outcome != OutcomeCompleted && response.Outcome != OutcomeToolCalls {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: &OutcomeError{
			Outcome: response.Outcome, FinishReason: response.FinishReason,
		}}
	}
	if err := validateResponseToolScope(exec, response); err != nil {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: err}
	}
	message, err := response.AssistantMessage()
	if err != nil {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: err}
	}
	calls, err := toolCallsFromContent(response.Content)
	if err != nil {
		return nil, &ToolDispatchError{Response: cloneResponse(response), Err: err}
	}
	callErrors, batchErr := validateToolBatch(calls, schemas, validator)
	batchErr = errors.Join(batchErr, ctx.Err())
	if batchErr != nil {
		return nil, &ToolCallError{
			RawAssistantMessage: &message, ToolResults: toolValidationResults(calls, callErrors),
			Err: batchErr, Response: cloneResponse(response),
		}
	}
	return calls, nil
}
