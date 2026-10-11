package prompty

import (
	"errors"
	"fmt"
	"reflect"
)

//nolint:gochecknoglobals // Cached immutable reflection type.
var schemaProviderType = reflect.TypeFor[SchemaProvider]()

// SchemaProvider allows caller-owned types to provide a JSON Schema without reflection.
type SchemaProvider interface {
	JSONSchema() JSONDocument
}

// ExtractSchema returns the explicit or inferred JSON Schema for the caller's type.
// Unsupported codecs, recursion and invalid explicit schemas return an error.
func ExtractSchema(v any) (map[string]any, error) {
	return extractSchema(v)
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
	if schema, ok, providerErr := schemaFromProvider(base); ok {
		return schema, providerErr
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
	seen := make(map[reflect.Type]bool)
	for t.Kind() == reflect.Pointer {
		if seen[t] || len(seen) >= maxSchemaDepth {
			return nil, fmt.Errorf("%w: recursive or excessive-depth pointer type %s", ErrSchemaUnsupported, t)
		}
		seen[t] = true
		t = t.Elem()
	}

	return t, nil
}

func schemaFromProvider(t reflect.Type) (map[string]any, bool, error) {
	if t == nil {
		return nil, false, nil
	}

	candidates := []reflect.Type{t}
	if t.Kind() != reflect.Pointer {
		candidates = append(candidates, reflect.PointerTo(t))
	}

	for _, candidate := range candidates {
		provider, ok := instantiateSchemaProvider(candidate)
		if ok {
			doc := provider.JSONSchema()
			m, err := SchemaDocumentAsMap(doc)
			if err != nil {
				return nil, true, fmt.Errorf("schema provider %s: %w", t, err)
			}
			if _, err := NewJSONSchemaValidator(nil).CompileSchema(doc, schemaDeclaredDialect(m)); err != nil {
				return nil, true, fmt.Errorf("schema provider %s: %w", t, err)
			}
			return cloneMapAny(m), true, nil
		}
	}
	return nil, false, nil
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

	provider, ok := reflect.TypeAssert[SchemaProvider](value)
	return provider, ok
}

func schemaDeclaredDialect(schema map[string]any) string {
	dialect, _ := schema["$schema"].(string)
	return dialect
}
