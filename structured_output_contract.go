package prompty

import (
	"errors"
	"fmt"
)

// ErrStructuredOutputUnavailable indicates a prompt recipe has no structured output schema contract.
var ErrStructuredOutputUnavailable = errors.New("structured output contract: schema unavailable")

// StructuredOutputContract exposes a prompt output schema without rendering a fake prompt.
type StructuredOutputContract interface {
	ResponseFormat() (*SchemaDefinition, error)
	JSONSchema() (JSONDocument, error)
}

// ResponseFormatFromStruct returns a response format schema for T.
func ResponseFormatFromStruct[T any]() (*SchemaDefinition, error) {
	var zero T
	schemaMap, err := extractSchema(zero)
	if err != nil {
		return nil, fmt.Errorf("structured output contract: %w", err)
	}
	doc, err := MapToJSONDocument(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("structured output contract: %w", err)
	}
	return &SchemaDefinition{Schema: doc}, nil
}

// JSONSchemaFromStruct returns a JSON schema document for T.
func JSONSchemaFromStruct[T any]() (JSONDocument, error) {
	format, err := ResponseFormatFromStruct[T]()
	if err != nil {
		return nil, err
	}
	return CloneJSONDocument(format.Schema), nil
}

// NewStructuredOutputContract returns a static structured output contract.
func NewStructuredOutputContract(format *SchemaDefinition) StructuredOutputContract {
	return staticStructuredOutputContract{format: cloneSchemaDefinition(format)}
}

type staticStructuredOutputContract struct {
	format *SchemaDefinition
}

func (c staticStructuredOutputContract) ResponseFormat() (*SchemaDefinition, error) {
	if c.format == nil || len(c.format.Schema) == 0 {
		return nil, ErrStructuredOutputUnavailable
	}
	return cloneSchemaDefinition(c.format), nil
}

func (c staticStructuredOutputContract) JSONSchema() (JSONDocument, error) {
	format, err := c.ResponseFormat()
	if err != nil {
		return nil, err
	}
	return CloneJSONDocument(format.Schema), nil
}
