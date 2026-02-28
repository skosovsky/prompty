// Package cast provides type conversion helpers for map[string]any and similar generic data.
package cast

import "math"

// ToFloat64 converts a numeric value to float64. Supports int/uint/float types.
func ToFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case int16:
		return float64(x), true
	case int8:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

// ToInt64 converts a numeric value to int64. Clamps uint64/uint to math.MaxInt64 when out of range.
func ToInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int16:
		return int64(x), true
	case int8:
		return int64(x), true
	case uint:
		if x > math.MaxInt64 {
			return math.MaxInt64, true
		}
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		if x > math.MaxInt64 {
			return math.MaxInt64, true
		}
		return int64(x), true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return int64(x), true
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return 0, false
		}
		return int64(x), true
	default:
		return 0, false
	}
}

// ToStringSlice converts v to []string. Accepts []string or []any where each element is string.
func ToStringSlice(v any) ([]string, bool) {
	if ss, ok := v.([]string); ok {
		return ss, true
	}
	slice, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(slice))
	for _, e := range slice {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
