package prompty

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/skosovsky/prompty/internal/late"
)

type renderContext struct {
	Input    any
	LateVars map[string]any
}

// RenderPlan is the deferred rendering contract for prompt execution.
type RenderPlan struct {
	template            *ChatPromptTemplate
	typedInput          any
	lateVars            map[string]any
	responseFormat      *SchemaDefinition
	toolScope           *ToolScope
	toolScopeDescriptor ManifestDescriptor
}

func newRenderPlan(tpl *ChatPromptTemplate, typedInput any) *RenderPlan {
	return &RenderPlan{
		template:            CloneTemplate(tpl),
		typedInput:          typedInput,
		lateVars:            map[string]any{},
		responseFormat:      nil,
		toolScope:           nil,
		toolScopeDescriptor: ManifestDescriptor{ID: "", Digest: ""},
	}
}

// NewRenderPlan builds a deferred render plan with no template input (.Input is empty).
func NewRenderPlan(tpl *ChatPromptTemplate) *RenderPlan {
	return newRenderPlan(tpl, nil)
}

// newRenderPlanFromMap builds a render plan from pre-shaped template variable maps.
// It does not run BindTemplateVars; test-only helper for callers that already
// have template keys shaped for .Input.
func newRenderPlanFromMap(tpl *ChatPromptTemplate, vars map[string]any) *RenderPlan {
	if vars == nil {
		return NewRenderPlan(tpl)
	}
	return newRenderPlan(tpl, cloneMapAny(vars))
}

// boundPlanInput holds template-bound vars and optional chat history for rendering.
type boundPlanInput struct {
	vars    map[string]any
	history []ChatMessage
}

// NewRenderPlanFromStruct builds a render plan from a struct payload (template .Input fields).
func NewRenderPlanFromStruct[T any](tpl *ChatPromptTemplate, input T) (*RenderPlan, error) {
	planInput, err := PlanInputFrom(input)
	if err != nil {
		return nil, err
	}
	return NewRenderPlanFromPlanInput(tpl, planInput)
}

// NewRenderPlanFromPlanInput builds a render plan from a bound registry payload.
func NewRenderPlanFromPlanInput(tpl *ChatPromptTemplate, input RegistryPlanInput) (*RenderPlan, error) {
	if tpl == nil {
		return nil, errors.New("render plan: template is nil")
	}
	if input.isEmpty() {
		return newRenderPlan(tpl, nil), nil
	}
	if err := validateEarlyInputFields(tpl.InputSchema, input.boundVars); err != nil {
		return nil, err
	}
	return newRenderPlan(tpl, &boundPlanInput{
		vars:    cloneMapAny(input.boundVars),
		history: cloneMessages(input.chatHistory),
	}), nil
}

// WithLateInputJSON returns a copy of the plan with late variables bound from a JSON object.
func (p *RenderPlan) WithLateInputJSON(doc JSONDocument) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	vars, err := jsonDocumentObject(doc, "late input")
	if err != nil {
		return nil, err
	}
	if err := validateLateInputFields(p.template.InputSchema, vars); err != nil {
		return nil, err
	}
	out := p.clone()
	if out.lateVars == nil {
		out.lateVars = make(map[string]any)
	}
	maps.Copy(out.lateVars, vars)
	return out, nil
}

// WithToolScope returns a copy of the plan with a runtime tool scope attached.
func (p *RenderPlan) WithToolScope(scope ToolScope) (*RenderPlan, error) {
	return p.withToolScope(scope, ManifestDescriptor{ID: "", Digest: ""})
}

func (p *RenderPlan) withToolScope(scope ToolScope, desc ManifestDescriptor) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if err := ValidateToolScope(desc, scope); err != nil {
		return nil, err
	}
	out := p.clone()
	out.toolScope = cloneToolScopePtr(&scope)
	out.toolScopeDescriptor = desc
	return out, nil
}

// Template returns a cloned underlying template for adapter registries.
func (p *RenderPlan) Template() *ChatPromptTemplate {
	if p == nil {
		return nil
	}
	return CloneTemplate(p.template)
}

// WithLateInput returns a copy of the plan with typed late variables bound from a struct (prompt tags).
func (p *RenderPlan) WithLateInput(input any) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if input == nil {
		return nil, errors.New("late input: payload is required")
	}
	vars, history, err := bindTemplateVars(input)
	if err != nil {
		return nil, fmt.Errorf("late input: %w", err)
	}
	if len(history) > 0 {
		return nil, errors.New("late input: chat history must not be bound as late variables")
	}
	if _, _, ok := structTypeOf(input); ok {
		if err := validateLateInputStruct(input); err != nil {
			return nil, fmt.Errorf("late input: %w", err)
		}
	}
	if err := validateLateInputFields(p.template.InputSchema, vars); err != nil {
		return nil, err
	}
	out := p.clone()
	if vars == nil {
		vars = make(map[string]any)
	}
	maps.Copy(out.lateVars, vars)
	return out, nil
}

// WithResponseFormatDefinition overrides response schema at runtime.
func (p *RenderPlan) WithResponseFormatDefinition(def *SchemaDefinition) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if def == nil {
		return nil, errors.New("response format schema is required")
	}
	rf := cloneSchemaDefinition(def)
	out := p.clone()
	out.responseFormat = rf
	return out, nil
}

// WithResponseFormatFromStruct overrides response schema using reflection/schema provider on T.
func WithResponseFormatFromStruct[T any](p *RenderPlan) (*RenderPlan, error) {
	var zero T
	schemaMap, err := extractSchema(zero)
	if err != nil {
		return nil, fmt.Errorf("response format: %w", err)
	}
	doc, err := MapToJSONDocument(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("response format: %w", err)
	}
	return p.WithResponseFormatDefinition(&SchemaDefinition{Schema: doc})
}

