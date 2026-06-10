package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestMatchCondition_DotPathStrictEquality(t *testing.T) {
	t.Parallel()
	ctx := map[string]any{
		"capabilities": map[string]any{
			"workspace_enabled": true,
		},
	}
	ok, err := MatchCondition(map[string]any{"capabilities.workspace_enabled": true}, ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = MatchCondition(map[string]any{"capabilities.workspace_enabled": 1}, ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = MatchCondition(map[string]any{"capabilities.missing": true}, ctx)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMergeInputSchemas_UnionImportFields(t *testing.T) {
	t.Parallel()
	local := &prompty.SchemaDefinition{
		Schema: prompty.MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		}),
	}
	imported := &RawManifest{
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"clinic_name": map[string]any{"type": "string"},
				},
			}),
		},
	}
	merged, err := MergeInputSchemas(local, imported)
	require.NoError(t, err)
	doc, err := prompty.JSONDocumentAsMap(merged.Schema)
	require.NoError(t, err)
	props, ok := doc["properties"].(map[string]any)
	require.True(t, ok)
	_, hasQuery := props["query"]
	_, hasClinic := props["clinic_name"]
	assert.True(t, hasQuery)
	assert.True(t, hasClinic)
}

func TestExpandRawManifest_LayersAndConditionalImport(t *testing.T) {
	t.Parallel()
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"workspace_format": {
			ID: "workspace_format",
			Messages: []RawMessage{
				{Role: "system", Content: []RawContentPart{{Type: "text", Text: "workspace rules"}}},
			},
			InputSchema: &prompty.SchemaDefinition{
				Schema: prompty.MustJSONDocumentFromMap(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"clinic_name": map[string]any{"type": "string"},
					},
				}),
			},
		},
	}}
	main := &RawManifest{
		ID: "main_agent",
		Imports: []RawImport{{
			ID: "workspace_format",
			Condition: &RawCondition{Match: map[string]any{
				"capabilities.workspace_enabled": true,
			}},
		}},
		Layers: []RawLayer{
			{
				ID: "base_system", Role: "system",
				Content: []RawContentPart{{Type: "text", Text: "You are an assistant."}},
			},
			{ID: "workspace_layer", ImportRef: "workspace_format"},
			{
				ID: "user_turn", Role: "user",
				Content: []RawContentPart{{Type: "text", Text: "{{ .Input.query }}"}},
			},
		},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			}),
		},
	}
	ctx := ComposeContext{
		Ctx: context.Background(),
		Capabilities: map[string]any{
			"capabilities": map[string]any{"workspace_enabled": true},
		},
		Loader: loader,
	}
	require.NoError(t, ExpandRawManifest(main, ctx))
	require.Len(t, main.Messages, 3)
	assert.Equal(t, "You are an assistant.", main.Messages[0].Content[0].Text)
	assert.Equal(t, "workspace rules", main.Messages[1].Content[0].Text)

	props, err := prompty.JSONDocumentAsMap(main.InputSchema.Schema)
	require.NoError(t, err)
	p, _ := props["properties"].(map[string]any)
	_, hasClinic := p["clinic_name"]
	assert.True(t, hasClinic)

	tpl, err := BuildFromRaw(main, &parseOpts{compose: &ctx})
	require.NoError(t, err)
	plan, err := prompty.NewRenderPlanFromStruct(tpl, struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)
	exec, err := plan.Execute(t.Context())
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3)
	assert.Equal(t, "hi", exec.Messages[2].Content[0].(prompty.TextPart).Text)
}

func TestExpandRawManifest_SkipsInactiveImport(t *testing.T) {
	t.Parallel()
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"workspace_format": {
			ID: "workspace_format",
			Messages: []RawMessage{
				{Role: "system", Content: []RawContentPart{{Type: "text", Text: "workspace"}}},
			},
		},
	}}
	main := &RawManifest{
		ID: "main_agent",
		Imports: []RawImport{{
			ID: "workspace_format",
			Condition: &RawCondition{Match: map[string]any{
				"capabilities.workspace_enabled": true,
			}},
		}},
		Layers: []RawLayer{
			{ID: "base", Role: "system", Content: []RawContentPart{{Type: "text", Text: "base"}}},
			{ID: "ws", ImportRef: "workspace_format"},
		},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{"type": "object", "properties": map[string]any{}}),
		},
	}
	require.NoError(t, ExpandRawManifest(main, ComposeContext{
		Ctx:          context.Background(),
		Capabilities: map[string]any{"capabilities": map[string]any{"workspace_enabled": false}},
		Loader:       loader,
	}))
	require.Len(t, main.Messages, 1)
	assert.Equal(t, "base", main.Messages[0].Content[0].Text)
}

