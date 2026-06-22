package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// ToolManifest is the runtime tool contract visible to prompt materialization.
type ToolManifest struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Parameters   JSONDocument `json:"parameters,omitempty"`
	Capabilities []string     `json:"capabilities,omitempty"`
}

// ToolRequirement describes a tool that must be available in a runtime scope.
type ToolRequirement struct {
	Name         string       `json:"name"`
	Parameters   JSONDocument `json:"parameters,omitempty"`
	Capabilities []string     `json:"capabilities,omitempty"`
}

// ToolManifestContract exposes full runtime tool manifests for fail-closed prompt validation.
type ToolManifestContract interface {
	ToolManifest(name string) (ToolManifest, bool)
}

// ToolManifestContractFunc adapts a function to ToolManifestContract.
type ToolManifestContractFunc func(name string) (ToolManifest, bool)

// ToolManifest implements ToolManifestContract.
func (f ToolManifestContractFunc) ToolManifest(name string) (ToolManifest, bool) {
	if f == nil {
		return ToolManifest{}, false
	}
	return f(name)
}

// ErrMissingRequiredTool indicates that a prompt required a tool absent from the supplied manifest contract.
var ErrMissingRequiredTool = errors.New("prompt contract: missing required tool")

// ErrToolContractMismatch indicates a runtime tool manifest does not satisfy the prompt contract.
var ErrToolContractMismatch = errors.New("prompt contract: tool manifest mismatch")

// ToolContractError carries audit-friendly context for tool contract failures.
type ToolContractError struct {
	Descriptor ManifestDescriptor
	ToolName   string
	Reason     string
	Err        error
}

func (e *ToolContractError) Error() string {
	if e == nil {
		return ErrToolContractMismatch.Error()
	}
	var b strings.Builder
	if e.Err != nil {
		b.WriteString(e.Err.Error())
	} else {
		b.WriteString(ErrToolContractMismatch.Error())
	}
	if e.Descriptor.ID != "" {
		b.WriteString(": descriptor ")
		b.WriteString(e.Descriptor.ID)
		if e.Descriptor.Digest != "" {
			b.WriteString("@")
			b.WriteString(e.Descriptor.Digest)
		}
	}
	if e.ToolName != "" {
		b.WriteString(": tool ")
		b.WriteString(e.ToolName)
	}
	if e.Reason != "" {
		b.WriteString(": ")
		b.WriteString(e.Reason)
	}
	return b.String()
}

func (e *ToolContractError) Unwrap() error {
	if e == nil || e.Err == nil {
		return ErrToolContractMismatch
	}
	return e.Err
}

// ToolManifestFromDefinition converts a prompt tool definition into a manifest contract.
func ToolManifestFromDefinition(def ToolDefinition) ToolManifest {
	return ToolManifest{
		Name:         def.Name,
		Description:  def.Description,
		Parameters:   CloneJSONDocument(def.Parameters),
		Capabilities: cloneStringSlice(def.Capabilities),
	}
}

func toolRequirementFromDefinition(def ToolDefinition) ToolRequirement {
	return ToolRequirement{
		Name:         def.Name,
		Parameters:   CloneJSONDocument(def.Parameters),
		Capabilities: cloneStringSlice(def.Capabilities),
	}
}

func toolScopeContract(scope ToolScope) ToolManifestContract {
	byName := make(map[string]ToolManifest, len(scope.Allowed))
	for _, tool := range scope.Allowed {
		if tool.Name == "" {
			continue
		}
		byName[tool.Name] = cloneToolManifest(tool)
	}
	return ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		tool, ok := byName[name]
		return cloneToolManifest(tool), ok
	})
}

func cloneToolManifest(tool ToolManifest) ToolManifest {
	return ToolManifest{
		Name:         tool.Name,
		Description:  tool.Description,
		Parameters:   CloneJSONDocument(tool.Parameters),
		Capabilities: cloneStringSlice(tool.Capabilities),
	}
}

func cloneToolRequirement(req ToolRequirement) ToolRequirement {
	return ToolRequirement{
		Name:         req.Name,
		Parameters:   CloneJSONDocument(req.Parameters),
		Capabilities: cloneStringSlice(req.Capabilities),
	}
}

