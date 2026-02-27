package adapter

import (
	"math"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTextFromParts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		parts []prompty.ContentPart
		want  string
	}{
		{"empty slice", []prompty.ContentPart{}, ""},
		{"nil slice", nil, ""},
		{"single text", []prompty.ContentPart{prompty.TextPart{Text: "hello"}}, "hello"},
		{"multiple text", []prompty.ContentPart{
			prompty.TextPart{Text: "a"},
			prompty.TextPart{Text: "b"},
			prompty.TextPart{Text: "c"},
		}, "abc"},
		{"mixed parts", []prompty.ContentPart{
			prompty.TextPart{Text: "x"},
			prompty.ImagePart{URL: "https://x"},
			prompty.TextPart{Text: "y"},
			prompty.ToolCallPart{ID: "1", Name: "f", Args: "{}"},
		}, "xy"},
		{"no text", []prompty.ContentPart{
			prompty.ImagePart{URL: "https://x"},
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := TextFromParts(tt.parts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractModelConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cfg    map[string]any
		check  func(t *testing.T, mp ModelParams)
	}{
		{"nil map", nil, func(t *testing.T, mp ModelParams) {
			assert.Nil(t, mp.Temperature)
			assert.Nil(t, mp.MaxTokens)
			assert.Nil(t, mp.TopP)
			assert.Nil(t, mp.Stop)
		}},
		{"empty map", map[string]any{}, func(t *testing.T, mp ModelParams) {
			assert.Nil(t, mp.Temperature)
			assert.Nil(t, mp.MaxTokens)
			assert.Nil(t, mp.TopP)
			assert.Nil(t, mp.Stop)
		}},
		{"temperature float64", map[string]any{"temperature": 0.7}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.Temperature)
			assert.Equal(t, 0.7, *mp.Temperature)
		}},
		{"temperature float32", map[string]any{"temperature": float32(0.5)}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.Temperature)
			assert.Equal(t, 0.5, *mp.Temperature)
		}},
		{"temperature int", map[string]any{"temperature": 1}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.Temperature)
			assert.Equal(t, float64(1), *mp.Temperature)
		}},
		{"max_tokens int64", map[string]any{"max_tokens": int64(100)}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.MaxTokens)
			assert.Equal(t, int64(100), *mp.MaxTokens)
		}},
		{"max_tokens int", map[string]any{"max_tokens": 200}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.MaxTokens)
			assert.Equal(t, int64(200), *mp.MaxTokens)
		}},
		{"max_tokens float64", map[string]any{"max_tokens": float64(300)}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.MaxTokens)
			assert.Equal(t, int64(300), *mp.MaxTokens)
		}},
		{"top_p", map[string]any{"top_p": 0.9}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.TopP)
			assert.Equal(t, 0.9, *mp.TopP)
		}},
		{"stop []string", map[string]any{"stop": []string{"A", "B"}}, func(t *testing.T, mp ModelParams) {
			assert.Equal(t, []string{"A", "B"}, mp.Stop)
		}},
		{"stop []any strings", map[string]any{"stop": []any{"x", "y"}}, func(t *testing.T, mp ModelParams) {
			assert.Equal(t, []string{"x", "y"}, mp.Stop)
		}},
		{"all keys", map[string]any{
			"temperature": 0.5,
			"max_tokens":  int64(50),
			"top_p":       0.95,
			"stop":        []string{"END"},
		}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.Temperature)
			assert.Equal(t, 0.5, *mp.Temperature)
			require.NotNil(t, mp.MaxTokens)
			assert.Equal(t, int64(50), *mp.MaxTokens)
			require.NotNil(t, mp.TopP)
			assert.Equal(t, 0.95, *mp.TopP)
			assert.Equal(t, []string{"END"}, mp.Stop)
		}},
		{"unknown key ignored", map[string]any{"model": "gpt-4", "foo": 1}, func(t *testing.T, mp ModelParams) {
			assert.Nil(t, mp.Temperature)
			assert.Nil(t, mp.MaxTokens)
		}},
		{"temperature invalid type", map[string]any{"temperature": "high"}, func(t *testing.T, mp ModelParams) {
			assert.Nil(t, mp.Temperature)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractModelConfig(tt.cfg)
			tt.check(t, got)
		})
	}
}

func TestToFloat64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		v     any
		want  float64
		ok    bool
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
			got, ok := toFloat64(tt.v)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestToInt64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		v     any
		want  int64
		ok    bool
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
			got, ok := toInt64(tt.v)
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
		wantOk  bool
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
			got, ok := toStringSlice(tt.v)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
