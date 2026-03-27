package prompty

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"text/template"
)

// maxRenderBufferCap is the maximum buffer capacity to return to the pool; larger buffers are dropped to avoid pool poisoning (OOM).
const maxRenderBufferCap = 64 * 1024

const (
	// partKindText is the canonical content kind for plain text template parts (message content and rendered output).
	partKindText = "text"
	// partKindMedia is the canonical content kind for media template parts.
	partKindMedia = "media"
)

var renderPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// ChatPromptTemplate holds message templates and options for rendering.
// Use NewChatPromptTemplate to construct; options are applied via ChatTemplateOption.
// Fields must not be mutated after construction to ensure goroutine safety.
type ChatPromptTemplate struct {
	Messages         []MessageTemplate
	PartialVariables map[string]any
	Tools            []ToolDefinition
	ModelOptions     *ModelOptions
	Metadata         PromptMetadata
	ResponseFormat   *SchemaDefinition // JSON Schema for structured output (passed to PromptExecution)
	InputSchema      *SchemaDefinition // JSON Schema for template input (prompty-gen, required/partial derivation)
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

type parsedPart struct {
	kind         string // "text" or "media"
	textTpl      *template.Template
	mediaTypeTpl *template.Template
	mimeTypeTpl  *template.Template
	urlTpl       *template.Template
	cacheControl *CacheControl
}

type parsedMessage struct {
	parts        []parsedPart
	role         Role
	optional     bool
	cacheControl *CacheControl
	metadata     map[string]any // provider-specific; copied to ChatMessage on render
	vars         []string       // pre-computed from all parts for optional-skip check
}

// NewChatPromptTemplate builds a template with defensive copies and applies options.
// Returns ErrTemplateParse if any message content fails to parse.
func NewChatPromptTemplate(
	messages []MessageTemplate,
	opts ...ChatTemplateOption,
) (*ChatPromptTemplate, error) {
	tpl := &ChatPromptTemplate{
		Messages: cloneMessageTemplates(messages),
	}
	for _, opt := range opts {
		opt(tpl)
	}
	if tpl.PartialVariables != nil {
		tpl.PartialVariables = maps.Clone(tpl.PartialVariables)
	}
	if tpl.Tools != nil {
		tpl.Tools = cloneToolDefinitions(tpl.Tools)
	}
	if tpl.RequiredVars != nil {
		tpl.RequiredVars = slices.Clone(tpl.RequiredVars)
	}
	tpl.ModelOptions = cloneModelOptions(tpl.ModelOptions)
	tpl.Metadata = clonePromptMetadata(tpl.Metadata)
	tpl.ResponseFormat = cloneSchemaDefinition(tpl.ResponseFormat)
	tpl.InputSchema = cloneSchemaDefinition(tpl.InputSchema)
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
			return nil, fmt.Errorf(
				"%w: partials glob %q: %w",
				ErrTemplateParse,
				tpl.partialsGlob,
				err,
			)
		}
	}
	if tpl.partialsFS.fsys != nil {
		root, err = root.ParseFS(tpl.partialsFS.fsys, tpl.partialsFS.pattern)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: partials fs %q: %w",
				ErrTemplateParse,
				tpl.partialsFS.pattern,
				err,
			)
		}
	}
	tpl.parsedTemplates = make([]parsedMessage, 0, len(tpl.Messages))
	for i, m := range tpl.Messages {
		var allVars []string
		parsedParts := make([]parsedPart, 0, len(m.Content))
		for j, part := range m.Content {
			namePrefix := fmt.Sprintf("msg_%d_part_%d", i, j)
			switch part.Type {
			case partKindText:
				textTpl, err := parsePartTemplate(root, namePrefix, part.Text)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d: %w",
						ErrTemplateParse,
						i,
						j,
						err,
					)
				}
				parsedParts = append(parsedParts, parsedPart{
					kind:         partKindText,
					textTpl:      textTpl,
					mediaTypeTpl: nil,
					mimeTypeTpl:  nil,
					urlTpl:       nil,
					cacheControl: cloneCacheControl(part.CacheControl),
				})
				allVars = append(allVars, extractVarsFromTree(textTpl.Tree)...)
			case partKindMedia:
				mediaTypeTpl, err := parsePartTemplate(
					root,
					namePrefix+"_media_type",
					part.MediaType,
				)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d media_type: %w",
						ErrTemplateParse,
						i,
						j,
						err,
					)
				}
				mimeTypeTpl, err := parsePartTemplate(root, namePrefix+"_mime_type", part.MIMEType)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d mime_type: %w",
						ErrTemplateParse,
						i,
						j,
						err,
					)
				}
				urlTpl, err := parsePartTemplate(root, namePrefix+"_url", part.URL)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d url: %w",
						ErrTemplateParse,
						i,
						j,
						err,
					)
				}
				parsedParts = append(parsedParts, parsedPart{
					kind:         partKindMedia,
					textTpl:      nil,
					mediaTypeTpl: mediaTypeTpl,
					mimeTypeTpl:  mimeTypeTpl,
					urlTpl:       urlTpl,
					cacheControl: cloneCacheControl(part.CacheControl),
				})
				allVars = append(allVars, extractVarsFromTree(mediaTypeTpl.Tree)...)
				allVars = append(allVars, extractVarsFromTree(mimeTypeTpl.Tree)...)
				allVars = append(allVars, extractVarsFromTree(urlTpl.Tree)...)
			default:
				return nil, fmt.Errorf(
					"%w: message %d part %d: unknown type %q",
					ErrTemplateParse,
					i,
					j,
					part.Type,
				)
			}
		}
		var meta map[string]any
		if len(m.Metadata) > 0 {
			meta = maps.Clone(m.Metadata)
		}
		tpl.parsedTemplates = append(tpl.parsedTemplates, parsedMessage{
			parts:        parsedParts,
			role:         m.Role,
			optional:     m.Optional,
			cacheControl: cloneCacheControl(m.CacheControl),
			metadata:     meta,
			vars:         allVars,
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
		Messages:        cloneMessageTemplates(c.Messages),
		Tools:           cloneToolDefinitions(c.Tools),
		RequiredVars:    slices.Clone(c.RequiredVars),
		requiredFromAST: c.requiredFromAST,
		Metadata:        clonePromptMetadata(c.Metadata),
		tokenCounter:    c.tokenCounter,
		parsedTemplates: c.parsedTemplates,
		partialsGlob:    c.partialsGlob,
		partialsFS:      c.partialsFS,
	}
	if c.ResponseFormat != nil {
		out.ResponseFormat = cloneSchemaDefinition(c.ResponseFormat)
	}
	if c.PartialVariables != nil {
		out.PartialVariables = maps.Clone(c.PartialVariables)
	}
	out.InputSchema = cloneSchemaDefinition(c.InputSchema)
	out.ModelOptions = cloneModelOptions(c.ModelOptions)
	return out
}

