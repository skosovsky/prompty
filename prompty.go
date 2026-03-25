package prompty

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Role is the message role in a chat (system, developer, user, assistant, tool).
type Role string

// Chat message roles.
const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer" // Replaces system for OpenAI o1/o3-style models
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart is a sealed interface for message parts. Only package types implement it via isContentPart().
//
// Contract: All ProviderAdapter implementations of ParseResponse MUST return
// a []ContentPart slice containing only value types (e.g. TextPart, not *TextPart).
// Consumers can rely on this and need no defensive checks for pointer vs value.
type ContentPart interface {
	isContentPart()
}

// TextPart holds plain text content.
type TextPart struct {
	Text string
}

func (TextPart) isContentPart() {}

// MediaPart holds universal media (image, audio, video, document). URL or Data may be set.
// Adapters that do not accept URL natively may require callers to resolve URLs into inline data first.
type MediaPart struct {
	MediaType string // "image", "audio", "video", "document"
	MIMEType  string // e.g. "application/pdf", "image/jpeg"
	URL       string // Optional: link (adapters may fetch and convert to inline)
	Data      []byte // Optional: raw bytes (base64 is decoded by adapters as needed)
}

func (MediaPart) isContentPart() {}

// ReasoningPart is the hidden reasoning chain returned by some models (e.g. DeepSeek R1, OpenAI o-series).
type ReasoningPart struct {
	Text string
}

func (ReasoningPart) isContentPart() {}

// ToolCallPart represents an AI request to call a function (in assistant message).
// In streaming: ArgsChunk holds incremental JSON; Args is set in non-stream ParseResponse.
type ToolCallPart struct {
	ID        string // Empty for models that do not support ID (e.g. base Gemini)
	Name      string
	Args      string // Full JSON string of arguments (non-stream response)
	ArgsChunk string // Chunk of JSON arguments (streaming); client glues chunks
}

func (ToolCallPart) isContentPart() {}

// ToolResultPart is the result of a tool call (in message with Role "tool").
// Content is a slice of multimodal parts (text, images, etc.).
type ToolResultPart struct {
	ToolCallID string
	Name       string
	Content    []ContentPart
	IsError    bool
}

func (ToolResultPart) isContentPart() {}

// ChatMessage is a single message with role and content parts (supports multimodal).
// CachePoint hints providers to cache this message (e.g. Anthropic ephemeral). Other provider-specific options go in Metadata.
type ChatMessage struct {
	Role       Role
	Content    []ContentPart
	CachePoint bool           // When true, adapters may set cache_control / use context caching per provider
	Metadata   map[string]any // Provider-specific flags; adapters read known keys (e.g. gemini_search_grounding)
}

// ToolDefinition is the universal tool schema.
// JSON tags are required for template functions (e.g. render_tools_as_json) that marshal tools.
type ToolDefinition struct {
	Name        string         `json:"name"                 yaml:"name"`
	Description string         `json:"description"          yaml:"description"`
	Parameters  map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"` // JSON Schema for parameters
}

// SchemaDefinition describes a structured output (JSON Schema) for response format.
type SchemaDefinition struct {
	Name        string         `json:"name,omitempty"        yaml:"name,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Schema      map[string]any `json:"schema"                yaml:"schema"` // JSON Schema
}

// PromptMetadata holds observability metadata (v2.0 DTO).
// Known fields: ID, Version, Description, Tags, Environment.
// Extras holds arbitrary keys from manifest metadata block for tracing/custom middleware.
type PromptMetadata struct {
	ID          string         `json:"id"`
	Version     string         `json:"version,omitempty"`
	Description string         `json:"description,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Extras      map[string]any `json:"extras,omitempty"`
}

// ModelOptions holds typed, cross-provider model settings for one template/execution.
// ProviderSettings preserves provider-specific manifest keys without requiring generic SDK mapping.
type ModelOptions struct {
	Model            string         `json:"model,omitempty"             yaml:"model,omitempty"`
	Temperature      *float64       `json:"temperature,omitempty"       yaml:"temperature,omitempty"`
	MaxTokens        *int64         `json:"max_tokens,omitempty"        yaml:"max_tokens,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"             yaml:"top_p,omitempty"`
	Stop             []string       `json:"stop,omitempty"              yaml:"stop,omitempty"`
	ProviderSettings map[string]any `json:"provider_settings,omitempty" yaml:"provider_settings,omitempty"`
}

