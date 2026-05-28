package prompty

import (
	"context"
	"errors"
	"maps"
	"reflect"
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

// NewRenderPlan builds a deferred render plan from template and typed input.
func NewRenderPlan(tpl *ChatPromptTemplate, typedInput any) *RenderPlan {
	return newRenderPlan(tpl, typedInput)
}

// Template returns a cloned underlying template for adapter registries.
func (p *RenderPlan) Template() *ChatPromptTemplate {
	if p == nil {
		return nil
	}
	return CloneTemplate(p.template)
}

// WithLateVariables returns a copy of the plan with merged late variables.
func (p *RenderPlan) WithLateVariables(vars map[string]any) *RenderPlan {
	if p == nil {
		return nil
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
	return out
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

// WithResponseFormat overrides response schema at runtime using a Go type or SchemaDefinition.
func (p *RenderPlan) WithResponseFormat(schema any) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	rf, err := schemaToDefinition(schema)
	if err != nil {
		return nil, err
	}
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
	if prepared.binding != nil {
		if validateErr := validateStructInputVars(
			p.template,
			prepared.structVal,
			prepared.binding,
		); validateErr != nil {
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
	binding   *structBinding
	structVal reflect.Value
	mergedMap map[string]any // set only when partial variables require map merge
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
		return &preparedInput{}, nil
	}
	if inputMap, ok := p.typedInput.(map[string]any); ok {
		return &preparedInput{
			input:     cloneMapAny(inputMap),
			history:   nil,
			binding:   nil,
			structVal: reflect.Value{},
			mergedMap: nil,
		}, nil
	}
	val, binding, history, err := extractStructPayload(p.typedInput)
	if err != nil {
		return nil, err
	}
	merged := buildStructTemplateInput(p.template, val, binding)
	return &preparedInput{
		input:     merged,
		history:   history,
		binding:   binding,
		structVal: val,
		mergedMap: merged,
	}, nil
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
