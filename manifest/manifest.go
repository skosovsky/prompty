package manifest

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/skosovsky/prompty"

	"gopkg.in/yaml.v3"
)

// fileManifest is the YAML manifest shape bound directly to domain types.
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
	Messages       []prompty.MessageTemplate `yaml:"messages"`
}

// ParseBytes parses a YAML manifest and returns a ChatPromptTemplate.
func ParseBytes(data []byte) (*prompty.ChatPromptTemplate, error) {
	var m fileManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	return buildTemplate(&m)
}

// ParseFile reads and parses a manifest file.
func ParseFile(path string) (*prompty.ChatPromptTemplate, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated by caller
	if err != nil {
		return nil, fmt.Errorf("manifest: read file: %w", err)
	}
	return ParseBytes(data)
}

// ParseFS reads and parses a manifest from fs.FS (e.g. embed.FS).
func ParseFS(fsys fs.FS, name string) (*prompty.ChatPromptTemplate, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("manifest: read fs: %w", err)
	}
	return ParseBytes(data)
}

func buildTemplate(m *fileManifest) (*prompty.ChatPromptTemplate, error) {
	if m.ID == "" {
		return nil, fmt.Errorf("%w: missing id", prompty.ErrInvalidManifest)
	}
	if len(m.Messages) == 0 {
		return nil, fmt.Errorf("%w: missing messages", prompty.ErrInvalidManifest)
	}
	validRoles := map[string]bool{
		string(prompty.RoleSystem):    true,
		string(prompty.RoleDeveloper): true,
		string(prompty.RoleUser):      true,
		string(prompty.RoleAssistant): true,
	}
	for i, msg := range m.Messages {
		if !validRoles[string(msg.Role)] {
			return nil, fmt.Errorf("%w: message %d: invalid role %q", prompty.ErrInvalidManifest, i, msg.Role)
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
	return prompty.NewChatPromptTemplate(m.Messages, opts...)
}
