package prompty

import (
	"fmt"
	"reflect"
	"strings"
)

// bindTemplateVars maps a struct payload to template variables using prompt tags only.
// Nested objects become map[string]any so text/template can address fields by prompt keys.
// []ChatMessage history is returned separately and is not included in the vars map.
func bindTemplateVars(v any) (map[string]any, []ChatMessage, error) {
	typ, val, ok := structTypeOf(v)
	if !ok {
		return nil, nil, ErrInvalidPayload
	}
	return bindStructFields(typ, val)
}

func promptAlias(field reflect.StructField) string {
	tag := strings.TrimSpace(strings.Split(field.Tag.Get("prompt"), ",")[0])
	if tag == "" || tag == "-" {
		return ""
	}
	return tag
}

func isChatMessageSlice(typ reflect.Type) bool {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Slice {
		return false
	}
	elem := typ.Elem()
	if elem == reflect.TypeFor[ChatMessage]() {
		return true
	}
	return elem.Kind() == reflect.Pointer && elem.Elem() == reflect.TypeFor[ChatMessage]()
}

func structHasPromptFields(typ reflect.Type) bool {
	for field := range typ.Fields() {
		if !field.IsExported() {
			continue
		}
		if isChatMessageSlice(field.Type) {
			continue
		}
		if promptAlias(field) != "" {
			return true
		}
	}
	return false
}

func errAnonymousEmbedWithoutPrompt(field reflect.StructField) error {
	if !field.Anonymous {
		return nil
	}
	elem := field.Type
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct || !structHasPromptFields(elem) {
		return nil
	}
	return fmt.Errorf(
		"%w: anonymous embedded type %s requires prompt tag",
		ErrInvalidPayload,
		elem.Name(),
	)
}

func bindStructFields(typ reflect.Type, val reflect.Value) (map[string]any, []ChatMessage, error) {
	out := make(map[string]any)
	var history []ChatMessage
	hasPrompt := false
	hasHistory := false

	for field := range typ.Fields() {
		if !field.IsExported() {
			continue
		}
		if isChatMessageSlice(field.Type) {
			hasHistory = true
			next, err := appendHistoryField(history, val.FieldByIndex(field.Index))
			if err != nil {
				return nil, nil, err
			}
			history = next
			continue
		}
		alias := promptAlias(field)
		if alias == "" {
			if err := errAnonymousEmbedWithoutPrompt(field); err != nil {
				return nil, nil, err
			}
			continue
		}

		hasPrompt = true
		if err := bindPromptFieldTo(out, field, val.FieldByIndex(field.Index)); err != nil {
			return nil, nil, err
		}
	}

	if typ.NumField() == 0 {
		return out, history, nil
	}
	return finalizeStructBinding(out, history, hasPrompt, hasHistory)
}

func finalizeStructBinding(
	out map[string]any,
	history []ChatMessage,
	hasPrompt bool,
	hasHistory bool,
) (map[string]any, []ChatMessage, error) {
	if !hasPrompt && !hasHistory {
		return nil, nil, ErrInvalidPayload
	}
	if !hasPrompt {
		return make(map[string]any), history, nil
	}
	return out, history, nil
}

func appendHistoryField(history []ChatMessage, fv reflect.Value) ([]ChatMessage, error) {
	if !fv.IsValid() || !fv.CanInterface() {
		return history, nil
	}
	fv = derefValue(fv)
	if fv.Kind() != reflect.Slice || !isChatMessageSlice(fv.Type()) {
		return history, nil
	}
	if fv.IsNil() || fv.Len() == 0 {
		return history, nil
	}
	cm := make([]ChatMessage, fv.Len())
	for i := range fv.Len() {
		item, ok := chatMessageFromValue(fv.Index(i))
		if !ok {
			return nil, fmt.Errorf(
				"%w: history element at index %d has type %s",
				ErrInvalidPayload,
				i,
				fv.Index(i).Type(),
			)
		}
		cm[i] = item
	}
	return cloneMessages(cm), nil
}

func chatMessageFromValue(v reflect.Value) (ChatMessage, bool) {
	if !v.IsValid() {
		return ChatMessage{}, false
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ChatMessage{}, false
		}
		v = v.Elem()
	}
	cm, ok := reflect.TypeAssert[ChatMessage](v)
	return cm, ok
}

func derefValue(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Zero(v.Type().Elem())
		}
		v = v.Elem()
	}
	return v
}

func bindValue(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil //nolint:nilnil // template binding: absent invalid value is nil
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil //nolint:nilnil // A nil pointer is a present JSON null.
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		if structHasPromptFields(v.Type()) {
			m, _, err := bindStructFields(v.Type(), v)
			return m, err
		}
		return nil, fmt.Errorf("%w: nested type %s lacks prompt fields", ErrInvalidPayload, v.Type().Name())
	case reflect.Slice:
		return bindSliceValue(v)
	case reflect.Interface:
		if v.IsNil() {
			return nil, nil //nolint:nilnil // nil interface value
		}
		return bindValue(v.Elem())
	default:
		return v.Interface(), nil
	}
}

func bindSliceValue(v reflect.Value) (any, error) {
	if v.IsNil() {
		return v.Interface(), nil
	}
	elem := v.Type().Elem()
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct || !structHasPromptFields(elem) {
		return v.Interface(), nil
	}
	out := make([]map[string]any, v.Len())
	for i := range v.Len() {
		item := v.Index(i)
		for item.Kind() == reflect.Pointer && !item.IsNil() {
			item = item.Elem()
		}
		if item.Kind() == reflect.Pointer && item.IsNil() {
			continue
		}
		m, _, err := bindStructFields(elem, item)
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

func structTypeOf(v any) (reflect.Type, reflect.Value, bool) {
	if v == nil {
		return nil, reflect.Value{}, false
	}
	val := reflect.ValueOf(v)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil, reflect.Value{}, false
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, reflect.Value{}, false
	}
	return val.Type(), val, true
}

func promptFieldOmitted(field reflect.StructField, value reflect.Value) bool {
	if !jsonTagOption(field.Tag.Get("prompt"), "omitempty") {
		return false
	}
	if value.IsZero() {
		return true
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return value.Len() == 0
	default:
		return false
	}
}

func bindPromptFieldTo(out map[string]any, field reflect.StructField, value reflect.Value) error {
	alias := promptAlias(field)
	if alias == "Tools" {
		return ErrReservedVariable
	}
	if _, exists := out[alias]; exists {
		return fmt.Errorf("%w: duplicate prompt tag %q", ErrInvalidPayload, alias)
	}
	if promptFieldOmitted(field, value) {
		return nil
	}
	bound, err := bindValue(value)
	if err != nil {
		return err
	}
	out[alias] = bound
	return nil
}

// historySchemaValues retains field presence and null independently of flattened render history.
func historySchemaValues(payload any) map[string]any {
	typ, value, ok := structTypeOf(payload)
	if !ok {
		return nil
	}
	result := make(map[string]any)
	for field := range typ.Fields() {
		name := promptAlias(field)
		if !field.IsExported() || name == "" || !isChatMessageSlice(field.Type) {
			continue
		}
		fieldValue := value.FieldByIndex(field.Index)
		if promptFieldOmitted(field, fieldValue) {
			continue
		}
		result[name] = fieldValue.Interface()
	}
	return cloneMapAny(result)
}
