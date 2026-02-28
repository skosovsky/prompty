package cast

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFloat64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		v    any
		want float64
		ok   bool
	}{
		{"float64", float64(1.5), 1.5, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", 3, 3, true},
		{"int64", int64(4), 4, true},
		{"int32", int32(5), 5, true},
		{"int16", int16(6), 6, true},
		{"int8", int8(7), 7, true},
		{"uint", uint(8), 8, true},
		{"uint8", uint8(9), 9, true},
		{"uint16", uint16(10), 10, true},
		{"uint32", uint32(11), 11, true},
		{"uint64", uint64(12), 12, true},
		{"string", "1.0", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ToFloat64(tt.v)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.InDelta(t, tt.want, got, 1e-9)
			}
		})
	}
}

func TestToInt64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		v    any
		want int64
		ok   bool
	}{
		{"int64", int64(1), 1, true},
		{"int", 2, 2, true},
		{"int32", int32(3), 3, true},
		{"int16", int16(4), 4, true},
		{"int8", int8(5), 5, true},
		{"uint", uint(6), 6, true},
		{"uint8", uint8(7), 7, true},
		{"uint16", uint16(8), 8, true},
		{"uint32", uint32(9), 9, true},
		{"uint64 small", uint64(10), 10, true},
		{"uint64 overflow clamped", uint64(math.MaxInt64) + 999, math.MaxInt64, true},
		{"float64", float64(9), 9, true},
		{"float32", float32(10), 10, true},
		{"string", "1", 0, false},
		{"bool", false, 0, false},
		{"nil", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ToInt64(tt.v)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestToStringSlice(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		v      any
		want   []string
		wantOk bool
	}{
		{"[]string", []string{"a", "b"}, []string{"a", "b"}, true},
		{"[]any all strings", []any{"x", "y"}, []string{"x", "y"}, true},
		{"[]any empty", []any{}, []string{}, true},
		{"[]any mixed types", []any{"a", 123, "b"}, nil, false},
		{"[]any with bool", []any{"a", true}, nil, false},
		{"non-slice", "not a slice", nil, false},
		{"nil", nil, nil, false},
		{"map", map[string]any{}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ToStringSlice(tt.v)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
