package manifest

import (
	"fmt"
	"io/fs"
	"maps"
	"os"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/cast"
)

// metadataToPromptMetadata extracts known fields from raw metadata and builds PromptMetadata.
// Known typed fields: tags -> Tags, environment -> Environment. Unknown keys go to Extras.
func metadataToPromptMetadata(raw *RawManifest) prompty.PromptMetadata {
	meta := prompty.PromptMetadata{
		ID:          raw.ID,
		Version:     raw.Version,
		Description: raw.Description,
	}
	if raw.Metadata != nil {
		if tags, ok := raw.Metadata["tags"]; ok {
			if ss, ok := cast.ToStringSlice(tags); ok {
				meta.Tags = ss
			}
		}
		if env, ok := raw.Metadata["environment"]; ok {
			if s, ok := env.(string); ok {
				meta.Environment = s
			}
		}
		extras := make(map[string]any)
		for k, v := range raw.Metadata {
			if k != "tags" && k != "environment" && v != nil {
				extras[k] = v
			}
		}
		if len(extras) > 0 {
			meta.Extras = extras
		}
	}
	return meta
}

// ParseOption configures parsing (e.g. partials).
type ParseOption func(*parseOpts)

type parseOpts struct {
	partialsGlob      string
	partialsFS        fs.FS
	partialsFSPattern string
}

// WithPartialsGlob sets a glob for partials when loading from file (e.g. "_partials/*.tmpl").
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

// Parse parses the manifest using the given Unmarshaler and returns ChatPromptTemplate.
func Parse(data []byte, u Unmarshaler, opts ...ParseOption) (*prompty.ChatPromptTemplate, error) {
	if u == nil {
		return nil, prompty.ErrNoParser
	}
	var raw RawManifest
	if err := u.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	var po parseOpts
	for _, opt := range opts {
		opt(&po)
	}
	return BuildFromRaw(&raw, &po)
}

// BuildFromRaw builds ChatPromptTemplate from RawManifest (used by parsers and tests).
func BuildFromRaw(raw *RawManifest, po *parseOpts) (*prompty.ChatPromptTemplate, error) {
	if raw.ID == "" {
		return nil, fmt.Errorf("%w: missing id", prompty.ErrInvalidManifest)
	}
	if len(raw.Messages) == 0 {
		return nil, fmt.Errorf("%w: missing messages", prompty.ErrInvalidManifest)
	}
	messages := make([]prompty.MessageTemplate, len(raw.Messages))
	for i := range raw.Messages {
		rm := &raw.Messages[i]
		content := make([]prompty.TemplatePart, len(rm.Content))
		for j, p := range rm.Content {
			content[j] = prompty.TemplatePart{Type: p.Type, Text: p.Text, URL: p.URL}
		}
		messages[i] = prompty.MessageTemplate{
			Role:       prompty.Role(rm.Role),
			Content:    content,
			Optional:   rm.Optional,
			CachePoint: rm.Cache,
			Metadata:   maps.Clone(rm.Metadata),
		}
	}
	opts := []prompty.ChatTemplateOption{
		prompty.WithMetadata(metadataToPromptMetadata(raw)),
	}
	if raw.InputSchema != nil {
		opts = append(opts, prompty.WithInputSchema(raw.InputSchema))
		if schema := raw.InputSchema.Schema; schema != nil {
			if req, ok := schema["required"]; ok {
				if ss, ok := cast.ToStringSlice(req); ok && len(ss) > 0 {
					opts = append(opts, prompty.WithRequiredVars(ss))
				}
			}
			if props, _ := schema["properties"].(map[string]any); props != nil {
				partial := make(map[string]any)
				for k, v := range props {
					if m, ok := v.(map[string]any); ok && m["default"] != nil {
						partial[k] = m["default"]
					}
				}
				if len(partial) > 0 {
					opts = append(opts, prompty.WithPartialVariables(partial))
				}
			}
		}
	}
	if len(raw.Tools) > 0 {
		opts = append(opts, prompty.WithTools(raw.Tools))
	}
	if len(raw.ModelConfig) > 0 {
		opts = append(opts, prompty.WithConfig(raw.ModelConfig))
	}
	if raw.ResponseFormat != nil {
		opts = append(opts, prompty.WithResponseFormat(raw.ResponseFormat))
	}
	if po != nil && po.partialsGlob != "" {
		opts = append(opts, prompty.WithPartialsGlob(po.partialsGlob))
	}
	if po != nil && po.partialsFS != nil {
		opts = append(opts, prompty.WithPartialsFS(po.partialsFS, po.partialsFSPattern))
	}
	return prompty.NewChatPromptTemplate(messages, opts...)
}

// ParseFile reads the file and calls Parse.
func ParseFile(path string, u Unmarshaler, opts ...ParseOption) (*prompty.ChatPromptTemplate, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated by caller
	if err != nil {
		return nil, fmt.Errorf("manifest: read file: %w", err)
	}
	return Parse(data, u, opts...)
}

// ParseFS reads from fs.FS and calls Parse.
func ParseFS(fsys fs.FS, name string, u Unmarshaler, opts ...ParseOption) (*prompty.ChatPromptTemplate, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("manifest: read fs: %w", err)
	}
	return Parse(data, u, opts...)
}
