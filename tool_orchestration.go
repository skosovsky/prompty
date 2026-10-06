package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ToolInvoker validates and executes typed tool calls.
type ToolInvoker interface {
	ToolValidator
	InvokeTool(ctx context.Context, name string, argsJSON string) (json.RawMessage, error)
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
	if contextErr := ctx.Err(); contextErr != nil {
		return workExec, contextErr
	}
	if err := ValidateExecutionToolScope(workExec); err != nil {
		return workExec, err
	}
	schemas, err := compileToolSchemas(workExec)
	if err != nil {
		return workExec, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return workExec, contextErr
	}
	if scopeErr := ValidateExecutionToolScope(workExec); scopeErr != nil {
		return workExec, scopeErr
	}
	resp, err := invoker.Execute(ctx, workExec.Clone())
	if dispatchErr := errors.Join(err, ctx.Err()); dispatchErr != nil {
		return workExec, &ToolDispatchError{Response: cloneResponse(resp), Err: dispatchErr}
	}
	if resp == nil {
		return workExec, errors.New("typed tools: nil response")
	}
	if metadataErr := ValidateResponseMetadata(resp); metadataErr != nil {
		return workExec, &ToolDispatchError{Response: cloneResponse(resp), Err: metadataErr}
	}
	if resp.Outcome == OutcomeRefusal || resp.Outcome == OutcomeIncomplete {
		return workExec, &ToolDispatchError{
			Response: cloneResponse(resp),
			Err:      &OutcomeError{Outcome: resp.Outcome, FinishReason: resp.FinishReason},
		}
	}
	if scopeErr := validateResponseToolScope(workExec, resp); scopeErr != nil {
		return workExec, &ToolDispatchError{Response: cloneResponse(resp), Err: scopeErr}
	}

	assistantMsg, err := resp.AssistantMessage()
	if err != nil {
		return workExec, &ToolDispatchError{Response: cloneResponse(resp), Err: err}
	}
	toolCalls, err := toolCallsFromContent(resp.Content)
	if err != nil {
		return workExec, &ToolDispatchError{Response: cloneResponse(resp), Err: err}
	}
	if len(toolCalls) == 0 {
		return workExec.AddMessage(assistantMsg), ctx.Err()
	}

	callErrs, batchErr := validateToolBatch(toolCalls, schemas, tools)
	if batchErr != nil {
		return workExec, &ToolCallError{
			RawAssistantMessage: &assistantMsg,
			ToolResults:         toolValidationResults(toolCalls, callErrs),
			Err:                 errors.Join(batchErr, ctx.Err()),
			Response:            cloneResponse(resp),
		}
	}

	return invokeToolBatch(ctx, workExec.AddMessage(assistantMsg), toolCalls, tools)
}

func invokeToolBatch(
	ctx context.Context,
	updated *PromptExecution,
	toolCalls []ToolCallPart,
	tools ToolInvoker,
) (*PromptExecution, error) {
	invokeErrs := make([]error, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if err := ctx.Err(); err != nil {
			return updated, errors.Join(append(invokeErrs, err)...)
		}
		if err := ValidateExecutionToolScope(updated); err != nil {
			return updated, errors.Join(append(invokeErrs, err)...)
		}
		result, invokeErr := tools.InvokeTool(ctx, toolCall.Name, toolCall.Args)
		invokeErr = errors.Join(invokeErr, ctx.Err())
		invokeErrs = append(invokeErrs, invokeErr)
		var resultPart ContentPart
		if invokeErr != nil {
			text := toolResultText(result)
			if len(result) == 0 {
				text = invokeErr.Error()
			}
			resultPart = newToolResultPart(toolCall.ID, toolCall.Name, text, true)
		} else {
			text := toolResultText(result)
			resultPart = newToolResultPart(toolCall.ID, toolCall.Name, text, false)
		}
		updated = updated.AddMessage(newToolMessageWithContent([]ContentPart{resultPart}))
		if cancelErr := ctx.Err(); cancelErr != nil {
			return updated, errors.Join(append(invokeErrs, cancelErr)...)
		}
	}
	if joined := errors.Join(invokeErrs...); joined != nil {
		return updated, fmt.Errorf("typed tools: invoke failed: %w", joined)
	}
	return updated, nil
}

func validateToolBatch(calls []ToolCallPart, schemas map[string]ValueValidator, tools ToolValidator) ([]error, error) {
	callErrs := make([]error, len(calls))
	for i, call := range calls {
		validator, exists := schemas[call.Name]
		if !exists || validator == nil {
			callErrs[i] = &ToolScopeError{Tool: call.Name, Reason: "missing execution schema"}
			continue
		}
		callErrs[i] = validator.ValidateJSON(JSONDocument(call.Args))
		if callErrs[i] == nil {
			callErrs[i] = tools.ValidateToolCall(call.Name, call.Args)
		}
	}
	return callErrs, errors.Join(callErrs...)
}