func (c *ChatPromptTemplate) renderTemplates(
	mergedVars map[string]any,
	history []ChatMessage,
) (*PromptExecution, error) {
	var out []ChatMessage
	for i, pm := range c.parsedTemplates {
		optionalSkip := pm.optional && allVarsZeroForMessage(mergedVars, pm.vars)
		if optionalSkip {
			continue
		}
		var contentParts []ContentPart
		for j, part := range pm.parts {
			switch part.kind {
			case partKindText:
				rendered, err := executeTemplateString(part.textTpl, mergedVars)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d: %w",
						ErrTemplateRender,
						i,
						j,
						err,
					)
				}
				contentParts = append(contentParts, TextPart{
					Text:         rendered,
					CacheControl: cloneCacheControl(part.cacheControl),
				})
			case partKindMedia:
				mediaType, err := executeTemplateString(part.mediaTypeTpl, mergedVars)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d media_type: %w",
						ErrTemplateRender,
						i,
						j,
						err,
					)
				}
				mimeType, err := executeTemplateString(part.mimeTypeTpl, mergedVars)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d mime_type: %w",
						ErrTemplateRender,
						i,
						j,
						err,
					)
				}
				url, err := executeTemplateString(part.urlTpl, mergedVars)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d url: %w",
						ErrTemplateRender,
						i,
						j,
						err,
					)
				}
				mediaType, err = normalizeMediaType(mediaType, mimeType)
				if err != nil {
					return nil, fmt.Errorf(
						"%w: message %d part %d media_type: %w",
						ErrTemplateRender,
						i,
						j,
						err,
					)
				}
				contentParts = append(contentParts, MediaPart{
					MediaType:    mediaType,
					MIMEType:     mimeType,
					URL:          url,
					CacheControl: cloneCacheControl(part.cacheControl),
				})
			default:
				return nil, fmt.Errorf(
					"%w: message %d part %d: unknown type %q",
					ErrTemplateRender,
					i,
					j,
					part.kind,
				)
			}
		}
		out = append(out, ChatMessage{
			Role:         pm.role,
			Content:      contentParts,
			CacheControl: cloneCacheControl(pm.cacheControl),
			Metadata:     maps.Clone(pm.metadata),
		})
	}
	out = spliceHistory(out, cloneMessages(history))
	return &PromptExecution{
		Messages:       out,
		Tools:          cloneToolDefinitions(c.Tools),
		ModelOptions:   cloneModelOptions(c.ModelOptions),
		Metadata:       clonePromptMetadata(c.Metadata),
		ResponseFormat: cloneSchemaDefinition(c.ResponseFormat),
	}, nil
}

