package gemini

import (
	"fmt"
	"slices"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

// MapSchema verifies the implemented strict typed-schema subset before translation.
// Explicit lowering transformations are not yet implemented by this mapper.
func (a *Adapter) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if a.nativeJSONSchema {
		return a.mapNativeSchema(request)
	}
	if request.Profile.Provider != wireGemini {
		return prompty.SchemaMapping{}, fidelityError("", "foreign schema profile")
	}
	source, err := prompty.SchemaDocumentAsMap(request.Source)
	if err != nil {
		return prompty.SchemaMapping{}, err
	}
	if _, err := mapToGenaiSchema(source); err != nil {
		return prompty.SchemaMapping{}, err
	}
	return prompty.SchemaMapping{Transformations: nil, Schema: prompty.CloneJSONDocument(request.Source)}, nil
}

// mapToGenaiSchema converts a JSON Schema (map[string]any) to genai.Schema.
// Unsupported keywords and shapes fail explicitly with source JSON pointers.
func mapToGenaiSchema(m map[string]any) (*genai.Schema, error) {
	return mapGenaiSchemaAt(m, "")
}

//nolint:cyclop,funlen,gocognit,gocyclo,nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func mapGenaiSchemaAt(m map[string]any, pointer string) (*genai.Schema, error) {
	if m == nil {
		return nil, fidelityError(pointer, "schema must be an object")
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		switch key {
		case "type", "properties", "required", "items", "description", "enum":
		default:
			return nil, fidelityError(pointerKey(pointer, key), "keyword is not mapped")
		}
	}
	s := &genai.Schema{}
	if raw, exists := m["type"]; exists {
		t, ok := raw.(string)
		if !ok || jsonSchemaTypeToGenai(t) == genai.TypeUnspecified {
			return nil, fidelityError(pointerKey(pointer, "type"), "type is not mapped")
		}
	}
	if t, ok := m["type"].(string); ok && t != "" {
		s.Type = jsonSchemaTypeToGenai(t)
	}
	if p, ok := m["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*genai.Schema)
		names := make([]string, 0, len(p))
		for name := range p {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, k := range names {
			v := p[k]
			sub, ok := v.(map[string]any)
			if !ok {
				return nil, fidelityError(pointerKey(pointerKey(pointer, "properties"), k), "schema must be an object")
			}
			conv, err := mapGenaiSchemaAt(sub, pointerKey(pointerKey(pointer, "properties"), k))
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", k, err)
			}
			if conv != nil {
				s.Properties[k] = conv
			}
		}
	} else if _, exists := m["properties"]; exists {
		return nil, fidelityError(pointerKey(pointer, "properties"), "properties must be an object")
	}
	if r, ok := m["required"].([]any); ok {
		required := make([]string, 0, len(r))
		for i, x := range r {
			if str, ok := x.(string); ok {
				required = append(required, str)
			} else {
				return nil, fidelityError(fmt.Sprintf("%s/required/%d", pointer, i), "required name must be a string")
			}
		}
		s.Required = required
	} else if r, ok := m["required"].([]string); ok {
		s.Required = r
	} else if _, exists := m["required"]; exists {
		return nil, fidelityError(pointerKey(pointer, "required"), "required must be an array")
	}
	if items, ok := m["items"]; ok {
		sub, ok := items.(map[string]any)
		if ok {
			conv, err := mapGenaiSchemaAt(sub, pointerKey(pointer, "items"))
			if err != nil {
				return nil, fmt.Errorf("items: %w", err)
			}
			if conv != nil {
				s.Items = conv
			}
		} else {
			return nil, fidelityError(pointerKey(pointer, "items"), "items schema must be an object")
		}
	}
	if desc, ok := m["description"].(string); ok {
		s.Description = desc
	} else if _, exists := m["description"]; exists {
		return nil, fidelityError(pointerKey(pointer, "description"), "description must be a string")
	}
	if enum, ok := m["enum"].([]any); ok {
		strs := make([]string, 0, len(enum))
		for i, e := range enum {
			if s, ok := e.(string); ok {
				strs = append(strs, s)
			} else {
				return nil, fidelityError(fmt.Sprintf("%s/enum/%d", pointer, i), "only string enum values are mapped")
			}
		}
		s.Enum = strs
	} else if _, exists := m["enum"]; exists {
		return nil, fidelityError(pointerKey(pointer, "enum"), "enum must be an array")
	}
	return s, nil
}

func pointerKey(parent, key string) string {
	return parent + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func fidelityError(pointer, reason string) error {
	return &prompty.SchemaFidelityError{Pointer: pointer, Reason: reason}
}

func jsonSchemaTypeToGenai(t string) genai.Type {
	switch t {
	case wireString:
		return genai.TypeString
	case wireNumber:
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case wireObject:
		return genai.TypeObject
	default:
		return genai.TypeUnspecified
	}
}
