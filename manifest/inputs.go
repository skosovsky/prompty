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
func DecodeInputs(raw map[string]any) (*prompty.SchemaDefinition, error) {
	if raw == nil {
		//nolint:nilnil // nil inputs block is valid and represented as nil schema.
		return nil, nil
	}
	normalized := normalizeMapAny(raw)
	if len(normalized) == 0 {
		//nolint:nilnil // empty inputs block is valid and represented as nil schema.
		return nil, nil
	}

	if _, hasWrapper := normalized["schema"].(map[string]any); hasWrapper {
		return nil, errors.New(
			"inputs must use contract-style fields (inputs.<field>), inputs.schema is not supported",
		)
	}
	if _, hasType := normalized["type"]; hasType {
		if _, hasProperties := normalized["properties"].(map[string]any); hasProperties {
			return nil, errors.New(
				"inputs must use contract-style fields (inputs.<field>), raw JSON-schema format is not supported",
			)
		}
	}
	if _, hasProperties := normalized["properties"].(map[string]any); hasProperties {
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
		properties[inputName] = prop
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return &prompty.SchemaDefinition{Schema: schema}, nil
}

func normalizeMapAny(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = normalizeAny(value)
	}
	return out
}

func normalizeAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeMapAny(typed)
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, val := range typed {
			if strKey, ok := key.(string); ok {
				normalized[strKey] = normalizeAny(val)
			}
		}
		return normalized
	case []any:
		items := make([]any, len(typed))
		for i := range typed {
			items[i] = normalizeAny(typed[i])
		}
		return items
	default:
		return value
	}
}