// Format renders the template using the given input map (reflection-free).
// Same merge and validation as FormatStruct. History is not supported.
func (c *ChatPromptTemplate) Format(vars map[string]any) (*PromptExecution, error) {
	if vars == nil {
		vars = make(map[string]any)
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
			return nil, &VariableError{
				Variable: name,
				Template: c.Metadata.ID,
				Err:      ErrMissingVariable,
			}
		}
	}
	return c.renderTemplates(merged, nil)
}

// FormatStruct renders the template using payload struct (prompt tags), merges input fields, validates, splices history.
func (c *ChatPromptTemplate) FormatStruct(payload any) (*PromptExecution, error) {
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
			return nil, &VariableError{
				Variable: name,
				Template: c.Metadata.ID,
				Err:      ErrMissingVariable,
			}
		}
	}
	return c.renderTemplates(merged, history)
}

// ValidateVariables runs a dry-run execute with the given data (same merge as FormatStruct: PartialVariables + data + Tools).
// Returns an error with role/message index context if any template references a missing or invalid input field.
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
		for j, part := range pm.parts {
			for _, tmpl := range part.templates() {
				if tmpl == nil {
					continue
				}
				if err := tmpl.Execute(io.Discard, merged); err != nil {
					return fmt.Errorf(
						"%w: message %d part %d (role %s): %w",
						ErrTemplateRender,
						i,
						j,
						pm.role,
						err,
					)
				}
			}
			if part.kind != partKindMedia {
				continue
			}
			mediaType, err := executeTemplateString(part.mediaTypeTpl, merged)
			if err != nil {
				return fmt.Errorf(
					"%w: message %d part %d (role %s) media_type: %w",
					ErrTemplateRender,
					i,
					j,
					pm.role,
					err,
				)
			}
			mimeType, err := executeTemplateString(part.mimeTypeTpl, merged)
			if err != nil {
				return fmt.Errorf(
					"%w: message %d part %d (role %s) mime_type: %w",
					ErrTemplateRender,
					i,
					j,
					pm.role,
					err,
				)
			}
			if _, err := normalizeMediaType(mediaType, mimeType); err != nil {
				return fmt.Errorf(
					"%w: message %d part %d (role %s) media_type: %w",
					ErrTemplateRender,
					i,
					j,
					pm.role,
					err,
				)
			}
		}
	}
	return nil
}

func parsePartTemplate(root *template.Template, name, src string) (*template.Template, error) {
	return root.New(name).Option("missingkey=error").Parse(src)
}

func normalizeMediaType(mediaType, mimeType string) (string, error) {
	normalized := strings.TrimSpace(mediaType)
	if normalized != "" {
		return normalized, nil
	}
	inferredType, ok := inferMediaTypeFromMIME(mimeType)
	if !ok {
		return "", fmt.Errorf("media_type is required when mime_type %q is not inferable", mimeType)
	}
	return inferredType, nil
}

func inferMediaTypeFromMIME(mimeType string) (string, bool) {
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image", true
	case strings.HasPrefix(mime, "audio/"):
		return "audio", true
	case strings.HasPrefix(mime, "video/"):
		return "video", true
	case mime == "application/pdf":
		return "document", true
	default:
		return "", false
	}
}

func executeTemplateString(t *template.Template, vars map[string]any) (string, error) {
	if t == nil {
		return "", nil
	}
	rawBuf := renderPool.Get()
	buf, ok := rawBuf.(*bytes.Buffer)
	if !ok || buf == nil {
		buf = new(bytes.Buffer)
	}
	putBuffer := func() {
		if buf.Cap() <= maxRenderBufferCap {
			buf.Reset()
			renderPool.Put(buf)
		}
	}
	if err := t.Execute(buf, vars); err != nil {
		putBuffer()
		return "", err
	}
	out := buf.String()
	putBuffer()
	return out, nil
}

func (p parsedPart) templates() []*template.Template {
	switch p.kind {
	case partKindText:
		return []*template.Template{p.textTpl}
	case partKindMedia:
		return []*template.Template{p.mediaTypeTpl, p.mimeTypeTpl, p.urlTpl}
	default:
		return nil
	}
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
