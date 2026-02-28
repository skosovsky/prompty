package prompty

// ChatTemplateOption configures ChatPromptTemplate (functional options pattern).
type ChatTemplateOption func(*ChatPromptTemplate)

// WithPartialVariables sets default variables merged with payload (payload overrides).
func WithPartialVariables(vars map[string]any) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.PartialVariables = vars
	}
}

// WithTools sets tool definitions available in templates as .Tools.
func WithTools(tools []ToolDefinition) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.Tools = tools
	}
}

// WithConfig sets model config (e.g. temperature, max_tokens).
func WithConfig(config map[string]any) ChatTemplateOption {
	return func(c *ChatPromptTemplate) {
		c.ModelConfig = config
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

// WithRequiredVars sets explicit required variable names (e.g. from manifest variables.required).
// Merged with variables inferred from template content in FormatStruct.
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