// PromptExecution is the result of formatting a template; immutable after creation.
type PromptExecution struct {
	Messages       []ChatMessage
	Tools          []ToolDefinition
	ModelOptions   *ModelOptions
	Metadata       PromptMetadata
	ResponseFormat *SchemaDefinition `json:"response_format,omitempty" yaml:"response_format,omitempty"`
}

// NewExecution creates a new prompt execution from a set of messages.
func NewExecution(messages []ChatMessage) *PromptExecution {
	return &PromptExecution{
		Messages: cloneMessages(messages),
	}
}

// WithHistory returns a new execution with cloned history messages appended.
func (e *PromptExecution) WithHistory(history []ChatMessage) *PromptExecution {
	if e == nil {
		return nil
	}
	messages := append(cloneMessages(e.Messages), cloneMessages(history)...)
	return cloneExecutionWithMessages(e, messages)
}

// AddMessage returns a new execution with one cloned message appended.
func (e *PromptExecution) AddMessage(msg ChatMessage) *PromptExecution {
	if e == nil {
		return nil
	}
	messages := append(cloneMessages(e.Messages), cloneChatMessage(msg))
	return cloneExecutionWithMessages(e, messages)
}

// Normalize returns a new PromptExecution with consecutive system/developer messages merged into one.
// Content is merged: TextPart texts are concatenated with "\n\n"; other parts (e.g. MediaPart) are preserved in order.
// Call explicitly when history may have produced adjacent system messages (e.g. to avoid provider 400).
func (e *PromptExecution) Normalize() *PromptExecution {
	if e == nil || len(e.Messages) == 0 {
		return e
	}
	var out []ChatMessage
	for i := 0; i < len(e.Messages); i++ {
		cur := e.Messages[i]
		if cur.Role != RoleSystem && cur.Role != RoleDeveloper {
			out = append(out, cloneChatMessage(cur))
			continue
		}
		// Merge all consecutive system/developer messages into one.
		merged := cloneChatMessage(cur)
		for j := i + 1; j < len(e.Messages) && (e.Messages[j].Role == RoleSystem || e.Messages[j].Role == RoleDeveloper); j++ {
			merged = mergeSystemMessages(merged, e.Messages[j])
			i = j
		}
		out = append(out, merged)
	}
	return &PromptExecution{
		Messages:       out,
		Tools:          cloneToolDefinitions(e.Tools),
		ModelOptions:   cloneModelOptions(e.ModelOptions),
		Metadata:       clonePromptMetadata(e.Metadata),
		ResponseFormat: cloneSchemaDefinition(e.ResponseFormat),
	}
}

// mergeSystemMessages merges two system/developer messages: text parts concatenated with "\n\n", other parts appended.
func mergeSystemMessages(a, b ChatMessage) ChatMessage {
	texts := textFromParts(a.Content)
	texts = append(texts, textFromParts(b.Content)...)
	var mergedText strings.Builder
	for i, t := range texts {
		if i > 0 {
			mergedText.WriteString("\n\n")
		}
		mergedText.WriteString(t)
	}
	content := make([]ContentPart, 0, 1+len(a.Content)+len(b.Content))
	if mergedText.String() != "" {
		content = append(content, TextPart{Text: mergedText.String()})
	}
	for _, p := range a.Content {
		if _, ok := p.(TextPart); !ok {
			content = append(content, cloneContentPart(p))
		}
	}
	for _, p := range b.Content {
		if _, ok := p.(TextPart); !ok {
			content = append(content, cloneContentPart(p))
		}
	}
	return ChatMessage{
		Role:       a.Role,
		Content:    content,
		CachePoint: a.CachePoint || b.CachePoint,
		Metadata:   cloneMapAny(a.Metadata),
	}
}

func textFromParts(parts []ContentPart) []string {
	var out []string
	for _, p := range parts {
		if t, ok := p.(TextPart); ok {
			out = append(out, t.Text)
		}
	}
	return out
}

// Fetcher defines how media URLs are resolved into raw bytes. Callers can use mediafetch.DefaultFetcher or provide a custom implementation (e.g. S3, local files).
type Fetcher interface {
	Fetch(ctx context.Context, url string) (data []byte, mimeType string, err error)
}

