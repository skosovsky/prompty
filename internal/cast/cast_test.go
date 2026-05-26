package cast

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{"float64 fractional", float64(9.5), 0, false},
		{"float32 fractional", float32(10.25), 0, false},
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
		name    string
		v       any
		want    []string
		wantErr bool
	}{
		{"[]string", []string{"a", "b"}, []string{"a", "b"}, false},
		{"[]any all strings", []any{"x", "y"}, []string{"x", "y"}, false},
		{"[]any empty", []any{}, []string{}, false},
		{"[]any mixed types", []any{"a", 123, "b"}, nil, true},
		{"[]any with bool", []any{"a", true}, nil, true},
		{"non-slice", "not a slice", nil, true},
		{"nil", nil, nil, true},
		{"map", map[string]any{}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToStringSlice(tt.v)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToFloat32(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		v       any
		want    float32
		wantErr bool
	}{
		{"int", 1, 1, false},
		{"float64", 1.25, 1.25, false},
		{"float32", float32(2.5), 2.5, false},
		{"float64 overflow", 1e100, 0, true},
		{"float64 inf", math.Inf(1), 0, true},
		{"float64 nan", math.NaN(), 0, true},
		{"string", "1.0", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToFloat32(tt.v)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 1e-6)
		})
	}
}

func TestToInt32(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		v       any
		want    int32
		wantErr bool
	}{
		{"int", 1, 1, false},
		{"float64", 2.0, 2, false},
		{"float32", float32(3), 3, false},
		{"float64 fractional", 2.5, 0, true},
		{"float32 fractional", float32(3.5), 0, true},
		{"min boundary", int64(math.MinInt32), math.MinInt32, false},
		{"max boundary", int64(math.MaxInt32), math.MaxInt32, false},
		{"underflow", int64(math.MinInt32) - 1, 0, true},
		{"overflow", int64(math.MaxInt32) + 1, 0, true},
		{"string", "1", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToInt32(tt.v)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		v       any
		want    bool
		wantErr bool
	}{
		{"true", true, true, false},
		{"false", false, false, false},
		{"string", "true", false, true},
		{"int", 1, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToBool(tt.v)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		v       any
		want    string
		wantErr bool
	}{
		{"string", "abc", "abc", false},
		{"empty", "", "", false},
		{"int", 1, "", true},
		{"bool", true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToString(tt.v)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