// Execute materializes plan into PromptExecution using `.Input` and `.LateVars` template context.
func (p *RenderPlan) Execute(ctx context.Context) (*PromptExecution, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := p.prepareExecutionInput()
	if err != nil {
		return nil, err
	}
	defer prepared.release()
	if inputMap, ok := prepared.input.(map[string]any); ok {
		if validateErr := validateRequiredInputVars(p.template, inputMap); validateErr != nil {
			return nil, validateErr
		}
	}
	templateID := ""
	if p.template != nil {
		templateID = p.template.Metadata.ID
	}
	lateVars := ensureOptionalLateDefaults(p.template.InputSchema, p.lateVars)
	if lateErr := validateRequiredLateVars(p.template.InputSchema, lateVars, templateID); lateErr != nil {
		return nil, lateErr
	}
	ctxData := renderContext{
		Input:    prepared.templateInput(),
		LateVars: cloneMapAny(lateVars),
	}
	exec, err := p.template.formatContext(ctxData)
	if err != nil {
		return nil, err
	}
	rendered := exec.Messages
	applyManifestProvenanceOnMessages(rendered, p.template.Metadata.ID)
	exec.Messages = spliceHistory(rendered, cloneMessages(prepared.history))
	p.applyRuntimeOverrides(exec)
	if p.toolScope != nil {
		desc := p.toolScopeDescriptor
		if desc.ID == "" {
			desc.ID = templateID
		}
		if err := ValidateExecutionManifestContract(exec, desc, toolScopeContract(*p.toolScope)); err != nil {
			return nil, err
		}
	}
	return exec, nil
}

// ValidateRuntimeBindings verifies runtime late bindings without invoking a model.
func (p *RenderPlan) ValidateRuntimeBindings() error {
	if p == nil {
		return ErrNilRenderPlan
	}
	templateID := ""
	if p.template != nil {
		templateID = p.template.Metadata.ID
	}
	lateVars := ensureOptionalLateDefaults(p.template.InputSchema, p.lateVars)
	return validateRequiredLateVars(p.template.InputSchema, lateVars, templateID)
}

func (p *RenderPlan) clone() *RenderPlan {
	if p == nil {
		return nil
	}
	out := &RenderPlan{
		template:            CloneTemplate(p.template),
		typedInput:          p.typedInput,
		lateVars:            cloneMapAny(p.lateVars),
		responseFormat:      cloneSchemaDefinition(p.responseFormat),
		toolScope:           cloneToolScopePtr(p.toolScope),
		toolScopeDescriptor: p.toolScopeDescriptor,
	}
	if out.lateVars == nil {
		out.lateVars = make(map[string]any)
	}
	return out
}

func (p *RenderPlan) applyRuntimeOverrides(exec *PromptExecution) {
	if p == nil || exec == nil || p.responseFormat == nil {
		return
	}
	exec.ResponseFormat = cloneSchemaDefinition(p.responseFormat)
}

type preparedInput struct {
	input     any
	history   []ChatMessage
	mergedMap map[string]any
}

func (pi *preparedInput) release() {
	if pi == nil || pi.mergedMap == nil {
		return
	}
	releaseBoundInputMap(pi.mergedMap)
	pi.mergedMap = nil
}

func (pi *preparedInput) templateInput() any {
	if pi == nil {
		return nil
	}
	return pi.input
}

func (p *RenderPlan) prepareExecutionInput() (*preparedInput, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if p.typedInput == nil {
		merged := mergeBoundVarsWithPartials(p.template, map[string]any{})
		return &preparedInput{
			input:     merged,
			history:   nil,
			mergedMap: merged,
		}, nil
	}
	if bound, ok := p.typedInput.(*boundPlanInput); ok {
		merged := mergeBoundVarsWithPartials(p.template, bound.vars)
		return &preparedInput{
			input:     merged,
			history:   bound.history,
			mergedMap: merged,
		}, nil
	}
	if inputMap, ok := p.typedInput.(map[string]any); ok {
		merged := mergeBoundVarsWithPartials(p.template, cloneMapAny(inputMap))
		return &preparedInput{
			input:     merged,
			history:   nil,
			mergedMap: merged,
		}, nil
	}
	return nil, ErrInvalidPayload
}

func validateRequiredInputVars(tpl *ChatPromptTemplate, input map[string]any) error {
	merged := maps.Clone(tpl.PartialVariables)
	if merged == nil {
		merged = make(map[string]any)
	}
	maps.Copy(merged, input)
	required := mergeRequiredVars(tpl.RequiredVars, tpl.requiredFromAST)
	if tpl.InputSchema != nil {
		doc, err := JSONDocumentAsMap(tpl.InputSchema.Schema)
		if err != nil {
			return fmt.Errorf("validate required input vars: %w", err)
		}
		props, _ := doc["properties"].(map[string]any)
		required = late.FilterEarlyRequired(required, props)
	}
	for _, name := range required {
		if _, ok := merged[name]; ok {
			continue
		}
		return &VariableError{
			Variable: name,
			Template: tpl.Metadata.ID,
			Err:      ErrMissingVariable,
		}
	}
	return nil
}

func applyManifestProvenanceOnMessages(messages []ChatMessage, manifestID string) {
	if manifestID == "" {
		return
	}
	for i := range messages {
		if messages[i].Provenance == nil {
			messages[i].Provenance = &MessageProvenance{ManifestID: manifestID, LayerID: ""}
			continue
		}
		if messages[i].Provenance.ManifestID == "" {
			messages[i].Provenance.ManifestID = manifestID
		}
	}
}
