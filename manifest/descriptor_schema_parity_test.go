package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestDescriptorSchemaParity_ComposedConditional(t *testing.T) {
	t.Parallel()
	main := &RawManifest{
		ID: "composed_conditional_main",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			}),
		},
		Imports: []RawImport{{
			ID: "composed_child",
			Condition: &RawCondition{Match: map[string]any{
				"capabilities.workspace_enabled": true,
			}},
		}},
		Layers: []RawLayer{{
			ID:        "child_layer",
			ImportRef: "composed_child",
		}},
	}
	child := &RawManifest{
		ID: "composed_child",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"clinic_name": map[string]any{"type": "string"},
				},
			}),
		},
	}
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"composed_conditional_main": main,
		"composed_child":            child,
	}}

	conservativeDesc, err := BuildDescriptorFromRaw(main, &parseOpts{compose: &ComposeContext{
		Ctx: context.Background(), Loader: loader, Capabilities: nil,
	}})
	require.NoError(t, err)
	conservativeProps := schemaProps(t, conservativeDesc.InputSchema)
	assert.Contains(t, conservativeProps, "query")
	assert.Contains(t, conservativeProps, "clinic_name")

	runtimeOffDesc, err := BuildDescriptorFromRaw(main, &parseOpts{compose: &ComposeContext{
		Ctx: context.Background(), Loader: loader, Capabilities: map[string]any{
			"capabilities": map[string]any{"workspace_enabled": false},
		},
	}})
	require.NoError(t, err)
	runtimeProps := schemaProps(t, runtimeOffDesc.InputSchema)
	assert.Contains(t, runtimeProps, "query")
	assert.NotContains(t, runtimeProps, "clinic_name")

	childLate := &RawManifest{
		ID: "late_child",
		Messages: []RawMessage{{
			Role:    "system",
			Content: []RawContentPart{{Type: "text", Text: "child"}},
		}},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patient_dossier": map[string]any{"type": "string", "late": true},
				},
				"required": []any{"patient_dossier"},
			}),
		},
	}
	mainLate := &RawManifest{
		ID: "late_main",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			}),
		},
		Imports: []RawImport{{ID: "late_child"}},
		Messages: []RawMessage{{
			Role:    "user",
			Content: []RawContentPart{{Type: "text", Text: "{{ .Input.query }}"}},
		}},
	}
	lateLoader := &MemoryLoader{ByID: map[string]*RawManifest{
		"late_main":  mainLate,
		"late_child": childLate,
	}}
	lateDesc, err := BuildDescriptorFromRaw(mainLate, &parseOpts{compose: &ComposeContext{
		Ctx: context.Background(), Loader: lateLoader, Capabilities: nil,
	}})
	require.NoError(t, err)
	assert.Contains(t, lateDesc.RequiredInputVars, "query")
	assert.NotContains(t, lateDesc.RequiredInputVars, "patient_dossier")

	working, err := cloneRawManifest(mainLate)
	require.NoError(t, err)
	require.NoError(t, ExpandRawManifest(working, ComposeContext{
		Ctx: context.Background(), Loader: lateLoader, Capabilities: nil,
	}))
	tpl, err := BuildFromRaw(working, &parseOpts{})
	require.NoError(t, err)
	assert.Equal(t, lateDesc.RequiredInputVars, tpl.RequiredVars)
}

func schemaProps(t *testing.T, schema *prompty.SchemaDefinition) map[string]bool {
	t.Helper()
	if schema == nil {
		return nil
	}
	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	require.NoError(t, err)
	props, _ := doc["properties"].(map[string]any)
	out := make(map[string]bool, len(props))
	for name := range props {
		out[name] = true
	}
	return out
}
