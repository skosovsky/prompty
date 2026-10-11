package gemini

import (
	"encoding/json"
	"reflect"
)

// Rounding is justified only at the actual SDK float leaf. An unrelated float
// never permits a caller's exact integer, string or native field to disagree.
//
//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func matchesGeminiSDKArguments(source, sdk any) bool {
	sourceJSON, sourceErr := json.Marshal(source)
	sdkJSON, sdkErr := json.Marshal(sdk)
	if sourceErr != nil || sdkErr != nil {
		return false
	}
	if exactGeminiSDKJSONEqual(sourceJSON, sdkJSON) {
		return true
	}
	v := reflect.ValueOf(sdk)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Float64, reflect.Float32:
		number, ok := source.(json.Number)
		if !ok {
			return false
		}
		parsed, err := number.Float64()
		if v.Kind() == reflect.Float32 {
			parsed = float64(float32(parsed))
		}
		return err == nil && parsed == v.Float()
	case reflect.Map:
		object, ok := source.(map[string]any)
		if !ok || v.Type().Key().Kind() != reflect.String || len(object) != v.Len() {
			return false
		}
		for _, key := range v.MapKeys() {
			value, exists := object[key.String()]
			if !exists || !matchesGeminiSDKArguments(value, v.MapIndex(key).Interface()) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		array, ok := source.([]any)
		if !ok || len(array) != v.Len() {
			return false
		}
		for i := range array {
			if !matchesGeminiSDKArguments(array[i], v.Index(i).Interface()) {
				return false
			}
		}
		return true
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Pointer,
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		fallthrough
	default:
		// Custom JSON projections cannot establish a per-field float binding.
		return false
	}
}
