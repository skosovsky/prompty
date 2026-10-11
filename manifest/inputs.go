package manifest

import (
	"errors"
	"fmt"
	"maps"
	"sort"

	"github.com/skosovsky/prompty"
)

// DecodeInputs converts v2 manifest `inputs` block into SchemaDefinition.
// Supported forms:
//   - Contract style:
//     inputs:
//     user_name: { type: string, required: true, default: "Alice" }
//   - Schema wrapper style:
//     inputs:
//     name: ...
//     description: ...
//     schema: { type: object, properties: ... }
func DecodeInputs(raw map[string]any) (*prompty.SchemaDefinition, error) { //nolint:gocognit,funlen
	if raw == nil {
		//nolint:nilnil // nil inputs block is valid and represented as nil schema.
		return nil, nil
	}
	document, encodeErr := prompty.MapToJSONDocument(raw)
	if encodeErr != nil {
		return nil, fmt.Errorf("inputs: %w", encodeErr)
	}
	normalized, decodeErr := prompty.JSONDocumentAsMap(document)
	if decodeErr != nil {
		return nil, fmt.Errorf("inputs: %w", decodeErr)
	}
	if len(normalized) == 0 {
		schemaDoc, err := prompty.MapToJSONDocument(map[string]any{
			schemaTypeKey:       schemaObjectType,
			schemaPropertiesKey: map[string]any{},
		})
		if err != nil {
			return nil, err
		}
		return &prompty.SchemaDefinition{Dialect: "", Name: "", Description: "", Schema: schemaDoc}, nil
	}

	if _, hasWrapper := normalized["schema"].(map[string]any); hasWrapper {
		return nil, errors.New(
			"inputs must use contract-style fields (inputs.<field>), inputs.schema is not supported",
		)
	}
	if _, hasType := normalized[schemaTypeKey]; hasType {
		if _, hasProperties := normalized[schemaPropertiesKey].(map[string]any); hasProperties {
			return nil, errors.New(
				"inputs must use contract-style fields (inputs.<field>), raw JSON-schema format is not supported",
			)
		}
	}
	if _, hasProperties := normalized[schemaPropertiesKey].(map[string]any); hasProperties {
		return nil, errors.New(
			"inputs must use contract-style fields (inputs.<field>), raw JSON-schema format is not supported",
		)
	}

	properties := make(map[string]any, len(normalized))
	required := make([]string, 0)
	keys := make([]string, 0, len(normalized))
	for key := range normalized {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, inputName := range keys {
		specAny := normalized[inputName]
		spec, ok := specAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("inputs.%s must be an object", inputName)
		}
		prop := maps.Clone(spec)
		if requiredAny, hasRequired := prop["required"]; hasRequired {
			requiredBool, ok := requiredAny.(bool)
			if !ok {
				return nil, fmt.Errorf("inputs.%s.required must be boolean", inputName)
			}
			if requiredBool {
				required = append(required, inputName)
			}
			delete(prop, "required")
		}
		if formatAny, hasFormat := prop["format"]; hasFormat {
			formatStr, ok := formatAny.(string)
			if !ok {
				return nil, fmt.Errorf("inputs.%s.format must be string", inputName)
			}
			if formatStr == "messages" {
				if typ, _ := prop[schemaTypeKey].(string); typ != "array" {
					return nil, fmt.Errorf("inputs.%s.format messages requires type array", inputName)
				}
			}
		}
		if lateAny, hasLate := prop["late"]; hasLate {
			lateBool, ok := lateAny.(bool)
			if !ok {
				return nil, fmt.Errorf("inputs.%s.late must be boolean", inputName)
			}
			if lateBool {
				prop["x-prompty-late"] = true
			}
		}
		properties[inputName] = prop
	}

	schema := map[string]any{
		schemaTypeKey:       schemaObjectType,
		schemaPropertiesKey: properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	schemaDoc, err := prompty.MapToJSONDocument(schema)
	if err != nil {
		return nil, err
	}
	return &prompty.SchemaDefinition{Dialect: "", Name: "", Description: "", Schema: schemaDoc}, nil
}
