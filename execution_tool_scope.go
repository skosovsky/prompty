package prompty

import (
	"context"
	"errors"
	"strings"
)

// WithToolScope binds a cloned declaration to a cloned execution. It is not IAM.
// Empty Allowed is an explicit denial of all local tools.
func (e *PromptExecution) WithToolScope(scope ToolScope) (*PromptExecution, error) {
	if e == nil {
		return nil, errors.New("tool scope: nil execution")
	}
	out := e.Clone()
	out.toolScope = cloneToolScopePtr(&scope)
	if err := ValidateExecutionToolScope(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ToolScope returns an isolated declaration, or nil when no additional scope exists.
func (e *PromptExecution) ToolScope() *ToolScope {
	if e == nil {
		return nil
	}
	return cloneToolScopePtr(e.toolScope)
}

// ValidateExecutionToolScope validates the current definitions against the bound
// runtime declaration. Call it at every dispatch boundary after caller overrides.
func ValidateExecutionToolScope(exec *PromptExecution) error {
	if exec == nil {
		return errors.New("tool scope: nil execution")
	}
	definitions, err := executionToolDefinitions(exec)
	if err != nil {
		return err
	}
	scope := exec.toolScope
	if scope == nil {
		return nil
	}
	desc := ManifestDescriptor{ID: exec.Metadata.ID, Digest: ""}
	if err := ValidateToolScope(desc, *scope); err != nil {
		return errors.Join(ErrToolScope, err)
	}
	contract := toolScopeContract(*scope)
	for _, def := range exec.Tools {
		allowed, ok := contract.ToolManifest(def.Name)
		if !ok {
			return &ToolScopeError{Tool: def.Name, Reason: "definition outside runtime scope"}
		}
		if !jsonDocumentsEqual(def.Parameters, allowed.Parameters) ||
			normalizedToolDialect(def.Dialect) != normalizedToolDialect(allowed.Dialect) {
			return &ToolScopeError{Tool: def.Name, Reason: "definition schema differs from runtime scope"}
		}
		if err := validateToolManifestSatisfies(desc, toolRequirementFromDefinition(def), allowed); err != nil {
			return errors.Join(ErrToolScope, err)
		}
	}
	for _, req := range scope.Required {
		def, exists := definitions[req.Name]
		if !exists {
			return &ToolScopeError{Tool: req.Name, Reason: "runtime required tool missing from execution"}
		}
		if err := validateToolManifestSatisfies(desc, req, ToolManifestFromDefinition(def)); err != nil {
			return errors.Join(ErrToolScope, err)
		}
	}
	return nil
}
func normalizedToolDialect(dialect string) string {
	if dialect == "" {
		return SchemaDialect2020
	}
	return dialect
}

func executionToolDefinitions(exec *PromptExecution) (map[string]ToolDefinition, error) {
	definitions := make(map[string]ToolDefinition, len(exec.Tools))
	for _, def := range exec.Tools {
		if strings.TrimSpace(def.Name) == "" {
			return nil, &ToolScopeError{Tool: "", Reason: "empty tool definition name"}
		}
		if _, exists := definitions[def.Name]; exists {
			return nil, &ToolScopeError{Tool: def.Name, Reason: "duplicate tool definition"}
		}
		definitions[def.Name] = def
	}
	required := make(map[string]bool)
	for _, name := range exec.RequiredTools {
		if strings.TrimSpace(name) == "" || required[name] {
			return nil, &ToolScopeError{Tool: name, Reason: "empty or duplicate required tool"}
		}
		required[name] = true
		if _, exists := definitions[name]; !exists {
			return nil, &ToolScopeError{Tool: name, Reason: "required tool missing from execution"}
		}
	}
	if exec.ForcedTool != "" {
		if _, exists := definitions[exec.ForcedTool]; !exists {
			return nil, &ToolScopeError{Tool: exec.ForcedTool, Reason: "forced tool missing from execution"}
		}
	}
	return definitions, nil
}

func validateDispatchBoundary(ctx context.Context, execution *PromptExecution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ValidateExecutionToolScope(execution)
}
