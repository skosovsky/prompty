package gen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizePropForCodegen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "strips late markers and required",
			in: map[string]any{
				"type":           "string",
				"late":           true,
				"x-prompty-late": true,
				"required":       true,
			},
			want: map[string]any{"type": "string"},
		},
		{
			name: "preserves other keys",
			in: map[string]any{
				"type":    "string",
				"default": "hello",
			},
			want: map[string]any{
				"type":    "string",
				"default": "hello",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sanitizePropForCodegen(tc.in))
		})
	}
}

func TestFilterRequired(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		required map[string]bool
		props    map[string]any
		want     []string
	}{
		{
			name:     "empty required",
			required: nil,
			props:    map[string]any{"a": map[string]any{}},
			want:     nil,
		},
		{
			name:     "keeps only props present in subset",
			required: map[string]bool{"a": true, "b": true, "c": true},
			props:    map[string]any{"a": map[string]any{}, "c": map[string]any{}},
			want:     []string{"a", "c"},
		},
		{
			name:     "sorted keys",
			required: map[string]bool{"z": true, "a": true},
			props:    map[string]any{"z": map[string]any{}, "a": map[string]any{}},
			want:     []string{"a", "z"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, filterRequired(tc.required, tc.props))
		})
	}
}

func TestSplitEarlyLateInputSchema(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		schema      map[string]any
		wantEarly   map[string]any
		wantLate    map[string]any
		wantHasLate bool
	}{
		{
			name:        "nil schema",
			schema:      nil,
			wantEarly:   nil,
			wantLate:    nil,
			wantHasLate: false,
		},
		{
			name: "no properties",
			schema: map[string]any{
				"type": "object",
			},
			wantEarly: map[string]any{
				"type": "object",
			},
			wantLate:    nil,
			wantHasLate: false,
		},
		{
			name: "splits late property",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_query": map[string]any{"type": "string"},
					"patient_dossier": map[string]any{
						"type":     "string",
						"late":     true,
						"required": true,
					},
				},
				"required": []any{"user_query", "patient_dossier"},
			},
			wantEarly: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_query": map[string]any{"type": "string"},
				},
				"required": []string{"user_query"},
			},
			wantLate: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patient_dossier": map[string]any{"type": "string"},
				},
				"required": []string{"patient_dossier"},
			},
			wantHasLate: true,
		},
		{
			name: "all early",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
			wantEarly: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []string{"query"},
			},
			wantLate:    nil,
			wantHasLate: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			early, late, hasLate := splitEarlyLateInputSchema(tc.schema)
			assert.Equal(t, tc.wantHasLate, hasLate)
			assert.Equal(t, tc.wantEarly, early)
			assert.Equal(t, tc.wantLate, late)
		})
	}
}
