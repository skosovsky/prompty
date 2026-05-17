package gen

import "github.com/skosovsky/prompty"

// PromptSpec holds the extracted specification for code generation.
type PromptSpec struct {
	ID             string
	RequiredTools  []string
	InputSchema    *prompty.SchemaDefinition
	ResponseFormat *prompty.SchemaDefinition
}
