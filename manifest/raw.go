package manifest

import "github.com/skosovsky/prompty"

// Unmarshaler parses raw manifest bytes into RawManifest.
// Implementations (e.g. parser/yaml) may use YAML, JSON, etc.
type Unmarshaler interface {
	Unmarshal(in []byte, out any) error
}

// RawContentPart is one content element of a message (format-agnostic).
type RawContentPart struct {
	Type         string                `json:"type"`
	Text         string                `json:"text,omitempty"`
	MediaType    string                `json:"media_type,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
	URL          string                `json:"url,omitempty"`
	CacheControl *prompty.CacheControl `json:"cache_control,omitempty"`
}

// RawMessage is the raw representation of a single message.
//
//nolint:golines // struct fields share aligned json/yaml tag layout
type RawMessage struct {
	Role           string                `json:"role"`
	LayerKind      prompty.LayerKind     `json:"layer_kind,omitempty" yaml:"layer_kind,omitempty"` //nolint:tagalign // golines-compatible struct tags
	LayerID        string                `json:"layer_id,omitempty" yaml:"layer_id,omitempty"`     //nolint:tagalign // golines-compatible struct tags
	LegacySourceID string                `json:"source_id,omitempty" yaml:"source_id,omitempty"`   //nolint:tagalign // legacy reject field
	Content        []RawContentPart      `json:"content"`
	Optional       bool                  `json:"optional"`
	CacheControl   *prompty.CacheControl `json:"cache_control,omitempty"`
	Metadata       map[string]any        `json:"metadata,omitempty"`
}

// RawManifest is the raw representation of a manifest, sufficient for buildTemplate.
// Supports Unmarshaler (YAML, JSON, etc.).
// InputSchema is the JSON Schema for input typing (prompty-gen, required/partial derivation).
// Metadata is the full metadata block; BuildFromRaw extracts tags and puts the rest into Extras.
type RawManifest struct {
	ID             string                    `json:"id"`
	Version        string                    `json:"version"`
	Description    string                    `json:"description"`
	LayerKind      prompty.LayerKind         `json:"layer_kind,omitempty"     yaml:"layer_kind,omitempty"`
	RequiredTools  []string                  `json:"required_tools,omitempty" yaml:"required_tools,omitempty"`
	ModelOptions   *prompty.ModelOptions     `json:"model_options"`
	Metadata       map[string]any            `json:"metadata"`
	InputSchema    *prompty.SchemaDefinition `json:"inputs"`
	Tools          []prompty.ToolDefinition  `json:"tools"`
	ResponseFormat *prompty.SchemaDefinition `json:"response_format"`
	Messages       []RawMessage              `json:"messages"`

	// Legacy v1 fields: strict-mode parser rejects manifests that still use them.
	LegacyModelConfig map[string]any            `json:"model_config,omitempty" yaml:"model_config,omitempty"`
	LegacyInputSchema *prompty.SchemaDefinition `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
}
