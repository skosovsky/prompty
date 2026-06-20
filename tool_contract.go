package prompty

import (
	"context"
	"errors"
	"fmt"
)

// ToolContract is the minimal required-tool availability contract for prompt materialization.
type ToolContract interface {
	HasTool(name string) bool
}

// ToolContractFunc adapts a function to ToolContract.
type ToolContractFunc func(name string) bool

// HasTool implements ToolContract.
func (f ToolContractFunc) HasTool(name string) bool {
	if f == nil {
		return false
	}
	return f(name)
}

// ErrMissingRequiredTool indicates that a prompt required a tool absent from the supplied contract.
var ErrMissingRequiredTool = errors.New("prompt contract: missing required tool")

// ValidateExecutionContract verifies PromptExecution.RequiredTools against a minimal tool contract.
func ValidateExecutionContract(exec *PromptExecution, contract ToolContract) error {
	if exec == nil {
		return errors.New("prompt contract: execution is nil")
	}
	if len(exec.RequiredTools) == 0 {
		return nil
	}
	if contract == nil {
		return errors.New("prompt contract: tool contract is required")
	}
	for _, name := range exec.RequiredTools {
		if name == "" {
			continue
		}
		if !contract.HasTool(name) {
			return fmt.Errorf("%w: %s", ErrMissingRequiredTool, name)
		}
	}
	return nil
}

// ExecuteWithContract materializes the render plan and validates required tools before returning it.
func (p *RenderPlan) ExecuteWithContract(ctx context.Context, contract ToolContract) (*PromptExecution, error) {
	exec, err := p.Execute(ctx)
	if err != nil {
		return nil, err
	}
	if err := ValidateExecutionContract(exec, contract); err != nil {
		return nil, err
	}
	return exec, nil
}
