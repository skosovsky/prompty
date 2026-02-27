package manifest

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/skosovsky/prompty"
	"gopkg.in/yaml.v3"
)

// rawManifest is the YAML/JSON manifest structure before conversion to ChatPromptTemplate.
type rawManifest struct {
	ID          string         `yaml:"id"`
	Version     string         `yaml:"version"`
	Description string         `yaml:"description"`
	ModelConfig map[string]any `yaml:"model_config"`
	Metadata    rawMetadata    `yaml:"metadata"`
	Variables   rawVariables   `yaml:"variables"`
	Tools       []rawTool      `yaml:"tools"`
	Messages    []rawMessage   `yaml:"messages"`
}

type rawMetadata struct {
	Tags []string `yaml:"tags"`
}

type rawVariables struct {
	Required []string       `yaml:"required"`
	Partial  map[string]any `yaml:"partial"`
}

type rawTool struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Parameters  map[string]any `yaml:"parameters"`
}

type rawMessage struct {
	Role     string `yaml:"role"`
	Content  string `yaml:"content"`
	Optional bool   `yaml:"optional"`
}

// ParseBytes parses a YAML manifest and returns a ChatPromptTemplate.
func ParseBytes(data []byte) (*prompty.ChatPromptTemplate, error) {
	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", prompty.ErrInvalidManifest, err)
	}
	return convert(&raw)
}

// ParseFile reads and parses a manifest file.
func ParseFile(path string) (*prompty.ChatPromptTemplate, error) {
	data, err := os.ReadFile(path)
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

func convert(raw *rawManifest) (*prompty.ChatPromptTemplate, error) {
	if raw.ID == "" {
		return nil, fmt.Errorf("%w: missing id", prompty.ErrInvalidManifest)
	}
	if len(raw.Messages) == 0 {
		return nil, fmt.Errorf("%w: missing messages", prompty.ErrInvalidManifest)
	}
	validRoles := map[string]bool{
		string(prompty.RoleSystem):    true,
		string(prompty.RoleUser):      true,
		string(prompty.RoleAssistant): true,
	}
	messages := make([]prompty.MessageTemplate, 0, len(raw.Messages))
	for i, m := range raw.Messages {
		if !validRoles[m.Role] {
			return nil, fmt.Errorf("%w: message %d: invalid role %q", prompty.ErrInvalidManifest, i, m.Role)
		}
		messages = append(messages, prompty.MessageTemplate{
			Role:     prompty.Role(m.Role),
			Content:  m.Content,
			Optional: m.Optional,
		})
	}
	tools := make([]prompty.ToolDefinition, 0, len(raw.Tools))
	for _, t := range raw.Tools {
		tools = append(tools, prompty.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	opts := []prompty.ChatTemplateOption{
		prompty.WithMetadata(prompty.PromptMetadata{
			ID:          raw.ID,
			Version:     raw.Version,
			Description: raw.Description,
			Tags:        raw.Metadata.Tags,
		}),
	}
	if len(raw.Variables.Required) > 0 {
		opts = append(opts, prompty.WithRequiredVars(raw.Variables.Required))
	}
	if len(raw.Variables.Partial) > 0 {
		opts = append(opts, prompty.WithPartialVariables(raw.Variables.Partial))
	}
	if len(tools) > 0 {
		opts = append(opts, prompty.WithTools(tools))
	}
	if len(raw.ModelConfig) > 0 {
		opts = append(opts, prompty.WithConfig(raw.ModelConfig))
	}
	return prompty.NewChatPromptTemplate(messages, opts...)
}