// A definition without parameter constraints still requires a JSON object.
// Every declared schema is compiled before invocation; registry schemas cannot
// substitute for the execution's original, caller-owned constraints.
func compileToolSchemas(exec *PromptExecution) (map[string]ValueValidator, error) {
	compiler := exec.Validator
	if compiler == nil {
		compiler = NewJSONSchemaValidator(nil)
	}
	compiled := make(map[string]ValueValidator, len(exec.Tools))
	for _, definition := range exec.Tools {
		if definition.Name == "" {
			return nil, &ToolScopeError{Tool: "", Reason: "empty tool definition name"}
		}
		if _, exists := compiled[definition.Name]; exists {
			return nil, &ToolScopeError{Tool: definition.Name, Reason: "duplicate tool definition"}
		}
		source := definition.Parameters
		if len(source) == 0 {
			source = JSONDocument(`{"type":"object"}`)
		}
		validator, err := compileSchemaSnapshot(compiler, source, definition.Dialect)
		if err != nil {
			return nil, fmt.Errorf("tool %q schema: %w", definition.Name, err)
		}
		if validator == nil {
			return nil, ErrSchemaValidatorRequired
		}
		compiled[definition.Name] = validator
	}
	return compiled, nil
}

// ErrToolScope indicates that definitions or calls violate the execution boundary.
var ErrToolScope = errors.New("prompty: tool call violates execution scope")

// ToolScopeError describes an execution-scoped contract violation, not an authorization decision.
type ToolScopeError struct {
	Tool   string
	Reason string
}

func (e *ToolScopeError) Error() string {
	return fmt.Sprintf("%v: %q: %s", ErrToolScope, e.Tool, e.Reason)
}
func (*ToolScopeError) Unwrap() error { return ErrToolScope }

func validateResponseToolScope(exec *PromptExecution, response *Response) error {
	parts := response.Content
	names := make(map[string]int)
	for _, part := range parts {
		if call, ok := toolCallPartValue(part); ok {
			names[call.Name]++
		}
	}
	allowed := toolDefinitionsByName(exec.Tools)
	seen := make(map[string]bool)
	for _, part := range parts {
		call, ok := toolCallPartValue(part)
		if !ok {
			continue
		}
		if _, exists := allowed[call.Name]; !exists {
			return &ToolScopeError{Tool: call.Name, Reason: "tool is outside execution scope"}
		}
		if exec.ForcedTool != "" && call.Name != exec.ForcedTool {
			return &ToolScopeError{Tool: call.Name, Reason: "tool conflicts with forced tool"}
		}
		if call.ID == "" && (!checkedToolNameCorrelation(exec, response.Diagnostics) || names[call.Name] != 1 ||
			call.Args == "" || call.ArgsChunk != "") {
			return &ToolScopeError{Tool: call.Name, Reason: "unconfirmed or ambiguous ID-less tool correlation"}
		}
		// Full responses contain complete calls, not stream fragments. Never merge
		// duplicate full calls, which could hide an unexpected second side effect.
		if call.ID != "" {
			if seen[call.ID] {
				return &ToolScopeError{Tool: call.Name, Reason: "duplicate tool call ID"}
			}
			seen[call.ID] = true
		}
	}
	return nil
}

func checkedToolNameCorrelation(exec *PromptExecution, report ExecutionDiagnostics) bool {
	if !slices.Contains(exec.Requirements.Capabilities, CapabilityToolNameCorrelation) {
		return false
	}
	for _, value := range []string{report.Profile.Provider, report.Profile.Endpoint, report.Profile.Model,
		report.Provenance, report.Snapshot} {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" && exec.ModelOptions.Model != report.Profile.Model {
		return false
	}
	if profile := exec.Requirements.Override; profile != nil {
		if ValidateCapabilityProfile(*profile) != nil || profile.Identity != report.Profile ||
			profile.Capabilities[CapabilityTools] != CapabilitySupported ||
			profile.Capabilities[CapabilityToolNameCorrelation] != CapabilitySupported {
			return false
		}
	}
	return checkedToolCapability(report, CapabilityTools) &&
		checkedToolCapability(report, CapabilityToolNameCorrelation)
}

func checkedToolCapability(report ExecutionDiagnostics, capability string) bool {
	checks := 0
	for _, check := range report.Checks {
		if check.Requirement != capability {
			continue
		}
		if check.Status != CapabilitySupported {
			return false
		}
		checks++
	}
	return checks == 1
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
