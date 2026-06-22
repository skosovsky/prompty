package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
)

// RuntimeRecipeState describes whether a raw runtime checkpoint carries late input.
type RuntimeRecipeState string

const (
	RuntimeRecipeStateNoLate      RuntimeRecipeState = "no_late"
	RuntimeRecipeStateLateUnbound RuntimeRecipeState = "late_unbound"
	RuntimeRecipeStateLateBound   RuntimeRecipeState = "late_bound"
)

// RuntimeRecipeCheckpoint is a JSON-safe raw checkpoint for runtime recipe binding.
type RuntimeRecipeCheckpoint struct {
	Descriptor     ManifestDescriptor   `json:"descriptor"`
	Input          JSONDocument         `json:"input,omitempty"`
	LateInput      JSONDocument         `json:"late_input,omitempty"`
	State          RuntimeRecipeState   `json:"state"`
	RuntimeOptions PromptRuntimeOptions `json:"runtime_options,omitzero"`
}

// RuntimeOverlay carries late-bound runtime values applied over a checkpoint.
type RuntimeOverlay struct {
	LateInput JSONDocument  `json:"late_input,omitempty"`
	Values    ComposeValues `json:"-"`
}

// ToolScope is the runtime allowed/required tool boundary for a prompt execution.
type ToolScope struct {
	Required []ToolRequirement `json:"required,omitempty"`
	Allowed  []ToolManifest    `json:"allowed,omitempty"`
}

// BindRuntime verifies a checkpoint, applies runtime overlay/tool scope, and returns a materializable plan.
func BindRuntime(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
	checkpoint RuntimeRecipeCheckpoint,
	overlay RuntimeOverlay,
	scope ToolScope,
) (*RenderPlan, error) {
	if registry == nil {
		return nil, errors.New("runtime binding: registry is required")
	}
	if err := validateRuntimeCheckpoint(checkpoint); err != nil {
		return nil, err
	}
	if err := registry.VerifyManifestDescriptor(ctx, checkpoint.Descriptor); err != nil {
		return nil, err
	}
	plan, err := runtimePlanFromCheckpoint(ctx, registry, checkpoint, overlay)
	if err != nil {
		return nil, err
	}
	plan, err = bindRuntimeLateInput(plan, checkpoint, overlay)
	if err != nil {
		return nil, err
	}
	return bindRuntimeToolScope(plan, checkpoint.Descriptor, scope)
}

func runtimePlanFromCheckpoint(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
	checkpoint RuntimeRecipeCheckpoint,
	overlay RuntimeOverlay,
) (*RenderPlan, error) {
	inputVars, err := jsonDocumentObject(checkpoint.Input, "runtime checkpoint input")
	if err != nil {
		return nil, err
	}
	input := RegistryPlanInput{
		boundVars:     inputVars,
		chatHistory:   nil,
		composeValues: nil,
		composeSet:    false,
	}
	values, err := checkpoint.RuntimeOptions.composeValues()
	if err != nil {
		return nil, err
	}
	input = planInputWithComposeValues(input, values)
	if overlay.Values.IsSet() {
		input = planInputWithComposeValues(input, overlay.Values)
	}
	plan, err := registry.Plan(ctx, checkpoint.Descriptor.ID, input)
	if err != nil {
		return nil, fmt.Errorf("runtime binding: build render plan: %w", err)
	}
	return plan, nil
}

func bindRuntimeLateInput(
	plan *RenderPlan,
	checkpoint RuntimeRecipeCheckpoint,
	overlay RuntimeOverlay,
) (*RenderPlan, error) {
	lateDoc, err := mergeLateRuntimeDocuments(checkpoint.LateInput, overlay.LateInput)
	if err != nil {
		return nil, err
	}
	if checkpoint.State == RuntimeRecipeStateNoLate && len(lateDoc) > 0 {
		return nil, errors.New("runtime binding: late input provided for no-late checkpoint")
	}
	if checkpoint.State == RuntimeRecipeStateLateBound && len(lateDoc) == 0 {
		return nil, errors.New("runtime binding: checkpoint is marked late-bound but late input is missing")
	}
	if len(lateDoc) > 0 {
		plan, err = plan.WithLateInputJSON(lateDoc)
		if err != nil {
			return nil, fmt.Errorf("runtime binding: bind late input: %w", err)
		}
	}
	validateErr := plan.ValidateRuntimeBindings()
	if validateErr != nil {
		return nil, fmt.Errorf("runtime binding: validate late input: %w", validateErr)
	}
	return plan, nil
}

func bindRuntimeToolScope(
	plan *RenderPlan,
	desc ManifestDescriptor,
	scope ToolScope,
) (*RenderPlan, error) {
	return plan.withToolScope(scope, desc)
}

// ValidateToolScope verifies that required runtime tools are inside the allowed scope.
func ValidateToolScope(desc ManifestDescriptor, scope ToolScope) error {
	allowed := make(map[string]ToolManifest, len(scope.Allowed))
	for _, tool := range scope.Allowed {
		if tool.Name == "" {
			return toolContractErr(desc, "", "allowed tool name is required", ErrToolContractMismatch)
		}
		allowed[tool.Name] = tool
	}
	for _, req := range scope.Required {
		if req.Name == "" {
			return toolContractErr(desc, "", "required tool name is required", ErrToolContractMismatch)
		}
		tool, ok := allowed[req.Name]
		if !ok {
			return toolContractErr(desc, req.Name, "required tool is outside allowed scope", ErrMissingRequiredTool)
		}
		if err := validateToolManifestSatisfies(desc, req, tool); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeCheckpoint(checkpoint RuntimeRecipeCheckpoint) error {
	if err := validateRecipeDescriptor(checkpoint.Descriptor); err != nil {
		return fmt.Errorf("runtime binding: %w", err)
	}
	switch checkpoint.State {
	case RuntimeRecipeStateNoLate, RuntimeRecipeStateLateUnbound, RuntimeRecipeStateLateBound:
		return nil
	default:
		return fmt.Errorf("runtime binding: unsupported checkpoint state %q", checkpoint.State)
	}
}

func mergeLateRuntimeDocuments(baseDoc, overlayDoc JSONDocument) (JSONDocument, error) {
	base, err := jsonDocumentObject(baseDoc, "runtime checkpoint late input")
	if err != nil {
		return nil, err
	}
	overlay, err := jsonDocumentObject(overlayDoc, "runtime overlay late input")
	if err != nil {
		return nil, err
	}
	if len(base) == 0 && len(overlay) == 0 {
		return nil, nil
	}
	maps.Copy(base, overlay)
	return MarshalJSONDocument(base)
}

func jsonDocumentObject(doc JSONDocument, label string) (map[string]any, error) {
	if len(doc) == 0 {
		return make(map[string]any), nil
	}
	var out map[string]any
	if err := json.Unmarshal(doc, &out); err != nil {
		return nil, fmt.Errorf("%s: decode JSON object: %w", label, err)
	}
	if out == nil {
		return make(map[string]any), nil
	}
	return out, nil
}

func cloneToolScopePtr(scope *ToolScope) *ToolScope {
	if scope == nil {
		return nil
	}
	out := &ToolScope{
		Required: make([]ToolRequirement, len(scope.Required)),
		Allowed:  make([]ToolManifest, len(scope.Allowed)),
	}
	for i, req := range scope.Required {
		out.Required[i] = cloneToolRequirement(req)
	}
	for i, tool := range scope.Allowed {
		out.Allowed[i] = cloneToolManifest(tool)
	}
	return out
}
