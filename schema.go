package prompty

import (
	"errors"
	"reflect"
)

var schemaProviderType = reflect.TypeFor[SchemaProvider]()

// SchemaProvider allows caller-owned types to provide a JSON Schema without reflection.
type SchemaProvider interface {
	JSONSchema() map[string]any
}

// ExtractSchema returns a JSON Schema for v using SchemaProvider when available,
// otherwise falling back to the built-in reflect generator.
//
// On unsupported input, it returns nil.
func ExtractSchema(v any) map[string]any {
	schema, err := extractSchema(v)
	if err != nil {
		return nil
	}
	return schema
}

func extractSchema(v any) (map[string]any, error) {
	t, err := normalizedSchemaTypeFromValue(v)
	if err != nil {
		return nil, err
	}
	return extractSchemaFromType(t)
}

func extractSchemaFromType(t reflect.Type) (map[string]any, error) {
	base, err := normalizedSchemaType(t)
	if err != nil {
		return nil, err
	}
	if schema, ok := schemaFromProvider(base); ok {
		return schema, nil
	}
	return newSchemaBuilder().schemaForType(base, 0)
}

func normalizedSchemaTypeFromValue(v any) (reflect.Type, error) {
	if v == nil {
		return nil, errors.New("schema: nil value")
	}
	return normalizedSchemaType(reflect.TypeOf(v))
}

func normalizedSchemaType(t reflect.Type) (reflect.Type, error) {
	if t == nil {
		return nil, errors.New("schema: nil type")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		if t == nil {
			return nil, errors.New("schema: nil type")
		}
	}
	return t, nil
}

func schemaFromProvider(t reflect.Type) (map[string]any, bool) {
	if t == nil {
		return nil, false
	}

	candidates := []reflect.Type{t}
	if t.Kind() != reflect.Pointer {
		candidates = append(candidates, reflect.PointerTo(t))
	}

	for _, candidate := range candidates {
		provider, ok := instantiateSchemaProvider(candidate)
		if ok {
			return cloneMapAny(provider.JSONSchema()), true
		}
	}
	return nil, false
}

func instantiateSchemaProvider(t reflect.Type) (SchemaProvider, bool) {
	if t == nil || !t.Implements(schemaProviderType) {
		return nil, false
	}

	var value reflect.Value
	if t.Kind() == reflect.Pointer {
		value = reflect.New(t.Elem())
	} else {
		value = reflect.New(t).Elem()
	}
	if !value.IsValid() || !value.CanInterface() {
		return nil, false
	}

	provider, ok := value.Interface().(SchemaProvider)
	return provider, ok
}