func TestExpandRawManifest_UnknownImportRefErrors(t *testing.T) {
	t.Parallel()
	main := &RawManifest{
		ID: "main",
		Layers: []RawLayer{
			{ID: "base", Role: "system", Content: []RawContentPart{{Type: "text", Text: "base"}}},
			{ID: "bad", ImportRef: "missing_child"},
		},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{"type": "object", "properties": map[string]any{}}),
		},
	}
	err := ExpandRawManifest(main, ComposeContext{
		Loader: &MemoryLoader{ByID: map[string]*RawManifest{}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown import_ref")
}

func TestCheckImportCycles_DetectsGraphCycle(t *testing.T) {
	t.Parallel()
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"a": {ID: "a", Imports: []RawImport{{ID: "b"}}},
		"b": {ID: "b", Imports: []RawImport{{ID: "a"}}},
	}}
	err := checkImportCycles(context.Background(), []RawImport{{ID: "a"}}, loader, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic import")
}

func TestExpandRawManifest_InactiveImportMissingChild_OK(t *testing.T) {
	t.Parallel()
	main := &RawManifest{
		ID: "main_agent",
		Imports: []RawImport{{
			ID: "missing_workspace",
			Condition: &RawCondition{Match: map[string]any{
				"capabilities.workspace_enabled": true,
			}},
		}},
		Layers: []RawLayer{
			{ID: "base", Role: "system", Content: []RawContentPart{{Type: "text", Text: "base"}}},
			{ID: "ws", ImportRef: "missing_workspace"},
		},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{"type": "object", "properties": map[string]any{}}),
		},
	}
	require.NoError(t, ExpandRawManifest(main, ComposeContext{
		Ctx:          context.Background(),
		Capabilities: map[string]any{"capabilities": map[string]any{"workspace_enabled": false}},
		Loader:       &MemoryLoader{ByID: map[string]*RawManifest{}},
	}))
	require.Len(t, main.Messages, 1)
	assert.Nil(t, main.Layers)
	assert.Nil(t, main.Imports)
}

func TestExpandRawManifest_MergeConflictPreservesRaw(t *testing.T) {
	t.Parallel()
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"child": {
			ID: "child",
			InputSchema: &prompty.SchemaDefinition{
				Schema: prompty.MustJSONDocumentFromMap(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string", "format": "uuid"},
					},
				}),
			},
		},
	}}
	main := &RawManifest{
		ID:      "main",
		Imports: []RawImport{{ID: "child"}},
		Layers: []RawLayer{
			{ID: "base", Role: "system", Content: []RawContentPart{{Type: "text", Text: "base"}}},
		},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "format": "email"},
				},
			}),
		},
	}
	err := ExpandRawManifest(main, ComposeContext{Ctx: context.Background(), Loader: loader})
	require.Error(t, err)
	require.Len(t, main.Layers, 1)
	require.Len(t, main.Imports, 1)
	assert.Empty(t, main.Messages)
}

func TestResolveEffectiveInputSchema_TransitiveUnion(t *testing.T) {
	t.Parallel()
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"b": {
			ID:      "b",
			Imports: []RawImport{{ID: "c"}},
			InputSchema: &prompty.SchemaDefinition{
				Schema: prompty.MustJSONDocumentFromMap(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"field_b": map[string]any{"type": "string"},
					},
				}),
			},
		},
		"c": {
			ID: "c",
			InputSchema: &prompty.SchemaDefinition{
				Schema: prompty.MustJSONDocumentFromMap(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"field_c": map[string]any{"type": "string"},
					},
				}),
			},
		},
	}}
	main := &RawManifest{
		ID:      "a",
		Imports: []RawImport{{ID: "b"}},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"field_a": map[string]any{"type": "string"},
				},
			}),
		},
	}
	merged, err := ResolveEffectiveInputSchema(context.Background(), main, loader)
	require.NoError(t, err)
	doc, err := prompty.JSONDocumentAsMap(merged.Schema)
	require.NoError(t, err)
	props, _ := doc["properties"].(map[string]any)
	_, hasA := props["field_a"]
	_, hasB := props["field_b"]
	_, hasC := props["field_c"]
	assert.True(t, hasA)
	assert.True(t, hasB)
	assert.True(t, hasC)
}