// ValidateExecutionContract verifies PromptExecution.RequiredTools against a runtime tool manifest contract.
func ValidateExecutionContract(exec *PromptExecution, contract ToolManifestContract) error {
	desc := ManifestDescriptor{ID: "", Digest: ""}
	if exec != nil {
		desc.ID = exec.Metadata.ID
	}
	return ValidateExecutionManifestContract(exec, desc, contract)
}

// ValidateExecutionManifestContract verifies required tools against a runtime tool manifest contract.
func ValidateExecutionManifestContract(
	exec *PromptExecution,
	desc ManifestDescriptor,
	contract ToolManifestContract,
) error {
	if exec == nil {
		return errors.New("prompt contract: execution is nil")
	}
	if len(exec.RequiredTools) == 0 {
		return nil
	}
	if contract == nil {
		return errors.New("prompt contract: tool manifest contract is required")
	}
	allowed := toolDefinitionsByName(exec.Tools)
	for _, name := range exec.RequiredTools {
		if name == "" {
			return toolContractErr(desc, "", "required tool name is required", ErrToolContractMismatch)
		}
		expected, declared := allowed[name]
		if len(exec.Tools) > 0 && !declared {
			return toolContractErr(desc, name, "required tool is outside prompt allowed tools", ErrToolContractMismatch)
		}
		actual, ok := contract.ToolManifest(name)
		if !ok {
			return toolContractErr(desc, name, "runtime manifest is missing", ErrMissingRequiredTool)
		}
		requirement := ToolRequirement{Name: name, Parameters: nil, Capabilities: nil}
		if declared {
			requirement = toolRequirementFromDefinition(expected)
		}
		if err := validateToolManifestSatisfies(desc, requirement, actual); err != nil {
			return err
		}
	}
	return nil
}

func toolDefinitionsByName(tools []ToolDefinition) map[string]ToolDefinition {
	byName := make(map[string]ToolDefinition, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		byName[tool.Name] = tool
	}
	return byName
}

func validateToolManifestSatisfies(desc ManifestDescriptor, expected ToolRequirement, actual ToolManifest) error {
	if expected.Name == "" {
		return nil
	}
	if actual.Name != expected.Name {
		return toolContractErr(
			desc,
			expected.Name,
			fmt.Sprintf("runtime manifest name mismatch: got %q", actual.Name),
			ErrToolContractMismatch,
		)
	}
	if len(expected.Parameters) > 0 && !jsonDocumentsEqual(expected.Parameters, actual.Parameters) {
		return toolContractErr(desc, expected.Name, "parameters schema mismatch", ErrToolContractMismatch)
	}
	for _, capability := range expected.Capabilities {
		if capability == "" {
			continue
		}
		if !slices.Contains(actual.Capabilities, capability) {
			return toolContractErr(
				desc,
				expected.Name,
				fmt.Sprintf("missing capability %q", capability),
				ErrToolContractMismatch,
			)
		}
	}
	return nil
}

func toolContractErr(desc ManifestDescriptor, toolName, reason string, err error) error {
	return &ToolContractError{
		Descriptor: desc,
		ToolName:   toolName,
		Reason:     reason,
		Err:        err,
	}
}

func jsonDocumentsEqual(a, b JSONDocument) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var av any
	if err := json.Unmarshal(a, &av); err != nil {
		return string(a) == string(b)
	}
	var bv any
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// ExecuteWithContract materializes the render plan and validates required tools before returning it.
func (p *RenderPlan) ExecuteWithContract(
	ctx context.Context,
	contract ToolManifestContract,
) (*PromptExecution, error) {
	desc := ManifestDescriptor{ID: "", Digest: ""}
	if p != nil && p.template != nil {
		desc.ID = p.template.Metadata.ID
	}
	return p.ExecuteWithManifestContract(ctx, desc, contract)
}

// ExecuteWithManifestContract materializes the render plan and validates required tools with descriptor context.
func (p *RenderPlan) ExecuteWithManifestContract(
	ctx context.Context,
	desc ManifestDescriptor,
	contract ToolManifestContract,
) (*PromptExecution, error) {
	exec, err := p.Execute(ctx)
	if err != nil {
		return nil, err
	}
	if err := ValidateExecutionManifestContract(exec, desc, contract); err != nil {
		return nil, err
	}
	return exec, nil
}
