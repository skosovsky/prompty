package prompty

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpliceHistory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		rendered    []ChatMessage
		history     []ChatMessage
		wantLen     int
		firstUserAt int // index of first non-system in result
	}{
		{"empty history", []ChatMessage{{Role: "system", Content: []ContentPart{TextPart{Text: "S"}}}, {Role: "user", Content: []ContentPart{TextPart{Text: "U"}}}}, nil, 2, 1},
		{"all system then history", []ChatMessage{{Role: "system", Content: []ContentPart{TextPart{Text: "S"}}}}, []ChatMessage{{Role: "user", Content: []ContentPart{TextPart{Text: "H"}}}}, 2, 1},
		{"system then user then history", []ChatMessage{{Role: "system", Content: []ContentPart{TextPart{Text: "S"}}}, {Role: "user", Content: []ContentPart{TextPart{Text: "U"}}}}, []ChatMessage{{Role: "assistant", Content: []ContentPart{TextPart{Text: "A"}}}}, 3, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := spliceHistory(tt.rendered, tt.history)
			assert.Len(t, got, tt.wantLen)
			if len(tt.history) > 0 && tt.wantLen > 0 {
				// First non-system in rendered was at firstUserAt; history should be inserted there
				foundUser := false
				for i, m := range got {
					if m.Role != RoleSystem {
						foundUser = true
						assert.Equal(t, tt.firstUserAt, i, "first user index")
						break
					}
				}
				assert.True(t, foundUser)
			}
		})
	}
}

func TestMergeRequiredVars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		explicit []string
		fromTpl  []string
		want     []string
	}{
		{"both empty", nil, nil, nil},
		{"explicit only", []string{"a", "b"}, nil, []string{"a", "b"}},
		{"fromTpl only", nil, []string{"x", "y"}, []string{"x", "y"}},
		{"merged no dup", []string{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
		{"explicit first", []string{"a"}, []string{"a", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mergeRequiredVars(tt.explicit, tt.fromTpl)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractVarsFromTree(t *testing.T) {
	t.Parallel()
	mustParse := func(s string) *template.Template {
		tpl, err := template.New("").Parse(s)
		require.NoError(t, err)
		return tpl
	}
	tests := []struct {
		name string
		tpl  string
		want []string
	}{
		{"no vars", "plain text", nil},
		{"one var", "{{ .user_name }}", []string{"user_name"}},
		{"two vars", "{{ .a }} {{ .b }}", []string{"a", "b"}},
		{"Tools excluded", "{{ .user }} {{ .Tools }}", []string{"user"}},
		{"nested", "{{ .x }} {{ if .y }}{{ .z }}{{ end }}", []string{"x", "y", "z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tpl := mustParse(tt.tpl)
			got := extractVarsFromTree(tpl.Tree)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestAllVarsZeroForMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		merged map[string]any
		vars   []string
		want   bool
	}{
		{"no vars", map[string]any{"a": "x"}, nil, true},
		{"missing key", map[string]any{}, []string{"a"}, true},
		{"nil value", map[string]any{"a": nil}, []string{"a"}, true},
		{"zero string", map[string]any{"a": ""}, []string{"a"}, true},
		{"non-zero", map[string]any{"a": "x"}, []string{"a"}, false},
		{"mixed zero and missing", map[string]any{"a": ""}, []string{"a", "b"}, true},
		{"mixed zero and non-zero", map[string]any{"a": "", "b": "y"}, []string{"a", "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := allVarsZeroForMessage(tt.merged, tt.vars)
			assert.Equal(t, tt.want, got)
		})
	}
}
