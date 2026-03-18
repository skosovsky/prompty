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
	ID             string                   `yaml:"id"`
	Version        string                   `yaml:"version"`
	Description    string                   `yaml:"description"`
	ModelConfig    map[string]any           `yaml:"model_config"`
	Metadata       map[string]any           `yaml:"metadata"`
	InputSchema    map[string]any           `yaml:"input_schema"`
	Tools          []prompty.ToolDefinition `yaml:"tools"`
	ResponseFormat map[string]any           `yaml:"response_format"`
	Messages       []rawMessage             `yaml:"messages"`
}

// Parser implements manifest.Unmarshaler for YAML manifests.
type Parser struct{}

// New returns a parser for YAML manifests.
func New() *Parser {
	return &Parser{}
}

// normalizeValue converts map[any]any and nested structures to map[string]any recursively.
// Non-string keys in maps are silently dropped (safe for JSON Schema contract).
func normalizeValue(v any) any {
	switch x := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			if strKey, ok := k.(string); ok {
				m[strKey] = normalizeValue(val)
			}
		}
		return m
	case map[string]any:
		return normalizeMap(x)
	case []any:
		arr := make([]any, len(x))
		for i, val := range x {
			arr[i] = normalizeValue(val)
		}
		return arr
	default:
		return v
	}
}

// normalizeMap recursively normalizes values in a map (handles nested map[any]any in values).
func normalizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	res := make(map[string]any, len(m))
	for k, v := range m {
		res[k] = normalizeValue(v)
	}
	return res
}

// rawToSchemaDefinition builds SchemaDefinition from a raw map, supporting both flat and nested formats.
// Nested: {name, description, schema: {type, properties}} -> extracts name/desc, Schema from inner.
// Flat: {type, properties, required, ...} -> whole map is the JSON schema.
func rawToSchemaDefinition(raw map[string]any) *prompty.SchemaDefinition {
	if raw == nil {
		return nil
	}
	normalized := normalizeMap(raw)
	inner, hasSchema := normalized["schema"].(map[string]any)
	if hasSchema && inner != nil {
		name, _ := normalized["name"].(string)
		desc, _ := normalized["description"].(string)
		return &prompty.SchemaDefinition{Name: name, Description: desc, Schema: inner}
	}
	return &prompty.SchemaDefinition{Schema: normalized}
}

// Unmarshal parses YAML into manifest.RawManifest.
func (p *Parser) Unmarshal(in []byte, out any) error {
	var fm fileManifest
	if err := yaml.Unmarshal(in, &fm); err != nil {
		return fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	// Direct normalization (no casts needed; fileManifest fields are already map[string]any)
	fm.ModelConfig = normalizeMap(fm.ModelConfig)
	fm.Metadata = normalizeMap(fm.Metadata)
	for i := range fm.Tools {
		fm.Tools[i].Parameters = normalizeMap(fm.Tools[i].Parameters)
	}
	for i := range fm.Messages {
		fm.Messages[i].Metadata = normalizeMap(fm.Messages[i].Metadata)
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
	// rawToSchemaDefinition calls normalizeMap internally
	raw.InputSchema = rawToSchemaDefinition(fm.InputSchema)
	raw.ResponseFormat = rawToSchemaDefinition(fm.ResponseFormat)
	raw.Tools = fm.Tools
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
