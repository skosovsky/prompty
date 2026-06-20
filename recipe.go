package prompty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ManifestDescriptorVerifier verifies that a stored manifest descriptor still matches registry state.
type ManifestDescriptorVerifier interface {
	VerifyManifestDescriptor(ctx context.Context, desc ManifestDescriptor) error
}

// PromptRuntimeOptions stores runtime values that belong to the prompt contract.
type PromptRuntimeOptions struct {
	ComposeValues JSONDocument `json:"compose_values,omitempty"`
}

// PromptRuntimeOptionsFromComposeValues builds JSON-safe runtime options from typed compose values.
func PromptRuntimeOptionsFromComposeValues(values ComposeValues) (PromptRuntimeOptions, error) {
	if !values.IsSet() {
		return PromptRuntimeOptions{}, nil
	}
	doc, err := MapToJSONDocument(values.mapValue())
	if err != nil {
		return PromptRuntimeOptions{}, fmt.Errorf("prompt runtime options: %w", err)
	}
	return PromptRuntimeOptions{ComposeValues: doc}, nil
}

// PromptRuntimeOptionsFromComposeContext builds JSON-safe runtime options from a typed compose context.
func PromptRuntimeOptionsFromComposeContext(ctx ComposeContext) (PromptRuntimeOptions, error) {
	if ctx == nil {
		return PromptRuntimeOptions{}, nil
	}
	if validator, ok := ctx.(interface{ ValidateComposeContext() error }); ok {
		if err := validator.ValidateComposeContext(); err != nil {
			return PromptRuntimeOptions{}, err
		}
	}
	return PromptRuntimeOptionsFromComposeValues(ctx.ComposeValues())
}

func (o PromptRuntimeOptions) composeValues() (ComposeValues, error) {
	if len(o.ComposeValues) == 0 {
		return ComposeValues{}, nil
	}
	m, err := JSONDocumentAsMap(o.ComposeValues)
	if err != nil {
		return ComposeValues{}, fmt.Errorf("prompt runtime options: %w", err)
	}
	return newComposeValuesFromMap(m), nil
}

// PromptRecipeCheckpoint is a JSON-safe checkpoint DTO for a prompt recipe.
type PromptRecipeCheckpoint[TInput any, TLate any] struct {
	Descriptor     ManifestDescriptor   `json:"descriptor"`
	Input          TInput               `json:"input"`
	LateInput      *TLate               `json:"late_input,omitempty"`
	LateBound      bool                 `json:"late_bound"`
	RuntimeOptions PromptRuntimeOptions `json:"runtime_options,omitzero"`
}

// UnmarshalJSON rejects inconsistent checkpoint state and unknown checkpoint/input fields.
func (cp *PromptRecipeCheckpoint[TInput, TLate]) UnmarshalJSON(data []byte) error {
	type checkpoint PromptRecipeCheckpoint[TInput, TLate]
	var raw checkpoint
	if err := strictUnmarshalJSON(data, &raw); err != nil {
		return err
	}
	if err := validateCheckpointLateState(raw.LateBound, raw.LateInput); err != nil {
		return err
	}
	*cp = PromptRecipeCheckpoint[TInput, TLate](raw)
	return nil
}

// PromptRecipe is a serializable prompt recipe that can rebuild and execute a render plan.
type PromptRecipe[TInput any, TLate any] struct {
	Descriptor     ManifestDescriptor   `json:"descriptor"`
	Input          TInput               `json:"input"`
	LateInput      *TLate               `json:"late_input,omitempty"`
	LateBound      bool                 `json:"late_bound"`
	RuntimeOptions PromptRuntimeOptions `json:"runtime_options,omitzero"`
}

// NewPromptRecipe builds a prompt recipe from a manifest descriptor and early input.
func NewPromptRecipe[TInput any, TLate any](
	desc ManifestDescriptor,
	input TInput,
	options ...PromptRuntimeOptions,
) (PromptRecipe[TInput, TLate], error) {
	if err := validateRecipeDescriptor(desc); err != nil {
		return PromptRecipe[TInput, TLate]{}, err
	}
	return PromptRecipe[TInput, TLate]{
		Descriptor:     desc,
		Input:          input,
		LateInput:      nil,
		LateBound:      false,
		RuntimeOptions: mergePromptRuntimeOptions(options),
	}, nil
}

