package prompty

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateChars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		text     string
		maxChars int
		want     string
	}{
		{"empty", "", 5, ""},
		{"ASCII under limit", "hello", 10, "hello"},
		{"ASCII exact", "hello", 5, "hello"},
		{"ASCII over", "hello world", 5, "hello"},
		{"Unicode", "привет", 3, "при"},
		{"Unicode under", "привет", 10, "привет"},
		{"limit over len", "hi", 100, "hi"},
		{"zero limit", "hello", 0, ""},
		{"negative limit", "hello", -1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateChars(tt.text, tt.maxChars)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncateTokens(t *testing.T) {
	t.Parallel()
	tc := &CharFallbackCounter{CharsPerToken: 4}
	fn := makeTruncateTokens(tc)
	tests := []struct {
		name      string
		text      string
		maxTokens int
		want      string
		wantErr   bool
	}{
		{"empty", "", 5, "", false},
		{"under limit", "hello", 10, "hello", false},
		{"exact", "abcdefgh", 2, "abcdefgh", false},
		{"over limit", "abcdefghijkl", 2, "abcdefgh", false},
		{"zero max", "hello", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := fn(tt.text, tt.maxTokens)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenderToolsAsXML_Golden(t *testing.T) {
	t.Parallel()
	tools := []ToolDefinition{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		{Name: "search", Description: "Search", Parameters: nil},
	}
	got, err := renderToolsAsXML(tools)
	require.NoError(t, err)
	golden := filepath.Join("testdata", "funcmap", "tools.xml.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0755))
		require.NoError(t, os.WriteFile(golden, []byte(got), 0644))
		return
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

func TestRenderToolsAsJSON_Golden(t *testing.T) {
	t.Parallel()
	tools := []ToolDefinition{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		{Name: "search", Description: "Search", Parameters: nil},
	}
	got, err := renderToolsAsJSON(tools)
	require.NoError(t, err)
	golden := filepath.Join("testdata", "funcmap", "tools.json.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0755))
		require.NoError(t, os.WriteFile(golden, []byte(got), 0644))
		return
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

func TestRenderToolsAsXML_Nil(t *testing.T) {
	t.Parallel()
	got, err := renderToolsAsXML(nil)
	require.NoError(t, err)
	assert.Equal(t, "<tools>\n</tools>", got)
}

func TestRenderToolsAsXML_InvalidType(t *testing.T) {
	t.Parallel()
	_, err := renderToolsAsXML(123)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected []ToolDefinition")
}

func TestRenderToolsAsJSON_Nil(t *testing.T) {
	t.Parallel()
	got, err := renderToolsAsJSON(nil)
	require.NoError(t, err)
	assert.Equal(t, "[]", got)
}

func TestRenderToolsAsJSON_InvalidType(t *testing.T) {
	t.Parallel()
	_, err := renderToolsAsJSON(123)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected []ToolDefinition")
}
