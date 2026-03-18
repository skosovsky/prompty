// Package yaml implements manifest.Unmarshaler for YAML manifests.
package yaml

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

type rawContentPart struct {
	Type string `yaml:"type"`
	Text string `yaml:"text,omitempty"`
	URL  string `yaml:"url,omitempty"`
}

type rawContentSlice []rawContentPart

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

type rawMessage struct {
	Role     string          `yaml:"role"`
	Content  rawContentSlice `yaml:"content"`
	Optional bool            `yaml:"optional"`
	Cache    bool            `yaml:"cache,omitempty"`
	Metadata map[string]any  `yaml:"metadata,omitempty"`
}

type fileManifest struct {
	ID             string                    `yaml:"id"`
	Version        string                    `yaml:"version"`
	Description    string                    `yaml:"description"`
	ModelConfig    map[string]any            `yaml:"model_config"`
	Metadata       map[string]any            `yaml:"metadata"`
	InputSchema    *prompty.SchemaDefinition `yaml:"input_schema"`
	Tools          []prompty.ToolDefinition  `yaml:"tools"`
	ResponseFormat *prompty.SchemaDefinition `yaml:"response_format"`
	Messages       []rawMessage              `yaml:"messages"`
}

// Parser implements manifest.Unmarshaler for YAML manifests.
type Parser struct{}

// New returns a parser for YAML manifests.
func New() *Parser {
	return &Parser{}
}

// Unmarshal parses YAML into manifest.RawManifest.
func (p *Parser) Unmarshal(in []byte, out any) error {
	var fm fileManifest
	if err := yaml.Unmarshal(in, &fm); err != nil {
		return fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	raw, ok := out.(*manifest.RawManifest)
	if !ok {
		return fmt.Errorf("%w: out must be *manifest.RawManifest", prompty.ErrInvalidManifest)
	}
	raw.ID = fm.ID
	raw.Version = fm.Version
	raw.Description = fm.Description
	raw.ModelConfig = fm.ModelConfig
	raw.Metadata = fm.Metadata
	raw.InputSchema = fm.InputSchema
	raw.Tools = fm.Tools
	raw.ResponseFormat = fm.ResponseFormat
	raw.Messages = make([]manifest.RawMessage, len(fm.Messages))
	for i := range fm.Messages {
		m := &fm.Messages[i]
		raw.Messages[i] = manifest.RawMessage{
			Role:     m.Role,
			Optional: m.Optional,
			Cache:    m.Cache,
			Metadata: m.Metadata,
		}
		raw.Messages[i].Content = make([]manifest.RawContentPart, len(m.Content))
		for j := range m.Content {
			c := &m.Content[j]
			raw.Messages[i].Content[j] = manifest.RawContentPart{
				Type: c.Type,
				Text: c.Text,
				URL:  c.URL,
			}
		}
	}
	return nil
}