// ResolvedMedia returns a cloned execution where MediaParts with URL and empty Data are fetched via Fetcher.
// Only "image" media type is supported; other types with URL and empty Data return an error (fail-fast).
func (e *PromptExecution) ResolvedMedia(ctx context.Context, fetcher Fetcher) (*PromptExecution, error) {
	if e == nil {
		return nil, nil
	}
	out := e.Clone()
	for i, msg := range out.Messages {
		for j, part := range msg.Content {
			mp, ok := part.(MediaPart)
			if !ok {
				continue
			}
			if mp.URL == "" || len(mp.Data) > 0 {
				continue
			}
			if mp.MediaType != "image" {
				return nil, fmt.Errorf(
					"resolve media %s: currently only 'image' media type is supported for downloading, got %q",
					mp.URL,
					mp.MediaType,
				)
			}
			if isNilFetcher(fetcher) {
				return nil, fmt.Errorf("resolve media %s: %w", mp.URL, ErrNoFetcher)
			}
			data, contentType, err := fetcher.Fetch(ctx, mp.URL)
			if err != nil {
				return nil, fmt.Errorf("resolve media %s: %w", mp.URL, err)
			}
			mp.Data = data
			mp.MIMEType = contentType
			out.Messages[i].Content[j] = mp
		}
	}
	return out, nil
}

func isNilFetcher(fetcher Fetcher) bool {
	if fetcher == nil {
		return true
	}
	value := reflect.ValueOf(fetcher)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		return value.IsNil()
	default:
		return false
	}
}

// NewSystemMessage creates a single system message with text content.
func NewSystemMessage(text string) ChatMessage {
	return ChatMessage{
		Role:    RoleSystem,
		Content: []ContentPart{TextPart{Text: text}},
	}
}

// NewUserMessage creates a single user message with text content.
func NewUserMessage(text string) ChatMessage {
	return ChatMessage{
		Role:    RoleUser,
		Content: []ContentPart{TextPart{Text: text}},
	}
}

// NewAssistantMessage creates a single assistant message with text content.
func NewAssistantMessage(text string) ChatMessage {
	return ChatMessage{
		Role:    RoleAssistant,
		Content: []ContentPart{TextPart{Text: text}},
	}
}

func newAssistantMessageWithContent(content []ContentPart) ChatMessage {
	return ChatMessage{
		Role:    RoleAssistant,
		Content: cloneContentParts(content),
	}
}

func newToolMessageWithContent(content []ContentPart) ChatMessage {
	return ChatMessage{
		Role:    RoleTool,
		Content: cloneContentParts(content),
	}
}

func newToolResultPart(toolCallID, name, text string, isError bool) ToolResultPart {
	return ToolResultPart{
		ToolCallID: toolCallID,
		Name:       name,
		Content:    []ContentPart{TextPart{Text: text}},
		IsError:    isError,
	}
}

func newToolResultMessage(toolCallID, name, text string, isError bool) ChatMessage {
	return ChatMessage{
		Role: RoleTool,
		Content: []ContentPart{
			newToolResultPart(toolCallID, name, text, isError),
		},
	}
}

// TemplatePart is one part of a message template (text or image_url). Type determines which field (Text or URL) is the template source.
type TemplatePart struct {
	Type string // "text" or "image_url"
	Text string // Go text/template for type "text"
	URL  string // Go text/template for type "image_url"
}

// TextContent returns a single text TemplatePart slice for convenience.
func TextContent(text string) []TemplatePart {
	return []TemplatePart{{Type: "text", Text: text}}
}

// MessageTemplate is the raw template for one message before rendering.
// After FormatStruct it becomes a ChatMessage with substituted values.
// Optional: true skips the message if all referenced variables are zero-value.
// CachePoint maps from YAML cache: true; adapters use it for prompt caching (e.g. Anthropic ephemeral).
type MessageTemplate struct {
	Role       Role           // RoleSystem, RoleUser, RoleAssistant (and others; see Role* constants)
	Content    []TemplatePart // Parts to render (text and/or image_url); each part is a Go text/template
	Optional   bool           // true → skip if all referenced variables are zero-value
	CachePoint bool           // When true, request caching for this message where supported
	Metadata   map[string]any `yaml:"metadata,omitempty"`
}

// TemplateInfo holds metadata about a template without parsing its body.
type TemplateInfo struct {
	ID        string
	Version   string
	UpdatedAt time.Time
}

// Registry returns a chat prompt template by id.
// id is a single identifier (e.g. "doctor", "doctor.prod"); environments are expressed via file layout.
type Registry interface {
	GetTemplate(ctx context.Context, id string) (*ChatPromptTemplate, error)
}

// Lister is optional. When implemented by a registry, List returns available template ids.
type Lister interface {
	List(ctx context.Context) ([]string, error)
}

// Statter is optional. When implemented by a registry, Stat returns template metadata without parsing the body.
type Statter interface {
	Stat(ctx context.Context, id string) (TemplateInfo, error)
}
