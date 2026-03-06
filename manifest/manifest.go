package manifest

import (
	"fmt"
	"io/fs"
	"maps"
	"os"

	"github.com/skosovsky/prompty"

	"gopkg.in/yaml.v3"
)

// ParseOption configures parsing (e.g. partials for DRY templates).
type ParseOption func(*parseOpts)

type parseOpts struct {
	partialsGlob      string
	partialsFS        fs.FS
	partialsFSPattern string
}

// WithPartialsGlob sets a glob pattern (e.g. "_partials/*.tmpl") for template partials when loading from file.
func WithPartialsGlob(glob string) ParseOption {
	return func(o *parseOpts) { o.partialsGlob = glob }
}

// WithPartialsFS sets fs.FS and pattern for partials (e.g. embed and "partials/*.tmpl").
func WithPartialsFS(fsys fs.FS, pattern string) ParseOption {
	return func(o *parseOpts) {
		o.partialsFS = fsys
		o.partialsFSPattern = pattern
	}
}

// rawContentPart is one content part in YAML (type + text or url).
type rawContentPart struct {
	Type string `yaml:"type"`
	Text string `yaml:"text,omitempty"`
	URL  string `yaml:"url,omitempty"`
}

type rawContentSlice []rawContentPart

// UnmarshalYAML decodes content as either a scalar string (one text part) or a sequence of parts.
func (r *rawContentSlice) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*r = rawContentSlice{{Type: "text", Text: value.Value}}
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		var parts []rawContentPart
		for _, n := range value.Content {
			var p rawContentPart
			if err := n.Decode(&p); err != nil {
				return err
			}
			parts = append(parts, p)
		}
		*r = parts
		return nil
	}
	return fmt.Errorf("%w: content must be a string or array of parts", prompty.ErrInvalidManifest)
}

// rawMessage is the YAML shape for one message; cache: true maps to CachePoint.
type rawMessage struct {
	Role     string          `yaml:"role"`
	Content  rawContentSlice `yaml:"content"`
	Optional bool            `yaml:"optional"`
	Cache    bool            `yaml:"cache,omitempty"`
	Metadata map[string]any  `yaml:"metadata,omitempty"`
}

// fileManifest is the YAML manifest shape.
type fileManifest struct {
	ID          string                  `yaml:"id"`
	Version     string                  `yaml:"version"`
	Description string                  `yaml:"description"`
	ModelConfig map[string]any          `yaml:"model_config"`
	Metadata    struct{ Tags []string } `yaml:"metadata"`
	Variables   struct {
		Required []string       `yaml:"required"`
		Partial  map[string]any `yaml:"partial"`
	} `yaml:"variables"`
	Tools          []prompty.ToolDefinition  `yaml:"tools"`
	ResponseFormat *prompty.SchemaDefinition `yaml:"response_format"`
	Messages       []rawMessage              `yaml:"messages"`
}

// ParseBytes parses a YAML manifest and returns a ChatPromptTemplate.
func ParseBytes(data []byte) (*prompty.ChatPromptTemplate, error) {
	var m fileManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	return buildTemplate(&m, nil)
}

// ParseFile reads and parses a manifest file.
func ParseFile(path string) (*prompty.ChatPromptTemplate, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated by caller
	if err != nil {
		return nil, fmt.Errorf("manifest: read file: %w", err)
	}
	return ParseBytes(data)
}

// ParseFileWithOptions reads and parses a manifest file with options (e.g. WithPartialsGlob for partials).
func ParseFileWithOptions(path string, opts ...ParseOption) (*prompty.ChatPromptTemplate, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated by caller
	if err != nil {
		return nil, fmt.Errorf("manifest: read file: %w", err)
	}
	var m fileManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	var po parseOpts
	for _, opt := range opts {
		opt(&po)
	}
	return buildTemplate(&m, &po)
}

// ParseFS reads and parses a manifest from fs.FS (e.g. embed.FS).
func ParseFS(fsys fs.FS, name string) (*prompty.ChatPromptTemplate, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("manifest: read fs: %w", err)
	}
	return ParseBytes(data)
}

// ParseFSWithOptions reads and parses a manifest from fs.FS with options (e.g. WithPartialsFS for partials).
func ParseFSWithOptions(fsys fs.FS, name string, opts ...ParseOption) (*prompty.ChatPromptTemplate, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("manifest: read fs: %w", err)
	}
	var m fileManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	var po parseOpts
	for _, opt := range opts {
		opt(&po)
	}
	return buildTemplate(&m, &po)
}

func buildTemplate(m *fileManifest, po *parseOpts) (*prompty.ChatPromptTemplate, error) {
	if m.ID == "" {
		return nil, fmt.Errorf("%w: missing id", prompty.ErrInvalidManifest)
	}
	if len(m.Messages) == 0 {
		return nil, fmt.Errorf("%w: missing messages", prompty.ErrInvalidManifest)
	}
	// Convert raw messages to domain MessageTemplate; cache: true → CachePoint (no metadata key).
	messages := make([]prompty.MessageTemplate, len(m.Messages))
	for i := range m.Messages {
		raw := &m.Messages[i]
		content := make([]prompty.TemplatePart, len(raw.Content))
		for j, p := range raw.Content {
			content[j] = prompty.TemplatePart{Type: p.Type, Text: p.Text, URL: p.URL}
		}
		messages[i] = prompty.MessageTemplate{
			Role:       prompty.Role(raw.Role),
			Content:    content,
			Optional:   raw.Optional,
			CachePoint: raw.Cache,
			Metadata:   maps.Clone(raw.Metadata),
		}
	}
	opts := []prompty.ChatTemplateOption{
		prompty.WithMetadata(prompty.PromptMetadata{
			ID:          m.ID,
			Version:     m.Version,
			Description: m.Description,
			Tags:        m.Metadata.Tags,
		}),
	}
	if len(m.Variables.Required) > 0 {
		opts = append(opts, prompty.WithRequiredVars(m.Variables.Required))
	}
	if len(m.Variables.Partial) > 0 {
		opts = append(opts, prompty.WithPartialVariables(m.Variables.Partial))
	}
	if len(m.Tools) > 0 {
		opts = append(opts, prompty.WithTools(m.Tools))
	}
	if len(m.ModelConfig) > 0 {
		opts = append(opts, prompty.WithConfig(m.ModelConfig))
	}
	if m.ResponseFormat != nil {
		opts = append(opts, prompty.WithResponseFormat(m.ResponseFormat))
	}
	if po != nil && po.partialsGlob != "" {
		opts = append(opts, prompty.WithPartialsGlob(po.partialsGlob))
	}
	if po != nil && po.partialsFS != nil {
		opts = append(opts, prompty.WithPartialsFS(po.partialsFS, po.partialsFSPattern))
	}
	return prompty.NewChatPromptTemplate(messages, opts...)
}
