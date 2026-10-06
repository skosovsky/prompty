package prompty

import (
	"encoding/json"
	"fmt"
)

// ValidateSchemaJSON validates a complete JSON value against an explicit local schema.
// References require resources declared inside the schema; no external IO is performed.
func ValidateSchemaJSON(definition *SchemaDefinition, document JSONDocument) error {
	if definition == nil {
		return nil
	}
	validator, err := NewJSONSchemaValidator(nil).CompileSchema(definition.Schema, definition.Dialect)
	if err != nil {
		return err
	}
	return validator.ValidateJSON(document)
}

// ValidateSchemaValue validates the JSON representation of a caller-owned value.
// Domain validation is a separate caller concern.
func ValidateSchemaValue(definition *SchemaDefinition, value any) error {
	if definition == nil {
		return nil
	}
	document, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("schema value: %w", err)
	}
	return ValidateSchemaJSON(definition, document)
}
