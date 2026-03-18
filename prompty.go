package prompty

import (
	"context"
	"fmt"
	"maps"
	"slices"
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
// Adapters that do not accept URL natively may download the URL in Translate(ctx) and send inline data.
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
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema for parameters
}

// SchemaDefinition describes a structured output (JSON Schema) for response format.
type SchemaDefinition struct {
	Name        string         `json:"name,omitempty" yaml:"name,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Schema      map[string]any `json:"schema" yaml:"schema"` // JSON Schema
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

// PromptExecution is the result of formatting a template; immutable after creation.
type PromptExecution struct {
	Messages       []ChatMessage
	Tools          []ToolDefinition
	ModelConfig    map[string]any
	Metadata       PromptMetadata
	ResponseFormat *SchemaDefinition `json:"response_format,omitempty" yaml:"response_format,omitempty"`
}

// NewExecution creates a new prompt execution from a set of messages.
func NewExecution(messages []ChatMessage) *PromptExecution {
	return &PromptExecution{
		Messages: slices.Clone(messages),
	}
}

// WithHistory appends history messages after system/developer block. Clones Messages before append; returns e for chaining.
func (e *PromptExecution) WithHistory(history []ChatMessage) *PromptExecution {
	e.Messages = append(slices.Clone(e.Messages), history...)
	return e
}

// AddMessage appends one message. Clones Messages before append; returns e for chaining.
func (e *PromptExecution) AddMessage(msg ChatMessage) *PromptExecution {
	e.Messages = append(slices.Clone(e.Messages), msg)
	return e
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
			out = append(out, cur)
			continue
		}
		// Merge all consecutive system/developer messages into one.
		merged := cur
		for j := i + 1; j < len(e.Messages) && (e.Messages[j].Role == RoleSystem || e.Messages[j].Role == RoleDeveloper); j++ {
			merged = mergeSystemMessages(merged, e.Messages[j])
			i = j
		}
		out = append(out, merged)
	}
	meta := e.Metadata
	if meta.Tags != nil {
		meta.Tags = slices.Clone(meta.Tags)
	}
	if meta.Extras != nil {
		meta.Extras = maps.Clone(meta.Extras)
	}
	return &PromptExecution{
		Messages:       out,
		Tools:          e.Tools,
		ModelConfig:    e.ModelConfig,
		Metadata:       meta,
		ResponseFormat: e.ResponseFormat,
	}
}

// mergeSystemMessages merges two system/developer messages: text parts concatenated with "\n\n", other parts appended.
func mergeSystemMessages(a, b ChatMessage) ChatMessage {
	texts := textFromParts(a.Content)
	texts = append(texts, textFromParts(b.Content)...)
	mergedText := ""
	for i, t := range texts {
		if i > 0 {
			mergedText += "\n\n"
		}
		mergedText += t
	}
	content := make([]ContentPart, 0, 1+len(a.Content)+len(b.Content))
	if mergedText != "" {
		content = append(content, TextPart{Text: mergedText})
	}
	for _, p := range a.Content {
		if _, ok := p.(TextPart); !ok {
			content = append(content, p)
		}
	}
	for _, p := range b.Content {
		if _, ok := p.(TextPart); !ok {
			content = append(content, p)
		}
	}
	return ChatMessage{
		Role:       a.Role,
		Content:    content,
		CachePoint: a.CachePoint || b.CachePoint,
		Metadata:   a.Metadata,
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

// ResolveMedia fills Data and MIMEType for all MediaParts that have a URL but no Data, using the provided Fetcher.
// Only "image" media type is supported; other types with URL and empty Data return an error (fail-fast).
func (e *PromptExecution) ResolveMedia(ctx context.Context, fetcher Fetcher) error {
	for i, msg := range e.Messages {
		for j, part := range msg.Content {
			mp, ok := part.(MediaPart)
			if !ok {
				continue
			}
			if mp.URL == "" || len(mp.Data) > 0 {
				continue
			}
			if mp.MediaType != "image" {
				return fmt.Errorf("resolve media %s: currently only 'image' media type is supported for downloading, got %q", mp.URL, mp.MediaType)
			}
			data, contentType, err := fetcher.Fetch(ctx, mp.URL)
			if err != nil {
				return fmt.Errorf("resolve media %s: %w", mp.URL, err)
			}
			mp.Data = data
			mp.MIMEType = contentType
			e.Messages[i].Content[j] = mp
		}
	}
	return nil
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

// AppendValidationRetry appends to the dialogue messages about failed structured output validation
// (assistant with the raw model output and user with the error description) and returns the updated execution.
func (e *PromptExecution) AppendValidationRetry(badModelOutput string, validationError error) *PromptExecution {
	if e == nil {
		return e
	}

	// Build new messages using cloning AddMessage to avoid corrupting the existing slice.
	msgAssistant := NewAssistantMessage(badModelOutput)
	msgUser := NewUserMessage(fmt.Sprintf("JSON validation failed: %v. Please fix your output.", validationError))

	e = e.AddMessage(msgAssistant)
	e = e.AddMessage(msgUser)
	return e
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

// PromptRegistry is an alias for Registry for backward compatibility; prefer Registry.
type PromptRegistry = Registry
