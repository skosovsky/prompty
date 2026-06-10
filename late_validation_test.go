package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRequiredLateVars_RejectsEmptyBinding(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patient_dossier": map[string]any{
					"type": "string",
					"late": true,
				},
			},
			"required": []string{"patient_dossier"},
		}),
	}
	err := validateRequiredLateVars(schema, map[string]any{"patient_dossier": ""}, "late_agent")
	require.Error(t, err)
	var ve *VariableError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "patient_dossier", ve.Variable)
	assert.Equal(t, "late_agent", ve.Template)
	assert.ErrorIs(t, err, ErrMissingVariable)
}

func TestValidateRequiredLateVars_AcceptsNonEmptyBinding(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patient_dossier": map[string]any{
					"type": "string",
					"late": true,
				},
			},
			"required": []string{"patient_dossier"},
		}),
	}
	err := validateRequiredLateVars(schema, map[string]any{"patient_dossier": "chart-1"}, "late_agent")
	require.NoError(t, err)
}

func TestEnsureOptionalLateDefaults_FillsOptionalLate(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patient_dossier": map[string]any{
					"type": "string",
					"late": true,
				},
				"notes": map[string]any{
					"type": "string",
					"late": true,
				},
			},
			"required": []string{"patient_dossier"},
		}),
	}
	out := ensureOptionalLateDefaults(schema, map[string]any{})
	assert.Empty(t, out["notes"])
	assert.NotContains(t, out, "patient_dossier")
}

func TestEnsureOptionalLateDefaults_PreservesExisting(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notes": map[string]any{
					"type": "string",
					"late": true,
				},
			},
		}),
	}
	out := ensureOptionalLateDefaults(schema, map[string]any{"notes": "keep"})
	assert.Equal(t, "keep", out["notes"])
}

func TestValidateRequiredInputVars_SkipsLateFieldsFromAST(t *testing.T) {
	t.Parallel()
	schema := &SchemaDefinition{
		Schema: MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_query": map[string]any{"type": "string"},
				"patient_dossier": map[string]any{
					"type":           "string",
					"x-prompty-late": true,
				},
			},
			"required": []any{"user_query", "patient_dossier"},
		}),
	}
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{
			Role:    RoleUser,
			Content: TextContent("{{ .Input.user_query }} {{ .Input.patient_dossier }}"),
		}},
		WithInputSchema(schema),
	)
	require.NoError(t, err)
	tpl.RequiredVars = []string{"user_query", "patient_dossier"}
	err = validateRequiredInputVars(tpl, map[string]any{"user_query": "hi"})
	require.NoError(t, err, "late field must not be required at early execute")
}