func TestExpandRawManifest_NestedChildLayers(t *testing.T) {
	t.Parallel()
	loader := &MemoryLoader{ByID: map[string]*RawManifest{
		"child": {
			ID: "child",
			Layers: []RawLayer{
				{ID: "inner", Role: "system", Content: []RawContentPart{{Type: "text", Text: "inner"}}},
			},
		},
	}}
	main := &RawManifest{
		ID:      "main",
		Imports: []RawImport{{ID: "child"}},
		Layers: []RawLayer{
			{ID: "outer", Role: "system", Content: []RawContentPart{{Type: "text", Text: "outer"}}},
			{ID: "nested", ImportRef: "child"},
		},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{"type": "object", "properties": map[string]any{}}),
		},
	}
	require.NoError(t, ExpandRawManifest(main, ComposeContext{Ctx: context.Background(), Loader: loader}))
	require.Len(t, main.Messages, 2)
	assert.Equal(t, "outer", main.Messages[0].Content[0].Text)
	assert.Equal(t, "inner", main.Messages[1].Content[0].Text)
}

func TestMergeRequired_DeterministicOrder(t *testing.T) {
	t.Parallel()
	local := &prompty.SchemaDefinition{
		Schema: prompty.MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"z_field": map[string]any{"type": "string"},
				"a_field": map[string]any{"type": "string"},
			},
			"required": []any{"z_field", "a_field"},
		}),
	}
	imported := &RawManifest{
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"m_field": map[string]any{"type": "string"},
				},
				"required": []any{"m_field"},
			}),
		},
	}
	merged, err := MergeInputSchemas(local, imported)
	require.NoError(t, err)
	doc, err := prompty.JSONDocumentAsMap(merged.Schema)
	require.NoError(t, err)
	switch r := doc["required"].(type) {
	case []string:
		assert.Equal(t, []string{"a_field", "m_field", "z_field"}, r)
	case []any:
		names := make([]string, len(r))
		for i, e := range r {
			names[i] = e.(string)
		}
		assert.Equal(t, []string{"a_field", "m_field", "z_field"}, names)
	default:
		t.Fatalf("unexpected required type %T", doc["required"])
	}
}

func TestMergeInputSchemas_ConflictingFormatErrors(t *testing.T) {
	t.Parallel()
	local := &prompty.SchemaDefinition{
		Schema: prompty.MustJSONDocumentFromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "format": "email"},
			},
		}),
	}
	imported := &RawManifest{
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "format": "uuid"},
				},
			}),
		},
	}
	_, err := MergeInputSchemas(local, imported)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting type")
}

func TestExpandRawManifest_LayersAndMessagesRejected(t *testing.T) {
	t.Parallel()
	raw := &RawManifest{
		ID: "mixed",
		Layers: []RawLayer{
			{ID: "sys", Role: "system", Content: []RawContentPart{{Type: "text", Text: "rules"}}},
		},
		Messages: []RawMessage{
			{Role: "user", Content: []RawContentPart{{Type: "text", Text: "hi"}}},
		},
	}
	err := ExpandRawManifest(raw, ComposeContext{Loader: &MemoryLoader{ByID: map[string]*RawManifest{}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both layers and messages")
}

func TestExpandRawManifest_InlineLayerRequiresID(t *testing.T) {
	t.Parallel()
	raw := &RawManifest{
		ID: "missing_id",
		Layers: []RawLayer{
			{Role: "system", Content: []RawContentPart{{Type: "text", Text: "rules"}}},
		},
	}
	err := ExpandRawManifest(raw, ComposeContext{Loader: &MemoryLoader{ByID: map[string]*RawManifest{}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline layer requires id")
}