// PromptRecipeFromCheckpoint restores a recipe from its checkpoint DTO.
func PromptRecipeFromCheckpoint[TInput any, TLate any](
	cp PromptRecipeCheckpoint[TInput, TLate],
) (PromptRecipe[TInput, TLate], error) {
	recipe := PromptRecipe[TInput, TLate](cp)
	if err := recipe.validateState(); err != nil {
		return PromptRecipe[TInput, TLate]{}, err
	}
	return recipe, nil
}

// Checkpoint returns a JSON-safe checkpoint DTO.
func (r PromptRecipe[TInput, TLate]) Checkpoint() (PromptRecipeCheckpoint[TInput, TLate], error) {
	if err := r.validateState(); err != nil {
		return PromptRecipeCheckpoint[TInput, TLate]{}, err
	}
	return PromptRecipeCheckpoint[TInput, TLate](r), nil
}

// BindLate returns a copy of the recipe with typed late input attached.
func (r PromptRecipe[TInput, TLate]) BindLate(late TLate) (PromptRecipe[TInput, TLate], error) {
	if err := validateLatePayload(late); err != nil {
		return PromptRecipe[TInput, TLate]{}, err
	}
	r.LateInput = &late
	r.LateBound = true
	return r, nil
}

// WithComposeContext returns a copy of the recipe with typed compose context attached.
func (r PromptRecipe[TInput, TLate]) WithComposeContext(
	ctx ComposeContext,
) (PromptRecipe[TInput, TLate], error) {
	options, err := PromptRuntimeOptionsFromComposeContext(ctx)
	if err != nil {
		return PromptRecipe[TInput, TLate]{}, err
	}
	r.RuntimeOptions = options
	return r, nil
}

// Verify checks the stored descriptor against current registry state.
func (r PromptRecipe[TInput, TLate]) Verify(
	ctx context.Context,
	verifier ManifestDescriptorVerifier,
) error {
	if err := r.validateState(); err != nil {
		return err
	}
	if verifier == nil {
		return errors.New("prompt recipe: descriptor verifier is required")
	}
	return verifier.VerifyManifestDescriptor(ctx, r.Descriptor)
}

// Plan rebuilds the deferred render plan from stored early input.
func (r PromptRecipe[TInput, TLate]) Plan(
	ctx context.Context,
	registry Registry,
) (*RenderPlan, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	if registry == nil {
		return nil, errors.New("prompt recipe: registry is required")
	}
	input, err := registryPlanInputFromRecipe(r.Input, r.RuntimeOptions)
	if err != nil {
		return nil, err
	}
	plan, err := registry.Plan(ctx, r.Descriptor.ID, input)
	if err != nil {
		return nil, fmt.Errorf("prompt recipe: build render plan: %w", err)
	}
	return plan, nil
}

// Execute verifies, rebuilds, binds late input, and materializes a prompt execution.
func (r PromptRecipe[TInput, TLate]) Execute(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
) (*PromptExecution, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	plan, err := r.verifiedPlan(ctx, registry)
	if err != nil {
		return nil, err
	}
	return r.executePlan(ctx, plan)
}

// ExecuteWithContract verifies, rebuilds, binds late input, materializes, and checks required tools.
func (r PromptRecipe[TInput, TLate]) ExecuteWithContract(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
	contract ToolContract,
) (*PromptExecution, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	plan, err := r.verifiedPlan(ctx, registry)
	if err != nil {
		return nil, err
	}
	if r.LateBound {
		if r.LateInput == nil {
			return nil, errors.New("prompt recipe: late input is marked bound but missing")
		}
		plan, err = plan.WithLateInput(*r.LateInput)
		if err != nil {
			return nil, fmt.Errorf("prompt recipe: bind late input: %w", err)
		}
	}
	return plan.ExecuteWithContract(ctx, contract)
}

func (r PromptRecipe[TInput, TLate]) verifiedPlan(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
) (*RenderPlan, error) {
	if err := r.Verify(ctx, registry); err != nil {
		return nil, err
	}
	return r.Plan(ctx, registry)
}

