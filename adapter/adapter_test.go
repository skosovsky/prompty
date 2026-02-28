package adapter

import (
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
			prompty.MediaPart{MediaType: "image", URL: "https://x"},
			prompty.TextPart{Text: "y"},
			prompty.ToolCallPart{ID: "1", Name: "f", Args: "{}"},
		}, "xy"},
		{"no text", []prompty.ContentPart{
			prompty.MediaPart{MediaType: "image", URL: "https://x"},
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
		name  string
		cfg   map[string]any
		check func(t *testing.T, mp ModelParams)
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
			assert.InDelta(t, 0.7, *mp.Temperature, 1e-9)
		}},
		{"temperature float32", map[string]any{"temperature": float32(0.5)}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.Temperature)
			assert.InDelta(t, 0.5, *mp.Temperature, 1e-9)
		}},
		{"temperature int", map[string]any{"temperature": 1}, func(t *testing.T, mp ModelParams) {
			require.NotNil(t, mp.Temperature)
			assert.InDelta(t, float64(1), *mp.Temperature, 1e-9)
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
			assert.InDelta(t, 0.9, *mp.TopP, 1e-9)
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
			assert.InDelta(t, 0.5, *mp.Temperature, 1e-9)
			require.NotNil(t, mp.MaxTokens)
			assert.Equal(t, int64(50), *mp.MaxTokens)
			require.NotNil(t, mp.TopP)
			assert.InDelta(t, 0.95, *mp.TopP, 1e-9)
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

// Cast helpers are tested in internal/cast.
