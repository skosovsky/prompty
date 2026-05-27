package prompty

import (
	"context"
	"errors"
	"maps"
	"slices"
)

type renderContext struct {
	Input    any
	LateVars map[string]any
}

// RenderPlan is the deferred rendering contract for prompt execution.
type RenderPlan struct {
	template     *ChatPromptTemplate
	typedInput   any
	lateVars     map[string]any
	replacements map[string]*RenderPlan
}

func newRenderPlan(tpl *ChatPromptTemplate, typedInput any) *RenderPlan {
	return &RenderPlan{
		template:     CloneTemplate(tpl),
		typedInput:   typedInput,
		lateVars:     map[string]any{},
		replacements: map[string]*RenderPlan{},
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
		template:     CloneTemplate(p.template),
		typedInput:   p.typedInput,
		lateVars:     cloneMapAny(p.lateVars),
		replacements: cloneLayerReplacements(p.replacements),
	}
	maps.Copy(out.lateVars, vars)
	return out
}

// ReplaceLayer returns a copy of the plan with source layer replacement registered.
func (p *RenderPlan) ReplaceLayer(sourceID string, newLayer *RenderPlan) (*RenderPlan, error) {
	if p == nil {
		return nil, ErrNilRenderPlan
	}
	if sourceID == "" {
		return nil, errors.New("replace layer: sourceID is required")
	}
	if newLayer == nil {
		return nil, errors.New("replace layer: newLayer is required")
	}
	if err := validateReplacementCompatibility(p.template, sourceID, newLayer.template); err != nil {
		return nil, err
	}
	out := &RenderPlan{
		template:     CloneTemplate(p.template),
		typedInput:   p.typedInput,
		lateVars:     cloneMapAny(p.lateVars),
		replacements: cloneLayerReplacements(p.replacements),
	}
	out.replacements[sourceID] = cloneRenderPlan(newLayer)
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
	input, history, err := p.prepareExecutionInput()
	if err != nil {
		return nil, err
	}
	if inputMap, ok := input.(map[string]any); ok {
		if validateErr := validateRequiredInputVars(p.template, inputMap); validateErr != nil {
			return nil, validateErr
		}
	}
	ctxData := renderContext{
		Input:    input,
		LateVars: cloneMapAny(p.lateVars),
	}
	exec, err := p.template.formatContext(ctxData)
	if err != nil {
		return nil, err
	}
	exec.Messages = spliceHistory(exec.Messages, cloneMessages(history))
	if len(p.replacements) > 0 {
		replacementMessages := make(map[string][]ChatMessage, len(p.replacements))
		for sourceID, replacementPlan := range p.replacements {
			replacementExec, execErr := replacementPlan.Execute(ctx)
			if execErr != nil {
				return nil, execErr
			}
			replacementMessages[sourceID] = cloneMessages(replacementExec.Messages)
		}
		exec.Messages = applyLayerReplacements(exec.Messages, replacementMessages)
	}
	return exec.Normalize(), nil
}

func (p *RenderPlan) prepareExecutionInput() (any, []ChatMessage, error) {
	if p == nil {
		return nil, nil, ErrNilRenderPlan
	}
	if p.typedInput == nil {
		return nil, nil, nil
	}
	if inputMap, ok := p.typedInput.(map[string]any); ok {
		return cloneMapAny(inputMap), nil, nil
	}
	vars, history, err := getPayloadFields(p.typedInput)
	if err != nil {
		if errors.Is(err, ErrInvalidPayload) {
			// Non-struct typed input is passed through for explicit advanced use-cases.
			return p.typedInput, nil, nil
		}
		return nil, nil, err
	}
	return vars, history, nil
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

func cloneRenderPlan(in *RenderPlan) *RenderPlan {
	if in == nil {
		return nil
	}
	return &RenderPlan{
		template:     CloneTemplate(in.template),
		typedInput:   in.typedInput,
		lateVars:     cloneMapAny(in.lateVars),
		replacements: cloneLayerReplacements(in.replacements),
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
		sourceID := msg.SourceID
		if sourceID == "" {
			out = append(out, cloneChatMessage(msg))
			i++
			continue
		}
		repl, ok := replacements[sourceID]
		if !ok {
			out = append(out, cloneChatMessage(msg))
			i++
			continue
		}
		// Replace each contiguous source segment exactly once.
		out = append(out, cloneMessages(repl)...)
		i++
		for i < len(messages) && messages[i].SourceID == sourceID {
			i++
		}
	}
	return out
}

func validateReplacementCompatibility(
	base *ChatPromptTemplate,
	sourceID string,
	replacement *ChatPromptTemplate,
) error {
	if base == nil || replacement == nil {
		return errors.New("replace layer: both base and replacement templates are required")
	}

	var baseKinds []LayerKind
	for _, message := range base.Messages {
		if message.SourceID == sourceID {
			baseKinds = append(baseKinds, message.LayerKind)
		}
	}
	if len(baseKinds) == 0 {
		return errors.New("replace layer: sourceID was not found in base template")
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