func (r PromptRecipe[TInput, TLate]) executePlan(
	ctx context.Context,
	plan *RenderPlan,
) (*PromptExecution, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	if r.LateBound {
		if r.LateInput == nil {
			return nil, errors.New("prompt recipe: late input is marked bound but missing")
		}
		next, err := plan.WithLateInput(*r.LateInput)
		if err != nil {
			return nil, fmt.Errorf("prompt recipe: bind late input: %w", err)
		}
		plan = next
	}
	return plan.Execute(ctx)
}

func (r PromptRecipe[TInput, TLate]) validateState() error {
	if err := validateRecipeDescriptor(r.Descriptor); err != nil {
		return err
	}
	if err := validateCheckpointLateState(r.LateBound, r.LateInput); err != nil {
		return err
	}
	if r.LateBound {
		if err := validateLatePayload(*r.LateInput); err != nil {
			return err
		}
	}
	return nil
}

func validateCheckpointLateState[TLate any](lateBound bool, lateInput *TLate) error {
	if lateBound && lateInput == nil {
		return errors.New("prompt recipe: late input is marked bound but missing")
	}
	if !lateBound && lateInput != nil {
		return errors.New("prompt recipe: late input is present but not marked bound")
	}
	return nil
}

func strictUnmarshalJSON(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

// PromptRecipeNoLateCheckpoint is a JSON-safe checkpoint DTO for prompts without late input.
type PromptRecipeNoLateCheckpoint[TInput any] struct {
	Descriptor     ManifestDescriptor   `json:"descriptor"`
	Input          TInput               `json:"input"`
	RuntimeOptions PromptRuntimeOptions `json:"runtime_options,omitzero"`
}

// UnmarshalJSON rejects unknown checkpoint/input fields.
func (cp *PromptRecipeNoLateCheckpoint[TInput]) UnmarshalJSON(data []byte) error {
	type checkpoint PromptRecipeNoLateCheckpoint[TInput]
	var raw checkpoint
	if err := strictUnmarshalJSON(data, &raw); err != nil {
		return err
	}
	*cp = PromptRecipeNoLateCheckpoint[TInput](raw)
	return nil
}

// PromptRecipeNoLate is a recipe variant for prompts that do not declare late input.
type PromptRecipeNoLate[TInput any] struct {
	Descriptor     ManifestDescriptor   `json:"descriptor"`
	Input          TInput               `json:"input"`
	RuntimeOptions PromptRuntimeOptions `json:"runtime_options,omitzero"`
}

// NewPromptRecipeNoLate builds a prompt recipe for prompts without late input.
func NewPromptRecipeNoLate[TInput any](
	desc ManifestDescriptor,
	input TInput,
	options ...PromptRuntimeOptions,
) (PromptRecipeNoLate[TInput], error) {
	if err := validateRecipeDescriptor(desc); err != nil {
		return PromptRecipeNoLate[TInput]{}, err
	}
	return PromptRecipeNoLate[TInput]{
		Descriptor:     desc,
		Input:          input,
		RuntimeOptions: mergePromptRuntimeOptions(options),
	}, nil
}

// PromptRecipeNoLateFromCheckpoint restores a no-late recipe from its checkpoint DTO.
func PromptRecipeNoLateFromCheckpoint[TInput any](
	cp PromptRecipeNoLateCheckpoint[TInput],
) (PromptRecipeNoLate[TInput], error) {
	recipe := PromptRecipeNoLate[TInput](cp)
	if err := recipe.validateState(); err != nil {
		return PromptRecipeNoLate[TInput]{}, err
	}
	return recipe, nil
}

// Checkpoint returns a JSON-safe checkpoint DTO.
func (r PromptRecipeNoLate[TInput]) Checkpoint() (PromptRecipeNoLateCheckpoint[TInput], error) {
	if err := r.validateState(); err != nil {
		return PromptRecipeNoLateCheckpoint[TInput]{}, err
	}
	return PromptRecipeNoLateCheckpoint[TInput](r), nil
}

// WithComposeContext returns a copy of the recipe with typed compose context attached.
func (r PromptRecipeNoLate[TInput]) WithComposeContext(
	ctx ComposeContext,
) (PromptRecipeNoLate[TInput], error) {
	options, err := PromptRuntimeOptionsFromComposeContext(ctx)
	if err != nil {
		return PromptRecipeNoLate[TInput]{}, err
	}
	r.RuntimeOptions = options
	return r, nil
}

// Verify checks the stored descriptor against current registry state.
func (r PromptRecipeNoLate[TInput]) Verify(
	ctx context.Context,
	verifier ManifestDescriptorVerifier,
) error {
	if err := r.validateState(); err != nil {
		return err
	}
	if verifier == nil {
		return errors.New("prompt recipe: descriptor verifier is required")
	}
	return verifier.VerifyManifestDescriptor(ctx, r.Descriptor)
}

// Plan rebuilds the deferred render plan from stored early input.
func (r PromptRecipeNoLate[TInput]) Plan(
	ctx context.Context,
	registry Registry,
) (*RenderPlan, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	if registry == nil {
		return nil, errors.New("prompt recipe: registry is required")
	}
	input, err := registryPlanInputFromRecipe(r.Input, r.RuntimeOptions)
	if err != nil {
		return nil, err
	}
	plan, err := registry.Plan(ctx, r.Descriptor.ID, input)
	if err != nil {
		return nil, fmt.Errorf("prompt recipe: build render plan: %w", err)
	}
	return plan, nil
}

// Execute verifies, rebuilds, and materializes a prompt execution.
func (r PromptRecipeNoLate[TInput]) Execute(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
) (*PromptExecution, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	if err := r.Verify(ctx, registry); err != nil {
		return nil, err
	}
	plan, err := r.Plan(ctx, registry)
	if err != nil {
		return nil, err
	}
	return plan.Execute(ctx)
}

// ExecuteWithContract verifies, rebuilds, materializes, and checks required tools.
func (r PromptRecipeNoLate[TInput]) ExecuteWithContract(
	ctx context.Context,
	registry ManifestCheckpointRegistry,
	contract ToolContract,
) (*PromptExecution, error) {
	if err := r.validateState(); err != nil {
		return nil, err
	}
	if err := r.Verify(ctx, registry); err != nil {
		return nil, err
	}
	plan, err := r.Plan(ctx, registry)
	if err != nil {
		return nil, err
	}
	return plan.ExecuteWithContract(ctx, contract)
}

func (r PromptRecipeNoLate[TInput]) validateState() error {
	return validateRecipeDescriptor(r.Descriptor)
}

func mergePromptRuntimeOptions(options []PromptRuntimeOptions) PromptRuntimeOptions {
	if len(options) == 0 {
		return PromptRuntimeOptions{}
	}
	return options[len(options)-1]
}

func validateRecipeDescriptor(desc ManifestDescriptor) error {
	if desc.ID == "" {
		return errors.New("prompt recipe: descriptor id is required")
	}
	if desc.Digest == "" {
		return errors.New("prompt recipe: descriptor digest is required")
	}
	return nil
}

func validateLatePayload(input any) error {
	if input == nil {
		return errors.New("prompt recipe: late input is required")
	}
	if _, history, err := bindTemplateVars(input); err != nil {
		return fmt.Errorf("prompt recipe: bind late input: %w", err)
	} else if len(history) > 0 {
		return errors.New("prompt recipe: late input must not include chat history")
	}
	if _, _, ok := structTypeOf(input); ok {
		if err := validateLateInputStruct(input); err != nil {
			return fmt.Errorf("prompt recipe: late input: %w", err)
		}
	}
	return nil
}

func registryPlanInputFromRecipe(
	input any,
	options PromptRuntimeOptions,
) (RegistryPlanInput, error) {
	planInput, err := PlanInputFrom(input)
	if err != nil {
		if !isEmptyRecipeInput(input) {
			return RegistryPlanInput{}, fmt.Errorf("prompt recipe: bind input: %w", err)
		}
		planInput = RegistryPlanInput{
			boundVars:     nil,
			chatHistory:   nil,
			composeValues: nil,
			composeSet:    false,
		}
	}
	values, err := options.composeValues()
	if err != nil {
		return RegistryPlanInput{}, err
	}
	return planInputWithComposeValues(planInput, values), nil
}

func isEmptyRecipeInput(input any) bool {
	typ, _, ok := structTypeOf(input)
	if !ok {
		return false
	}
	return !structHasPromptFields(typ)
}
