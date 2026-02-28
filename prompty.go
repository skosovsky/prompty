package prompty

import "context"

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
type ContentPart interface {
	isContentPart()
}

// TextPart holds plain text content.
type TextPart struct {
	Text         string
	CacheControl string // e.g. "ephemeral" for prompt caching (Anthropic)
}

func (TextPart) isContentPart() {}

// MediaPart holds universal media (image, audio, video, document). URL or Data may be set.
// Adapters that do not accept URL natively may download the URL in Translate(ctx) and send inline data.
type MediaPart struct {
	MediaType    string // "image", "audio", "video", "document"
	MIMEType     string // e.g. "application/pdf", "image/jpeg"
	URL          string // Optional: link (adapters may fetch and convert to inline)
	Data         []byte // Optional: raw bytes (base64 is decoded by adapters as needed)
	CacheControl string // e.g. "ephemeral" for prompt caching (Anthropic)
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
type ToolResultPart struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
}

func (ToolResultPart) isContentPart() {}

// ChatMessage is a single message with role and content parts (supports multimodal).
type ChatMessage struct {
	Role    Role
	Content []ContentPart
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

// PromptMetadata holds observability metadata.
type PromptMetadata struct {
	ID          string // From manifest id
	Version     string
	Description string   // From manifest description
	Tags        []string // From manifest metadata.tags
	Environment string   // Set by registry when loading by env (e.g. production); not from manifest
}

// PromptExecution is the result of formatting a template; immutable after creation.
type PromptExecution struct {
	Messages       []ChatMessage
	Tools          []ToolDefinition
	ModelConfig    map[string]any
	Metadata       PromptMetadata
	ResponseFormat *SchemaDefinition `json:"response_format,omitempty" yaml:"response_format,omitempty"`
}

// MessageTemplate is the raw template for one message before rendering.
// After FormatStruct it becomes a ChatMessage with substituted values.
// Optional: true skips the message if all referenced variables are zero-value.
type MessageTemplate struct {
	Role         Role   // RoleSystem, RoleUser, RoleAssistant
	Content      string // Go text/template: e.g. "Hello, {{ .user_name }}"
	Optional     bool   // true → skip if all referenced variables are zero-value
	CacheControl string `yaml:"cache_control"` // e.g. "ephemeral" for prompt caching (Anthropic)
}

// PromptRegistry returns a chat prompt template by name and environment.
type PromptRegistry interface {
	GetTemplate(ctx context.Context, name, env string) (*ChatPromptTemplate, error)
}
