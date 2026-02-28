package gemini

import (
	"fmt"

	"google.golang.org/genai"
)

// mapToGenaiSchema converts a JSON Schema (map[string]any) to genai.Schema.
// Handles type, properties, items, required. Recursive for nested objects and arrays.
func mapToGenaiSchema(m map[string]any) (*genai.Schema, error) {
	if m == nil {
		return nil, nil
	}
	s := &genai.Schema{}
	if t, ok := m["type"].(string); ok && t != "" {
		s.Type = jsonSchemaTypeToGenai(t)
	}
	if p, ok := m["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*genai.Schema)
		for k, v := range p {
			sub, ok := v.(map[string]any)
			if !ok {
				continue
			}
			conv, err := mapToGenaiSchema(sub)
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", k, err)
			}
			if conv != nil {
				s.Properties[k] = conv
			}
		}
	}
	if r, ok := m["required"].([]any); ok {
		required := make([]string, 0, len(r))
		for _, x := range r {
			if str, ok := x.(string); ok {
				required = append(required, str)
			}
		}
		s.Required = required
	} else if r, ok := m["required"].([]string); ok {
		s.Required = r
	}
	if items, ok := m["items"]; ok {
		sub, ok := items.(map[string]any)
		if ok {
			conv, err := mapToGenaiSchema(sub)
			if err != nil {
				return nil, fmt.Errorf("items: %w", err)
			}
			if conv != nil {
				s.Items = conv
			}
		}
	}
	if desc, ok := m["description"].(string); ok {
		s.Description = desc
	}
	if enum, ok := m["enum"].([]any); ok {
		strs := make([]string, 0, len(enum))
		for _, e := range enum {
			if s, ok := e.(string); ok {
				strs = append(strs, s)
			}
		}
		s.Enum = strs
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
