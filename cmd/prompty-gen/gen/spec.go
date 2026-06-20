package gen

import "github.com/skosovsky/prompty"

// ComposeConditionSpec holds one generated typed compose context field.
type ComposeConditionSpec struct {
	Key       string
	FieldName string
	Kind      string
}

// PromptSpec holds the extracted specification for code generation.
type PromptSpec struct {
	ID                string
	Metadata          prompty.PromptMetadata
	Descriptor        prompty.ManifestDescriptor
	RequiredTools     []string
	InputSchema       *prompty.SchemaDefinition
	ResponseFormat    *prompty.SchemaDefinition
	ComposeConditions []ComposeConditionSpec
}
