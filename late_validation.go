package prompty

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/skosovsky/prompty/internal/cast"
	"github.com/skosovsky/prompty/internal/late"
)

//nolint:gochecknoglobals // shared validator for WithLateInput struct tags (matches prompty-gen codegen)
var lateInputValidator = validator.New()

func inputPropertyIsLate(prop map[string]any) bool {
	return late.PropertyIsLate(prop)
}

func validateLateInputFields(schema *SchemaDefinition, vars map[string]any) error {
	if schema == nil || len(schema.Schema) == 0 || len(vars) == 0 {
		return nil
	}
	doc, err := JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return fmt.Errorf("late input: decode schema: %w", err)
	}
	props, _ := doc["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	for name := range vars {
		raw, ok := props[name]
		if !ok {
			return fmt.Errorf("late input: unknown field %q", name)
		}
		pm, _ := raw.(map[string]any)
		if !inputPropertyIsLate(pm) {
			return fmt.Errorf("late input: field %q is early-bound; use Render input instead", name)
		}
	}
	return nil
}

func validateEarlyInputFields(schema *SchemaDefinition, vars map[string]any) error {
	if schema == nil || len(schema.Schema) == 0 || len(vars) == 0 {
		return nil
	}
	doc, err := JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return fmt.Errorf("early input: decode schema: %w", err)
	}
	props, _ := doc["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	for name := range vars {
		raw, ok := props[name]
		if !ok {
			return fmt.Errorf("early input: unknown field %q", name)
		}
		pm, _ := raw.(map[string]any)
		if inputPropertyIsLate(pm) {
			return fmt.Errorf("early input: field %q is late-bound; use WithLateInput instead", name)
		}
	}
	return nil
}

func validateRequiredLateVars(schema *SchemaDefinition, lateVars map[string]any, templateID string) error {
	if schema == nil || len(schema.Schema) == 0 {
		return nil
	}
	doc, err := JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return fmt.Errorf("late input: decode schema: %w", err)
	}
	props, _ := doc["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	required := lateRequiredNames(doc, props)
	if len(required) == 0 {
		return nil
	}
	merged := mapsCloneStringAny(lateVars)
	for _, name := range required {
		v, ok := merged[name]
		if !ok || !lateBindingPresent(v) {
			return &VariableError{
				Variable: name,
				Template: templateID,
				Err:      ErrMissingVariable,
			}
		}
	}
	return nil
}

func lateBindingPresent(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) != ""
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.String:
			return strings.TrimSpace(rv.String()) != ""
		case reflect.Pointer, reflect.Interface:
			if rv.IsNil() {
				return false
			}
			return lateBindingPresent(rv.Elem().Interface())
		case reflect.Slice, reflect.Map:
			return rv.Len() > 0
		default:
			return true
		}
	}
}

func lateRequiredNames(doc map[string]any, props map[string]any) []string {
	req, ok := doc["required"]
	if !ok {
		return nil
	}
	ss, err := cast.ToStringSlice(req)
	if err != nil || len(ss) == 0 {
		return nil
	}
	out := make([]string, 0)
	for _, name := range ss {
		raw, _ := props[name].(map[string]any)
		if inputPropertyIsLate(raw) {
			out = append(out, name)
		}
	}
	return out
}

func mapsCloneStringAny(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}
	return cloneMapAny(m)
}

func ensureOptionalLateDefaults(schema *SchemaDefinition, lateVars map[string]any) map[string]any {
	if schema == nil || len(schema.Schema) == 0 {
		return lateVars
	}
	doc, err := JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return lateVars
	}
	props, _ := doc["properties"].(map[string]any)
	if len(props) == 0 {
		return lateVars
	}
	requiredLate := make(map[string]bool, len(lateRequiredNames(doc, props)))
	for _, name := range lateRequiredNames(doc, props) {
		requiredLate[name] = true
	}
	out := cloneMapAny(lateVars)
	if out == nil {
		out = make(map[string]any)
	}
	for name, raw := range props {
		pm, ok := raw.(map[string]any)
		if !ok || !inputPropertyIsLate(pm) || requiredLate[name] {
			continue
		}
		if _, present := out[name]; !present {
			out[name] = ""
		}
	}
	return out
}

func validateLateInputStruct(input any) error {
	_, val, ok := structTypeOf(input)
	if !ok {
		return nil
	}
	return lateInputValidator.Struct(val.Interface())
}
