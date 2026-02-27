package prompty

import "context"

// Role is the message role in a chat (system, user, assistant, tool).
type Role string

// Chat message roles.
const (
	RoleSystem    Role = "system"
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
	Text string
}

func (TextPart) isContentPart() {}

// ImagePart holds image URL, MIME type, and optional inline data.
type ImagePart struct {
	URL      string
	MIMEType string
	Data     []byte
}

func (ImagePart) isContentPart() {}

// ToolCallPart represents an AI request to call a function (in assistant message).
type ToolCallPart struct {
	ID   string // Empty for models that do not support ID (e.g. base Gemini)
	Name string
	Args string // JSON string of arguments
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
	Messages    []ChatMessage
	Tools       []ToolDefinition
	ModelConfig map[string]any
	Metadata    PromptMetadata
}

// MessageTemplate is the raw template for one message before rendering.
// After FormatStruct it becomes a ChatMessage with substituted values.
// Optional: true skips the message if all referenced variables are zero-value.
type MessageTemplate struct {
	Role     Role   // RoleSystem, RoleUser, RoleAssistant
	Content  string // Go text/template: e.g. "Hello, {{ .user_name }}"
	Optional bool   // true → skip if all referenced variables are zero-value
}

// PromptRegistry returns a chat prompt template by name and environment.
type PromptRegistry interface {
	GetTemplate(ctx context.Context, name, env string) (*ChatPromptTemplate, error)
}
