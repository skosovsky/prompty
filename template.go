package prompty

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"text/template"
)

// ChatPromptTemplate holds message templates and options for rendering.
// Use NewChatPromptTemplate to construct; options are applied via ChatTemplateOption.
// Fields must not be mutated after construction to ensure goroutine safety.
type ChatPromptTemplate struct {
	Messages         []MessageTemplate
	PartialVariables map[string]any
	Tools            []ToolDefinition
	ModelConfig      map[string]any
	Metadata         PromptMetadata
	ResponseFormat   *SchemaDefinition // JSON Schema for structured output (passed to PromptExecution)
	RequiredVars     []string          // explicit required vars from manifest; merged with template-derived in FormatStruct
	requiredFromAST  []string          // pre-computed in constructor from non-optional message templates
	tokenCounter     TokenCounter
	parsedTemplates  []parsedMessage
}

type parsedMessage struct {
	tpl          *template.Template
	role         Role
	optional     bool
	cacheControl string
	vars         []string // pre-computed from AST for optional-skip check
}

// NewChatPromptTemplate builds a template with defensive copies and applies options.
// Returns ErrTemplateParse if any message content fails to parse.
func NewChatPromptTemplate(messages []MessageTemplate, opts ...ChatTemplateOption) (*ChatPromptTemplate, error) {
	tpl := &ChatPromptTemplate{
		Messages: slices.Clone(messages),
	}
	for _, opt := range opts {
		opt(tpl)
	}
	if tpl.PartialVariables != nil {
		tpl.PartialVariables = maps.Clone(tpl.PartialVariables)
	}
	if tpl.Tools != nil {
		tpl.Tools = slices.Clone(tpl.Tools)
	}
	if tpl.ModelConfig != nil {
		tpl.ModelConfig = maps.Clone(tpl.ModelConfig)
	}
	if tpl.RequiredVars != nil {
		tpl.RequiredVars = slices.Clone(tpl.RequiredVars)
	}
	tc := tpl.tokenCounter
	if tc == nil {
		tc = &CharFallbackCounter{}
	}
	funcMap := defaultFuncMap(tc)
	tpl.parsedTemplates = make([]parsedMessage, 0, len(tpl.Messages))
	for i, m := range tpl.Messages {
		parsed, err := template.New("").Funcs(funcMap).Parse(m.Content)
		if err != nil {
			return nil, fmt.Errorf("%w: message %d: %w", ErrTemplateParse, i, err)
		}
		tpl.parsedTemplates = append(tpl.parsedTemplates, parsedMessage{
			tpl: parsed, role: m.Role, optional: m.Optional,
			cacheControl: m.CacheControl,
			vars:         extractVarsFromTree(parsed.Tree),
		})
	}
	tpl.requiredFromAST = extractRequiredVarsFromParsed(tpl.parsedTemplates)
	return tpl, nil
}

// CloneTemplate returns a copy of the template with cloned slice and map fields.
// Registries use this so callers cannot mutate the cached template.
func CloneTemplate(c *ChatPromptTemplate) *ChatPromptTemplate {
	if c == nil {
		return nil
	}
	out := &ChatPromptTemplate{
		Messages:        slices.Clone(c.Messages),
		Tools:           slices.Clone(c.Tools),
		ResponseFormat:  c.ResponseFormat,
		RequiredVars:    slices.Clone(c.RequiredVars),
		requiredFromAST: c.requiredFromAST,
		Metadata:        c.Metadata,
		tokenCounter:    c.tokenCounter,
		parsedTemplates: c.parsedTemplates,
	}
	if c.PartialVariables != nil {
		out.PartialVariables = maps.Clone(c.PartialVariables)
	}
	if c.ModelConfig != nil {
		out.ModelConfig = maps.Clone(c.ModelConfig)
	}
	if len(c.Metadata.Tags) > 0 {
		out.Metadata.Tags = slices.Clone(c.Metadata.Tags)
	}
	return out
}

// FormatStruct renders the template using payload struct (prompt tags), merges variables, validates, splices history.
func (c *ChatPromptTemplate) FormatStruct(ctx context.Context, payload any) (*PromptExecution, error) {
	vars, history, err := getPayloadFields(payload)
	if err != nil {
		return nil, err
	}
	merged := maps.Clone(c.PartialVariables)
	if merged == nil {
		merged = make(map[string]any)
	}
	maps.Copy(merged, vars)
	merged["Tools"] = c.Tools
	required := mergeRequiredVars(c.RequiredVars, c.requiredFromAST)
	for _, name := range required {
		if _, ok := merged[name]; !ok {
			return nil, &VariableError{Variable: name, Template: c.Metadata.ID, Err: ErrMissingVariable}
		}
	}
	var out []ChatMessage
	for i, pm := range c.parsedTemplates {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if pm.tpl == nil {
			return nil, fmt.Errorf("%w: message %d", ErrTemplateParse, i)
		}
		optionalSkip := pm.optional && allVarsZeroForMessage(merged, pm.vars)
		if optionalSkip {
			continue
		}
		var buf bytes.Buffer
		if err := pm.tpl.Execute(&buf, merged); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrTemplateRender, err)
		}
		text := buf.String()
		out = append(out, ChatMessage{Role: pm.role, Content: []ContentPart{TextPart{Text: text, CacheControl: pm.cacheControl}}})
	}
	out = spliceHistory(out, history)
	meta := c.Metadata
	meta.Tags = slices.Clone(meta.Tags)
	return &PromptExecution{
		Messages:       out,
		Tools:          slices.Clone(c.Tools),
		ModelConfig:    maps.Clone(c.ModelConfig),
		Metadata:       meta,
		ResponseFormat: c.ResponseFormat,
	}, nil
}

// mergeRequiredVars returns unique names from explicit and template-derived, preserving order (explicit first).
func mergeRequiredVars(explicit, fromTemplates []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, name := range explicit {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, name := range fromTemplates {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func allVarsZeroForMessage(merged map[string]any, vars []string) bool {
	for _, name := range vars {
		v, ok := merged[name]
		if !ok {
			continue
		}
		if v == nil {
			continue
		}
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Func, reflect.Chan, reflect.UnsafePointer:
			return false
		default:
			if !rv.IsZero() {
				return false
			}
		}
	}
	return true
}

func spliceHistory(rendered []ChatMessage, history []ChatMessage) []ChatMessage {
	if len(history) == 0 {
		return rendered
	}
	insertAt := 0
	for i, m := range rendered {
		if m.Role != RoleSystem {
			insertAt = i
			break
		}
		insertAt = i + 1
	}
	return slices.Concat(rendered[:insertAt], history, rendered[insertAt:])
}
