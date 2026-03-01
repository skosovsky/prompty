package prompty

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
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
	partialsGlob     string   // e.g. "_partials/*.tmpl" for ParseGlob
	partialsFS       struct { // for ParseFS (e.g. embed)
		fsys    fs.FS
		pattern string
	}
}

type parsedMessage struct {
	tpl      *template.Template
	role     Role
	optional bool
	metadata map[string]any // provider-specific; copied to ChatMessage on render
	vars     []string       // pre-computed from AST for optional-skip check
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
	root, err := template.New("root").Funcs(funcMap).Parse("")
	if err != nil {
		return nil, fmt.Errorf("%w: root: %w", ErrTemplateParse, err)
	}
	if tpl.partialsGlob != "" {
		root, err = root.ParseGlob(tpl.partialsGlob)
		if err != nil {
			return nil, fmt.Errorf("%w: partials glob %q: %w", ErrTemplateParse, tpl.partialsGlob, err)
		}
	}
	if tpl.partialsFS.fsys != nil {
		root, err = root.ParseFS(tpl.partialsFS.fsys, tpl.partialsFS.pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: partials fs %q: %w", ErrTemplateParse, tpl.partialsFS.pattern, err)
		}
	}
	tpl.parsedTemplates = make([]parsedMessage, 0, len(tpl.Messages))
	for i, m := range tpl.Messages {
		name := fmt.Sprintf("msg_%d", i)
		msgTmpl, err := root.New(name).Option("missingkey=error").Parse(m.Content)
		if err != nil {
			return nil, fmt.Errorf("%w: message %d: %w", ErrTemplateParse, i, err)
		}
		var meta map[string]any
		if len(m.Metadata) > 0 {
			meta = maps.Clone(m.Metadata)
		}
		tpl.parsedTemplates = append(tpl.parsedTemplates, parsedMessage{
			tpl: msgTmpl, role: m.Role, optional: m.Optional,
			metadata: meta,
			vars:     extractVarsFromTree(msgTmpl.Tree),
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
		RequiredVars:    slices.Clone(c.RequiredVars),
		requiredFromAST: c.requiredFromAST,
		Metadata:        c.Metadata,
		tokenCounter:    c.tokenCounter,
		parsedTemplates: c.parsedTemplates,
		partialsGlob:    c.partialsGlob,
		partialsFS:      c.partialsFS,
	}
	if c.ResponseFormat != nil {
		clonedFormat := &SchemaDefinition{
			Name:        c.ResponseFormat.Name,
			Description: c.ResponseFormat.Description,
		}
		if c.ResponseFormat.Schema != nil {
			clonedFormat.Schema = maps.Clone(c.ResponseFormat.Schema)
		}
		out.ResponseFormat = clonedFormat
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
		msgMeta := maps.Clone(pm.metadata)
		out = append(out, ChatMessage{
			Role:     pm.role,
			Content:  []ContentPart{TextPart{Text: text}},
			Metadata: msgMeta,
		})
	}
	out = spliceHistory(out, history)
	meta := c.Metadata
	meta.Tags = slices.Clone(meta.Tags)
	var clonedFormat *SchemaDefinition
	if c.ResponseFormat != nil {
		clonedFormat = &SchemaDefinition{
			Name:        c.ResponseFormat.Name,
			Description: c.ResponseFormat.Description,
		}
		if c.ResponseFormat.Schema != nil {
			clonedFormat.Schema = maps.Clone(c.ResponseFormat.Schema)
		}
	}
	return &PromptExecution{
		Messages:       out,
		Tools:          slices.Clone(c.Tools),
		ModelConfig:    maps.Clone(c.ModelConfig),
		Metadata:       meta,
		ResponseFormat: clonedFormat,
	}, nil
}

// ValidateVariables runs a dry-run execute with the given data (same merge as FormatStruct: PartialVariables + data + Tools).
// Returns an error with role/message index context if any template references a missing or invalid variable.
func (c *ChatPromptTemplate) ValidateVariables(data map[string]any) error {
	merged := maps.Clone(c.PartialVariables)
	if merged == nil {
		merged = make(map[string]any)
	}
	if data != nil {
		maps.Copy(merged, data)
	}
	merged["Tools"] = c.Tools
	for i, pm := range c.parsedTemplates {
		if pm.tpl == nil {
			return fmt.Errorf("%w: message %d", ErrTemplateParse, i)
		}
		if err := pm.tpl.Execute(io.Discard, merged); err != nil {
			return fmt.Errorf("%w: message %d (role %s): %w", ErrTemplateRender, i, pm.role, err)
		}
	}
	return nil
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
		if m.Role != RoleSystem && m.Role != RoleDeveloper {
			insertAt = i
			break
		}
		insertAt = i + 1
	}
	return slices.Concat(rendered[:insertAt], history, rendered[insertAt:])
}
