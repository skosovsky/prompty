package adapter

import "github.com/skosovsky/prompty"

// ProviderAdapter maps the canonical PromptExecution to a provider-specific request type
// and parses the provider response back to []ContentPart. No implementations in this package.
type ProviderAdapter interface {
	// Translate converts PromptExecution into the provider request payload (e.g. OpenAI chat params).
	// Callers must type-assert the result to the provider-specific type.
	Translate(exec *prompty.PromptExecution) (any, error)
	// ParseResponse converts the raw provider response into canonical content parts.
	ParseResponse(raw any) ([]prompty.ContentPart, error)
}
