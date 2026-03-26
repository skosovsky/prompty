package manifest

import "github.com/skosovsky/prompty"

// Unmarshaler parses raw manifest bytes into RawManifest.
// Implementations (e.g. parser/yaml) may use YAML, JSON, etc.
type Unmarshaler interface {
	Unmarshal(in []byte, out any) error
}

// RawContentPart is one content element of a message (format-agnostic).
type RawContentPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	URL       string `json:"url,omitempty"`
}

// RawMessage is the raw representation of a single message.
type RawMessage struct {
	Role     string           `json:"role"`
	Content  []RawContentPart `json:"content"`
	Optional bool             `json:"optional"`
	Cache    bool             `json:"cache,omitempty"`
	Metadata map[string]any   `json:"metadata,omitempty"`
}

// RawManifest is the raw representation of a manifest, sufficient for buildTemplate.
// Supports Unmarshaler (YAML, JSON, etc.).
// InputSchema is the JSON Schema for input typing (prompty-gen, required/partial derivation).
// Metadata is the full metadata block; BuildFromRaw extracts tags and puts the rest into Extras.
type RawManifest struct {
	ID             string                    `json:"id"`
	Version        string                    `json:"version"`
	Description    string                    `json:"description"`
	ModelOptions   *prompty.ModelOptions     `json:"model_config"`
	Metadata       map[string]any            `json:"metadata"`
	InputSchema    *prompty.SchemaDefinition `json:"input_schema"`
	Tools          []prompty.ToolDefinition  `json:"tools"`
	ResponseFormat *prompty.SchemaDefinition `json:"response_format"`
	Messages       []RawMessage              `json:"messages"`
}
