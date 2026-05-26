// Package cast provides type conversion helpers for map[string]any and similar generic data.
package cast

import (
	"fmt"
	"math"
)

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

// ToInt64 converts a numeric value to int64. Clamps uint64/uint to [math.MaxInt64] when out of range.
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
		if math.Trunc(x) != x || x < math.MinInt64 || x > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	case float32:
		f64 := float64(x)
		if math.IsNaN(f64) || math.IsInf(f64, 0) {
			return 0, false
		}
		if math.Trunc(f64) != f64 || f64 < math.MinInt64 || f64 > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	default:
		return 0, false
	}
}

// ToFloat32 converts a numeric value to float32.
func ToFloat32(v any) (float32, error) {
	f64, ok := ToFloat64(v)
	if !ok {
		return 0, fmt.Errorf("cast: cannot convert %T to float32", v)
	}
	if math.IsNaN(f64) || math.IsInf(f64, 0) {
		return 0, fmt.Errorf("cast: cannot convert non-finite %T to float32", v)
	}
	f32 := float32(f64)
	if math.IsNaN(float64(f32)) || math.IsInf(float64(f32), 0) {
		return 0, fmt.Errorf("cast: value %v is out of float32 range", f64)
	}
	return f32, nil
}

// ToInt32 converts a numeric value to int32.
func ToInt32(v any) (int32, error) {
	i64, ok := ToInt64(v)
	if !ok {
		return 0, fmt.Errorf("cast: cannot convert %T to int32", v)
	}
	if i64 < math.MinInt32 || i64 > math.MaxInt32 {
		return 0, fmt.Errorf("cast: value %d is out of int32 range", i64)
	}
	return int32(i64), nil
}

// ToBool converts a value to bool.
func ToBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("cast: cannot convert %T to bool", v)
	}
	return b, nil
}

// ToString converts a value to string.
func ToString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("cast: cannot convert %T to string", v)
	}
	return s, nil
}

// ToStringSlice converts v to []string.
// Accepts []string or []any where each element is convertible via ToString.
func ToStringSlice(v any) ([]string, error) {
	if ss, ok := v.([]string); ok {
		return ss, nil
	}
	slice, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("cast: cannot convert %T to []string", v)
	}
	out := make([]string, 0, len(slice))
	for _, e := range slice {
		s, err := ToString(e)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
