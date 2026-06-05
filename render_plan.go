package prompty

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

type renderContext struct {
	Input    any
	LateVars map[string]any
}

// RenderPlan is the deferred rendering contract for prompt execution.
type RenderPlan struct {
	template       *ChatPromptTemplate
	typedInput     any
	lateVars       map[string]any
	replacements   map[string]*RenderPlan
	appends        map[string]*RenderPlan
	responseFormat *SchemaDefinition
}

func newRenderPlan(tpl *ChatPromptTemplate, typedInput any) *RenderPlan {
	return &RenderPlan{
		template:       CloneTemplate(tpl),
		typedInput:     typedInput,
		lateVars:       map[string]any{},
		replacements:   map[string]*RenderPlan{},
		appends:        map[string]*RenderPlan{},
		responseFormat: nil,
	}
}

// NewRenderPlan builds a deferred render plan with no template input (.Input is empty).
func NewRenderPlan(tpl *ChatPromptTemplate) *RenderPlan {
	return newRenderPlan(tpl, nil)
}

// NewRenderPlanFromMap builds a render plan from pre-shaped template variable maps.
// It does not run BindTemplateVars; use for tests or advanced callers that already
// have template keys shaped for .Input.
func NewRenderPlanFromMap(tpl *ChatPromptTemplate, vars map[string]any) *RenderPlan {
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
	return newRenderPlan(tpl, &boundPlanInput{
		vars:    cloneMapAny(input.boundVars),
		history: cloneMessages(input.chatHistory),
	}), nil
}

// Template returns a cloned underlying template for adapter registries.
func (p *RenderPlan) Template() *ChatPromptTemplate {
	if p == nil {
		return nil
	}
	return CloneTemplate(p.template)
}

// WithLateVariablesJSON returns a copy of the plan with merged late variables from a JSON object.
func (p *RenderPlan) WithLateVariablesJSON(doc JSONDocument) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	vars, err := JSONDocumentAsMap(doc)
	if err != nil {
		return nil, fmt.Errorf("late variables: %w", err)
	}
	out := &RenderPlan{
		template:       CloneTemplate(p.template),
		typedInput:     p.typedInput,
		lateVars:       cloneMapAny(p.lateVars),
		replacements:   cloneLayerReplacements(p.replacements),
		appends:        cloneLayerAppends(p.appends),
		responseFormat: cloneSchemaDefinition(p.responseFormat),
	}
	maps.Copy(out.lateVars, vars)
	return out, nil
}

// ReplaceLayer returns a copy of the plan with source layer replacement registered.
func (p *RenderPlan) ReplaceLayer(layerID string, newLayer *RenderPlan) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if layerID == "" {
		return nil, errors.New("replace layer: layerID is required")
	}
	if newLayer == nil {
		return nil, errors.New("replace layer: newLayer is required")
	}
	if err := validateReplacementCompatibility(p.template, layerID, newLayer.template); err != nil {
		return nil, err
	}
	out := &RenderPlan{
		template:       CloneTemplate(p.template),
		typedInput:     p.typedInput,
		lateVars:       cloneMapAny(p.lateVars),
		replacements:   cloneLayerReplacements(p.replacements),
		appends:        cloneLayerAppends(p.appends),
		responseFormat: cloneSchemaDefinition(p.responseFormat),
	}
	out.replacements[layerID] = cloneRenderPlan(newLayer)
	return out, nil
}

// AppendToLayer registers messages to append after the contiguous segment for layerID.
func (p *RenderPlan) AppendToLayer(layerID string, layerPlan *RenderPlan) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if layerID == "" {
		return nil, errors.New("append layer: layerID is required")
	}
	if layerPlan == nil {
		return nil, errors.New("append layer: layerPlan is required")
	}
	out := &RenderPlan{
		template:       CloneTemplate(p.template),
		typedInput:     p.typedInput,
		lateVars:       cloneMapAny(p.lateVars),
		replacements:   cloneLayerReplacements(p.replacements),
		appends:        cloneLayerAppends(p.appends),
		responseFormat: cloneSchemaDefinition(p.responseFormat),
	}
	out.appends[layerID] = cloneRenderPlan(layerPlan)
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
	out := &RenderPlan{
		template:       CloneTemplate(p.template),
		typedInput:     p.typedInput,
		lateVars:       cloneMapAny(p.lateVars),
		replacements:   cloneLayerReplacements(p.replacements),
		appends:        cloneLayerAppends(p.appends),
		responseFormat: rf,
	}
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
	ctxData := renderContext{
		Input:    prepared.templateInput(),
		LateVars: cloneMapAny(p.lateVars),
	}
	exec, err := p.template.formatContext(ctxData)
	if err != nil {
		return nil, err
	}
	exec.Messages = spliceHistory(exec.Messages, cloneMessages(prepared.history))
	if compErr := p.applyComposition(ctx, exec); compErr != nil {
		return nil, compErr
	}
	applyManifestProvenance(exec, p.template.Metadata.ID)
	return exec, nil
}

