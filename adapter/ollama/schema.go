package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// MapSchema preserves output schema objects as native Format JSON and accepts
// tool schemas only when every source field survives SDK serialization.
func (*Adapter) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if request.Profile.Provider != wireOllama {
		return prompty.SchemaMapping{}, schemaLoss("", "foreign schema profile")
	}
	if request.Role == prompty.SchemaOutput {
		if _, err := prompty.SchemaDocumentAsMap(request.Source); err != nil {
			return prompty.SchemaMapping{}, schemaLoss("", "native Format requires a JSON Schema object")
		}
		return prompty.SchemaMapping{Transformations: nil, Schema: prompty.CloneJSONDocument(request.Source)}, nil
	}
	if request.Role != prompty.SchemaToolInput {
		return prompty.SchemaMapping{}, adapter.ErrStructuredOutputNotSupported
	}
	if _, err := decodeToolSchema(request.Source); err != nil {
		return prompty.SchemaMapping{}, err
	}
	return prompty.SchemaMapping{Transformations: nil, Schema: prompty.CloneJSONDocument(request.Source)}, nil
}

func decodeToolSchema(source prompty.JSONDocument) (api.ToolFunctionParameters, error) {
	params := api.ToolFunctionParameters{Type: "object", Properties: api.NewToolPropertiesMap()}
	if len(source) == 0 {
		return params, nil
	}
	original, err := prompty.SchemaDocumentAsMap(source)
	if err != nil {
		return params, fmt.Errorf("%w: %w", adapter.ErrMalformedArgs, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	if decodeErr := decoder.Decode(&params); decodeErr != nil {
		return params, fmt.Errorf("%w: tool schema: %w", adapter.ErrMalformedArgs, decodeErr)
	}
	if params.Properties == nil {
		params.Properties = api.NewToolPropertiesMap()
	}
	serialized, err := json.Marshal(params)
	if err != nil {
		return params, err
	}
	actual, err := prompty.SchemaDocumentAsMap(serialized)
	if err != nil {
		return params, err
	}
	if pointer, changed := firstSchemaLoss(original, actual, ""); changed {
		return params, schemaLoss(pointer, "tool schema field is lost or changed by SDK mapping")
	}
	return params, nil
}

func schemaLoss(pointer, reason string) error {
	return &prompty.SchemaFidelityError{Pointer: pointer, Reason: reason}
}

// Empty SDK properties/default object describe the existing object-only tool-args
// contract. No other added fields are accepted, and every source field must survive.
func firstSchemaLoss(source, actual any, pointer string) (string, bool) {
	switch original := source.(type) {
	case map[string]any:
		mapped, ok := actual.(map[string]any)
		if !ok {
			return pointer, true
		}
		return firstMapSchemaLoss(original, mapped, pointer)
	case []any:
		mapped, ok := actual.([]any)
		if !ok || len(mapped) != len(original) {
			return pointer, true
		}
		for i := range original {
			if lost, changed := firstSchemaLoss(original[i], mapped[i], fmt.Sprintf("%s/%d", pointer, i)); changed {
				return lost, true
			}
		}
		return "", false
	default:
		return pointer, !reflect.DeepEqual(source, actual)
	}
}

func firstMapSchemaLoss(original, mapped map[string]any, pointer string) (string, bool) {
	for _, key := range schemaKeys(original) {
		path := schemaFieldPointer(pointer, key)
		value, exists := mapped[key]
		if !exists {
			return path, true
		}
		if lost, changed := firstSchemaLoss(original[key], value, path); changed {
			return lost, true
		}
	}
	for _, key := range schemaKeys(mapped) {
		value := mapped[key]
		if _, exists := original[key]; exists {
			continue
		}
		if pointer == "" && key == "type" && value == "object" {
			continue
		}
		if pointer == "" && key == "properties" {
			if object, ok := value.(map[string]any); ok && len(object) == 0 {
				continue
			}
		}
		return schemaFieldPointer(pointer, key), true
	}
	return "", false
}

func schemaKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func schemaFieldPointer(parent, key string) string {
	return parent + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
