package gemini

import (
	"fmt"

	"google.golang.org/genai"
)

// mapToGenaiSchema converts a JSON Schema (map[string]any) to genai.Schema.
// Handles type, properties, items, required. Recursive for nested objects and arrays.
func mapToGenaiSchema(m map[string]any) (*genai.Schema, error) {
	if m == nil {
		return nil, nil //nolint:nilnil // An absent JSON schema maps to no provider schema.
	}
	s := &genai.Schema{}
	if t, ok := m["type"].(string); ok && t != "" {
		s.Type = jsonSchemaTypeToGenai(t)
	}
	if properties, ok := m["properties"].(map[string]any); ok {
		converted, err := genaiSchemaProperties(properties)
		if err != nil {
			return nil, err
		}
		s.Properties = converted
	}
	if r, ok := m["required"].([]any); ok {
		s.Required = schemaStringValues(r)
	} else if r, ok := m["required"].([]string); ok {
		s.Required = r
	}
	if items, ok := m["items"].(map[string]any); ok {
		converted, err := mapToGenaiSchema(items)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		if converted != nil {
			s.Items = converted
		}
	}
	if desc, ok := m["description"].(string); ok {
		s.Description = desc
	}
	if enum, ok := m["enum"].([]any); ok {
		s.Enum = schemaStringValues(enum)
	}
	return s, nil
}

func jsonSchemaTypeToGenai(t string) genai.Type {
	switch t {
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case "object":
		return genai.TypeObject
	default:
		return genai.TypeUnspecified
	}
}

func genaiSchemaProperties(properties map[string]any) (map[string]*genai.Schema, error) {
	out := make(map[string]*genai.Schema)
	for key, value := range properties {
		schema, ok := value.(map[string]any)
		if !ok {
			continue
		}
		converted, err := mapToGenaiSchema(schema)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", key, err)
		}
		if converted != nil {
			out[key] = converted
		}
	}
	return out, nil
}

func schemaStringValues(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
