package adapter

import "reflect"

// HasUnverifiedJSONNumbers reports floating-point values in a caller SDK argument
// graph. Such values cannot attest original provider JSON precision without source.
func HasUnverifiedJSONNumbers(value any) bool {
	return containsFloatValue(reflect.ValueOf(value), make(map[numericSourceVisit]bool))
}

type numericSourceVisit struct {
	typ     reflect.Type
	pointer uintptr
	length  int
}

func containsFloatValue(value reflect.Value, visited map[numericSourceVisit]bool) bool {
	if !value.IsValid() || numericReferenceSeen(value, visited) {
		return false
	}
	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		return true
	case reflect.Interface, reflect.Pointer:
		return !value.IsNil() && containsFloatValue(value.Elem(), visited)
	case reflect.Map:
		return containsMapFloat(value, visited)
	case reflect.Slice, reflect.Array:
		return containsIndexedFloat(value, visited)
	case reflect.Struct:
		return containsStructFloat(value, visited)
	case reflect.Invalid, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func, reflect.String, reflect.UnsafePointer:
		return false
	}
	return false
}
func numericReferenceSeen(value reflect.Value, visited map[numericSourceVisit]bool) bool {
	if value.Kind() != reflect.Pointer && value.Kind() != reflect.Map && value.Kind() != reflect.Slice {
		return false
	}
	if value.IsNil() {
		return true
	}
	length := 0
	if value.Kind() != reflect.Pointer {
		length = value.Len()
	}
	key := numericSourceVisit{typ: value.Type(), pointer: value.Pointer(), length: length}
	if visited[key] {
		return true
	}
	visited[key] = true
	return false
}
func containsMapFloat(value reflect.Value, visited map[numericSourceVisit]bool) bool {
	iterator := value.MapRange()
	for iterator.Next() {
		if containsFloatValue(iterator.Value(), visited) {
			return true
		}
	}
	return false
}
func containsIndexedFloat(value reflect.Value, visited map[numericSourceVisit]bool) bool {
	for index := range value.Len() {
		if containsFloatValue(value.Index(index), visited) {
			return true
		}
	}
	return false
}
func containsStructFloat(value reflect.Value, visited map[numericSourceVisit]bool) bool {
	for index := range value.NumField() {
		if value.Type().Field(index).IsExported() && containsFloatValue(value.Field(index), visited) {
			return true
		}
	}
	return false
}