func (p *RenderPlan) applyComposition(ctx context.Context, exec *PromptExecution) error {
	if len(p.replacements) > 0 {
		replacementMessages := make(map[string][]ChatMessage, len(p.replacements))
		for layerID, replacementPlan := range p.replacements {
			replacementExec, execErr := replacementPlan.Execute(ctx)
			if execErr != nil {
				return execErr
			}
			replacementMessages[layerID] = cloneMessages(replacementExec.Messages)
		}
		exec.Messages = applyLayerReplacements(exec.Messages, replacementMessages)
	}
	if len(p.appends) > 0 {
		appendMessages := make(map[string][]ChatMessage, len(p.appends))
		for layerID, appendPlan := range p.appends {
			appendExec, appendErr := appendPlan.Execute(ctx)
			if appendErr != nil {
				return appendErr
			}
			appendMessages[layerID] = cloneMessages(appendExec.Messages)
		}
		exec.Messages = applyLayerAppends(exec.Messages, appendMessages)
	}
	if p.responseFormat != nil {
		exec.ResponseFormat = cloneSchemaDefinition(p.responseFormat)
	}
	return nil
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

func cloneLayerReplacements(in map[string]*RenderPlan) map[string]*RenderPlan {
	if in == nil {
		return map[string]*RenderPlan{}
	}
	out := make(map[string]*RenderPlan, len(in))
	for key, replacementPlan := range in {
		out[key] = cloneRenderPlan(replacementPlan)
	}
	return out
}

func cloneLayerAppends(in map[string]*RenderPlan) map[string]*RenderPlan {
	if in == nil {
		return map[string]*RenderPlan{}
	}
	out := make(map[string]*RenderPlan, len(in))
	for key, appendPlan := range in {
		out[key] = cloneRenderPlan(appendPlan)
	}
	return out
}

func cloneRenderPlan(in *RenderPlan) *RenderPlan {
	if in == nil {
		return nil
	}
	return &RenderPlan{
		template:       CloneTemplate(in.template),
		typedInput:     in.typedInput,
		lateVars:       cloneMapAny(in.lateVars),
		replacements:   cloneLayerReplacements(in.replacements),
		appends:        cloneLayerAppends(in.appends),
		responseFormat: cloneSchemaDefinition(in.responseFormat),
	}
}

func applyLayerAppends(messages []ChatMessage, appends map[string][]ChatMessage) []ChatMessage {
	if len(appends) == 0 {
		return messages
	}
	out := make([]ChatMessage, 0, len(messages))
	for i := 0; i < len(messages); {
		msg := messages[i]
		layerID := msg.LayerID
		if layerID == "" {
			out = append(out, cloneChatMessage(msg))
			i++
			continue
		}
		segment := []ChatMessage{cloneChatMessage(msg)}
		i++
		for i < len(messages) && messages[i].LayerID == layerID {
			segment = append(segment, cloneChatMessage(messages[i]))
			i++
		}
		out = append(out, segment...)
		if extra, ok := appends[layerID]; ok {
			out = append(out, cloneMessages(extra)...)
		}
	}
	return out
}

func applyManifestProvenance(exec *PromptExecution, manifestID string) {
	if exec == nil || manifestID == "" {
		return
	}
	for i := range exec.Messages {
		if exec.Messages[i].ManifestID == "" {
			exec.Messages[i].ManifestID = manifestID
		}
		if exec.Messages[i].LayerRef.ManifestID == "" {
			exec.Messages[i].LayerRef.ManifestID = manifestID
		}
		if exec.Messages[i].LayerID != "" && exec.Messages[i].LayerRef.LayerID == "" {
			exec.Messages[i].LayerRef.LayerID = exec.Messages[i].LayerID
		}
	}
}

func applyLayerReplacements(
	messages []ChatMessage,
	replacements map[string][]ChatMessage,
) []ChatMessage {
	if len(replacements) == 0 {
		return messages
	}
	out := make([]ChatMessage, 0, len(messages))
	for i := 0; i < len(messages); {
		msg := messages[i]
		layerID := msg.LayerID
		if layerID == "" {
			out = append(out, cloneChatMessage(msg))
			i++
			continue
		}
		repl, ok := replacements[layerID]
		if !ok {
			out = append(out, cloneChatMessage(msg))
			i++
			continue
		}
		// Replace each contiguous source segment exactly once.
		out = append(out, cloneMessages(repl)...)
		i++
		for i < len(messages) && messages[i].LayerID == layerID {
			i++
		}
	}
	return out
}

func validateReplacementCompatibility(
	base *ChatPromptTemplate,
	layerID string,
	replacement *ChatPromptTemplate,
) error {
	if base == nil || replacement == nil {
		return errors.New("replace layer: both base and replacement templates are required")
	}

	var baseKinds []LayerKind
	for _, message := range base.Messages {
		if message.LayerID == layerID {
			baseKinds = append(baseKinds, message.LayerKind)
		}
	}
	if len(baseKinds) == 0 {
		return errors.New("replace layer: layerID was not found in base template")
	}

	var replacementKinds []LayerKind
	for _, message := range replacement.Messages {
		if message.LayerKind != "" {
			replacementKinds = append(replacementKinds, message.LayerKind)
		}
	}
	replacementKinds = slices.Compact(replacementKinds)

	var requiredKinds []LayerKind
	for _, kind := range baseKinds {
		if kind != "" {
			requiredKinds = append(requiredKinds, kind)
		}
	}
	requiredKinds = slices.Compact(requiredKinds)
	if len(requiredKinds) == 0 || len(replacementKinds) == 0 {
		return nil
	}

	for _, replacementKind := range replacementKinds {
		if !slices.Contains(requiredKinds, replacementKind) {
			return errors.New(
				"replace layer: replacement layer kind is incompatible with source layer",
			)
		}
	}
	return nil
}
