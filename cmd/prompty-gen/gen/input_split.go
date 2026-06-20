package gen

import (
	"maps"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/late"
)

func propertyIsLate(prop map[string]any) bool {
	return late.PropertyIsLate(prop)
}

func sanitizePropForCodegen(prop map[string]any) map[string]any {
	if prop == nil {
		return nil
	}
	out := maps.Clone(prop)
	delete(out, "late")
	delete(out, "x-prompty-late")
	delete(out, "required")
	return out
}

// splitEarlyLateInputSchema splits bindable input properties into early recipe input and late input.
func splitEarlyLateInputSchema(schema map[string]any) (map[string]any, map[string]any, bool) {
	if schema == nil {
		return nil, nil, false
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return schema, nil, false
	}

	earlyProps := make(map[string]any, len(props))
	lateProps := make(map[string]any)
	hasLate := false
	for _, name := range sortedKeys(props) {
		raw, _ := props[name].(map[string]any)
		clean := sanitizePropForCodegen(raw)
		if propertyIsLate(raw) {
			lateProps[name] = clean
			hasLate = true
			continue
		}
		earlyProps[name] = clean
	}

	early := cloneObjectSchemaWithProps(schema, earlyProps)
	var late map[string]any
	if hasLate {
		late = cloneObjectSchemaWithProps(schema, lateProps)
	}
	return early, late, hasLate
}

func cloneObjectSchemaWithProps(base map[string]any, props map[string]any) map[string]any {
	out := map[string]any{
		"type":       jsonSchemaTypeObject,
		"properties": props,
	}
	required := filterRequired(getRequired(base), props)
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func filterRequired(required map[string]bool, props map[string]any) []string {
	if len(required) == 0 {
		return nil
	}
	out := make([]string, 0)
	for _, name := range sortedKeys(props) {
		if required[name] {
			out = append(out, name)
		}
	}
	return out
}

func schemaDefinitionFromMap(schema map[string]any) (*prompty.SchemaDefinition, error) {
	if schema == nil {
		return nil, nil //nolint:nilnil // absent schema is valid for empty input structs
	}
	doc, err := prompty.MapToJSONDocument(schema)
	if err != nil {
		return nil, err
	}
	return &prompty.SchemaDefinition{Schema: doc}, nil
}
