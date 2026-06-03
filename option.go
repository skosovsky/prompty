package prompty

import (
	"fmt"
	"io/fs"
)

// ChatTemplateOption configures ChatPromptTemplate (functional options pattern).
type ChatTemplateOption func(*ChatPromptTemplate)

// WithPartialVariablesJSON sets default input values from a JSON object (payload overrides).
func WithPartialVariablesJSON(doc JSONDocument) (ChatTemplateOption, error) {
	vars, err := JSONDocumentAsMap(doc)
	if err != nil {
		return nil, fmt.Errorf("partial variables: %w", err)
	}
	return func(c *ChatPromptTemplate) {
		c.PartialVariables = vars
	}, nil
}

// MustWithPartialVariablesJSON is like WithPartialVariablesJSON but panics on decode error (tests).
func MustWithPartialVariablesJSON(doc JSONDocument) ChatTemplateOption {
	opt, err := WithPartialVariablesJSON(doc)
	if err != nil {
		panic(err)
	}
	return opt
}

// WithTools sets tool definitions available in templates as .Tools.
func WithTools(tools []ToolDefinition) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.Tools = tools
	}
}

// WithRequiredTools sets tool names the prompt contract requires (from manifest required_tools).
func WithRequiredTools(tools []string) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.RequiredTools = tools
	}
}

// WithModelOptions sets model options (e.g. temperature, max_tokens).
func WithModelOptions(config *ModelOptions) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.ModelOptions = config
	}
}

// WithMetadata sets prompt metadata for observability.
func WithMetadata(meta PromptMetadata) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.Metadata = meta
	}
}

// WithTokenCounter sets the token counter for truncate_tokens in templates.
func WithTokenCounter(tc TokenCounter) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.tokenCounter = tc
	}
}

// WithRequiredVars sets explicit required input field names (e.g. from manifest input_schema.required).
// Merged with input fields inferred from template content in RenderPlan.Execute.
func WithRequiredVars(vars []string) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.RequiredVars = vars
	}
}

// WithResponseFormat sets the JSON Schema for structured response format (used by OpenAI, Gemini).
func WithResponseFormat(schema *SchemaDefinition) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.ResponseFormat = schema
	}
}

// WithInputSchema sets the JSON Schema for template input (used by prompty-gen and manifest inputs contract).
func WithInputSchema(schema *SchemaDefinition) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.InputSchema = schema
	}
}

// WithPartialsGlob sets a glob pattern (e.g. "_partials/*.tmpl") to parse before message templates; enables {{ template "name" }}.
func WithPartialsGlob(glob string) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.partialsGlob = glob
	}
}

// WithPartialsFS sets an [fs.FS] and pattern for partials (e.g. embed FS and "partials/*.tmpl"); enables {{ template "name" }}.
func WithPartialsFS(fsys fs.FS, pattern string) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.partialsFS = struct {
			fsys    fs.FS
			pattern string
		}{fsys: fsys, pattern: pattern}
	}
}
