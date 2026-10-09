package gen

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dave/jennifer/jen"

	"github.com/skosovsky/prompty"
)

const legacyClientTypeName = "LLM" + "Client"

// --- Consts tests ---

func TestGenerateConstsPackage(t *testing.T) {
	f, err := GenerateConstsPackage("prompts", []string{"support_agent", "greeter"})
	assertGenerationSucceeded(t, err, "GenerateConstsPackage: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	assertGeneratedContains(t, out, "type PromptID string", "expected type PromptID string")
	assertGeneratedContains(t, out, "Greeter", "expected Greeter const")
	assertGeneratedContains(t, out, "SupportAgent", "expected SupportAgent const")
	assertGeneratedContains(t, out, "func AllPromptIDs()", "expected AllPromptIDs func")
	// Sorted order: greeter before support_agent
	if strings.Index(out, "Greeter") > strings.Index(out, "SupportAgent") {
		t.Error("expected deterministic sorted order (Greeter before SupportAgent); got SupportAgent before Greeter")
	}
}

// --- Shared types tests ---

func TestGenerateSharedTypes(t *testing.T) {
	specs := []*PromptSpec{sharedTestSpec("support_agent"), sharedTestSpec("greeter")}
	f, err := GenerateSharedTypes("prompts", specs)
	assertGenerationSucceeded(t, err, "GenerateSharedTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	assertGeneratedContains(t, out, "type PromptID string", "expected type PromptID string")
	assertGeneratedContains(t, out, "var validate = validator.New()", "expected package-level validate singleton")
	assertGeneratedContains(t, out, "type PromptCatalog interface", "expected PromptCatalog interface")
	assertGeneratedContains(
		t,
		out,
		"Descriptor(ctx context.Context, id PromptID)",
		"expected Descriptor method in PromptCatalog",
	)
	assertGeneratedContains(t, out, "func NewPromptCatalog(", "expected NewPromptCatalog")
	if strings.Contains(out, "RenderSupportAgent") || strings.Contains(out, "RenderGreeter") {
		t.Error("render-first catalog methods must not be generated")
	}
	if strings.Contains(out, "RenderByID") {
		t.Error("legacy RenderByID must not be generated")
	}
	if strings.Contains(out, "renderFromAny") {
		t.Error("legacy renderFromAny must not be generated")
	}
	assertGeneratedContains(
		t,
		out,
		"PromptCatalogRegistry",
		"expected NewPromptCatalog to accept PromptCatalogRegistry",
	)
	assertGeneratedContains(t, out, "type PromptIndex struct", "expected generated PromptIndex")
	assertGeneratedContains(t, out, "func (i PromptIndex) Lookup(", "expected PromptIndex Lookup")
	assertGeneratedContains(t, out, "NewSupportAgentRecipe", "expected typed recipe constructor in catalog")
	assertGeneratedContains(t, out, "NewRecipeFromJSON", "expected prompt-index JSON recipe decoder")
	assertGeneratedContains(t, out, "DecodeRecipeCheckpoint", "expected prompt-index checkpoint decoder")
	assertGeneratedContains(t, out, "CheckpointJSON()", "expected generic recipe checkpoint JSON method")
	assertGeneratedContains(t, out, "ResponseFormat()", "expected generic recipe structured output contract method")
	assertGeneratedContains(t, out, "JSONSchema()", "expected generic recipe JSON schema contract method")
	if strings.Contains(out, "ToolContract") {
		t.Error("legacy name-only ToolContract must not be generated")
	}
	assertGeneratedContains(t, out, "func AllPromptIDs()", "expected AllPromptIDs")
	if strings.Contains(out, legacyClientTypeName) {
		t.Error("DoD: must not contain the legacy client type")
	}
}

func TestGenerateSharedTypes_RequiresDescriptorDigest(t *testing.T) {
	_, err := GenerateSharedTypes("prompts", []*PromptSpec{{ID: "support_agent"}})

	if err == nil {
		t.Fatal("expected descriptor digest requirement error")
	}
	if !strings.Contains(err.Error(), "descriptor digest is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Manifest types tests ---

func TestGenerateManifestTypes_SupportAgent(t *testing.T) {
	spec := &PromptSpec{
		ID: "support_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_name":   map[string]any{"type": "string"},
					"user_query": map[string]any{"type": "string"},
				},
				"required": []any{"user_query"},
			}),
		},
	}

	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "const SupportAgent PromptID = \"support_agent\"") {
		t.Error("expected SupportAgent const")
	}
	assertGeneratedContains(t, out, "type SupportAgentInput struct", "expected SupportAgentInput struct")
	assertGeneratedContains(t, out, "type SupportAgentPrompt struct", "expected SupportAgentPrompt type")
	if strings.Contains(out, "func (p *SupportAgentPrompt) Render(") {
		t.Error("render-first prompt method must not be generated")
	}
	assertGeneratedContains(
		t,
		out,
		"func (p *SupportAgentPrompt) RequiredTools()",
		"expected RequiredTools method on SupportAgentPrompt",
	)
	assertGeneratedContains(t, out, "type SupportAgentRecipe struct", "expected SupportAgentRecipe type")
	assertGeneratedContains(
		t,
		out,
		"func (p *SupportAgentPrompt) NewRecipe(",
		"expected NewRecipe method on SupportAgentPrompt",
	)
	assertGeneratedContains(
		t,
		out,
		"func NewSupportAgentRecipeFromCheckpoint(",
		"expected generated checkpoint restore constructor",
	)
	assertGeneratedContains(t, out, "func (r SupportAgentRecipe) PromptID()", "expected recipe PromptID method")
	assertGeneratedContains(
		t,
		out,
		"func (r SupportAgentRecipe) ResponseFormat()",
		"expected generated ResponseFormat method",
	)
	assertGeneratedContains(
		t,
		out,
		"ErrStructuredOutputUnavailable",
		"expected unavailable structured output error for prompts without response_format",
	)
	assertGeneratedContains(t, out, "PromptRecipeNoLate[SupportAgentInput]", "expected no-late prompt recipe wrapper")
	if strings.Contains(out, "SupportAgentComposeContext") {
		t.Error("compose context must not be generated without compose conditions")
	}
	if strings.Contains(out, "NewRecipeWithComposeContext") {
		t.Error("compose recipe constructor must not be generated without compose conditions")
	}
	assertGeneratedContains(t, out, "func (p *SupportAgentPrompt) ID()", "expected ID method on SupportAgentPrompt")
	assertGeneratedContains(t, out, "validate.Struct", "expected input validation")
	if strings.Contains(out, "p.registry.Plan") {
		t.Error("NewRecipe must not build a render plan")
	}
	assertGeneratedContains(t, out, "PlanInputFrom", "expected PlanInputFrom call")
	if !strings.Contains(out, `prompt:"`) {
		t.Error("expected prompt tags in generated input struct")
	}
	assertGeneratedContains(t, out, "string(SupportAgent)", "expected string(PromptID) for manifest descriptor lookup")
	if strings.Contains(out, "build render plan") {
		t.Error("render-plan build wrapper must not be generated")
	}
	if strings.Contains(out, "renderFromAny") {
		t.Error("legacy renderFromAny must not be generated")
	}
	if strings.Contains(out, "Format(ctx,") {
		t.Error("DoD: generated code must not use the removed Format(ctx, ...) signature")
	}
	if strings.Contains(out, legacyClientTypeName) {
		t.Error("DoD: must not contain the legacy client type")
	}
	if strings.Contains(out, "ExecuteWithStructuredOutput") {
		t.Error("DoD: must not contain ExecuteWithStructuredOutput")
	}
	if strings.Contains(out, "func (p *SupportAgentPrompt) WithLate(") {
		t.Error("legacy WithLate(plan, late) must not be generated")
	}
}

func TestGenerateManifestTypes_ComposeConditionRequiresContextPayload(t *testing.T) {
	spec := &PromptSpec{
		ID: "compose_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			}),
		},
		ComposeConditions: []ComposeConditionSpec{
			{Key: "routing.enabled", Kind: "bool"},
		},
	}

	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "func (p *ComposeAgentPrompt) NewRecipe(") {
		t.Error("contextless NewRecipe must not be generated for compose-required prompts")
	}
	assertGeneratedContains(
		t,
		out,
		"func (p *ComposeAgentPrompt) NewRecipeWithComposeContext(",
		"expected compose-aware recipe constructor",
	)
	assertGeneratedContains(
		t,
		out,
		"type ComposeAgentRecipePayload struct",
		"expected compose-aware JSON recipe payload",
	)
	if !strings.Contains(out, "Compose *ComposeAgentComposeContext `json:\"compose\"`") {
		t.Error("expected required compose payload pointer")
	}
	if !strings.Contains(out, "RoutingEnabled *bool `json:\"routing_enabled\"`") {
		t.Error("expected pointer compose condition field with presence semantics")
	}
	assertGeneratedContains(
		t,
		out,
		"func NewComposeAgentComposeContext(routingEnabled bool)",
		"expected compose context constructor",
	)
	assertGeneratedContains(t, out, "ValidateComposeContext() error", "expected compose context validator")
}

func TestGenerateManifestTypes_ComposeConditionFieldCollision(t *testing.T) {
	spec := &PromptSpec{
		ID: "compose_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
		ComposeConditions: []ComposeConditionSpec{
			{Key: "a.b", Kind: "bool"},
			{Key: "a-b", Kind: "bool"},
		},
	}

	_, err := GenerateManifestTypes(spec, "prompts")
	if err == nil {
		t.Fatal("expected compose condition field collision error")
	}
	if !strings.Contains(err.Error(), "field name") || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("expected field collision error, got: %v", err)
	}
}

func TestGenerateManifestTypes_NoResponseFormat(t *testing.T) {
	spec := &PromptSpec{
		ID: "simple",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "string"},
				},
				"required": []any{"q"},
			}),
		},
		// No ResponseFormat
	}

	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "SimpleOutput") {
		t.Error("Output type must NOT be generated when response_format absent")
	}
}

func TestGenerateManifestTypes_WithResponseFormat(t *testing.T) {
	spec := &PromptSpec{
		ID: "greeter",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			}),
		},
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
				"required": []any{"message"},
			}),
		},
	}

	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	assertGeneratedContains(t, out, "type GreeterOutput struct", "expected GreeterOutput when response_format present")
	assertGeneratedContains(
		t,
		out,
		"func (r GreeterRecipe) ResponseFormat()",
		"expected generated ResponseFormat method",
	)
	assertGeneratedContains(t, out, "func (r GreeterRecipe) JSONSchema()", "expected generated JSONSchema method")
	if strings.Contains(out, "ErrStructuredOutputUnavailable") {
		t.Error("response-format prompt must not return unavailable schema")
	}
	wantStaticSchema := `prompty.JSONDocument(` +
		`"{\"properties\":{\"message\":{\"type\":\"string\"}},\"required\":[\"message\"],\"type\":\"object\"}")`
	if !strings.Contains(out, wantStaticSchema) {
		t.Error("expected generated static schema document")
	}
	if strings.Contains(out, "(*prompty.RenderPlan, error)") {
		t.Error("response-format prompts must still use recipe-first generated API")
	}
}

// --- Nested type naming regression tests ---

func TestGenerateManifestTypes_NestedObjectInOutput(t *testing.T) {
	spec := &PromptSpec{
		ID: "router",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entities": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{"type": "string"},
						},
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	assertGeneratedContains(
		t,
		out,
		"type RouterOutputEntities struct",
		"expected type RouterOutputEntities struct for nested object in Output",
	)
	assertGeneratedContains(
		t,
		out,
		"Entities *RouterOutputEntities",
		"expected field Entities *RouterOutputEntities (parent-based naming)",
	)
}

func TestGenerateManifestTypes_NestedObjectInInput(t *testing.T) {
	spec := &PromptSpec{
		ID: "router",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payload": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	assertGeneratedContains(
		t,
		out,
		"type RouterInputPayload struct",
		"expected type RouterInputPayload struct for nested object in Input",
	)
	assertGeneratedContains(
		t,
		out,
		"Payload *RouterInputPayload",
		"expected field Payload *RouterInputPayload (parent-based naming)",
	)
	if strings.Contains(out, `vars["payload"]`) {
		t.Error("must not generate legacy vars map assignments")
	}
}

func TestGenerateManifestTypes_ArrayOfObjectInOutput(t *testing.T) {
	spec := &PromptSpec{
		ID: "router",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"users": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{"type": "string"},
							},
						},
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	assertGeneratedContains(
		t,
		out,
		"type RouterOutputUsersItem struct",
		"expected type RouterOutputUsersItem struct for array-of-object in Output",
	)
	assertGeneratedContains(
		t,
		out,
		"Users []RouterOutputUsersItem",
		"expected field Users []RouterOutputUsersItem (not ...OutputItemUsers)",
	)
}

func TestGenerateManifestTypes_EmptyInputSchema(t *testing.T) {
	spec := &PromptSpec{
		ID: "no_vars",
		// No InputSchema
	}

	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	assertGeneratedContains(t, out, "type NoVarsInput struct", "expected empty Input struct")
	if strings.Contains(out, "func (p *NoVarsPrompt) Render(") {
		t.Error("render-first prompt method must not be generated")
	}
	assertEmptyInputRecipeBindingContract(t, out)
}

func TestGenerateManifestTypes_RequiredTools(t *testing.T) {
	spec := &PromptSpec{
		ID:            "doctor_agent",
		RequiredTools: []string{"doctor_search_knowledge_base", "get_current_time"},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"doctor_search_knowledge_base"`) {
		t.Error("expected doctor_search_knowledge_base in RequiredTools return")
	}
	if !strings.Contains(out, `"get_current_time"`) {
		t.Error("expected get_current_time in RequiredTools return")
	}
	assertEmptyInputRecipeBindingContract(t, out)
}

func assertEmptyInputRecipeBindingContract(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "PlanInputFrom") {
		t.Error("empty input schema must not call PlanInputFrom")
	}
}

// --- Schema regression tests ---

func TestGenerateManifestTypes_Dive(t *testing.T) {
	spec := &PromptSpec{
		ID: "orders",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id": map[string]any{"type": "string"},
							},
							"required": []any{"id"},
						},
					},
				},
				"required": []any{"items"},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(t, out, "dive", "expected validate dive tag for array of objects")
}

func TestGenerateManifestTypes_AdditionalProperties(t *testing.T) {
	spec := &PromptSpec{
		ID: "meta",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"extra": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(
		t,
		out,
		"map[string]string",
		"expected map[string]string for additionalProperties with type string",
	)
}

func TestGenerateManifestTypes_OpenObjectWithoutTypedAdditionalPropertiesFails(t *testing.T) {
	spec := &PromptSpec{
		ID: "bag",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"meta": map[string]any{
						"type": "object",
					},
				},
			}),
		},
	}
	_, err := GenerateManifestTypes(spec, "prompts")
	if err == nil {
		t.Fatal("expected error for object without properties and typed additionalProperties")
	}
	if !strings.Contains(err.Error(), "requires typed additionalProperties") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateManifestTypes_UntypedPropertyFails(t *testing.T) {
	spec := &PromptSpec{
		ID: "flex",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payload": map[string]any{}, // no type -> strict codegen error
				},
			}),
		},
	}
	_, err := GenerateManifestTypes(spec, "prompts")
	if err == nil {
		t.Fatal("expected error for property without type keyword")
	}
	if !strings.Contains(err.Error(), "missing or unsupported type keyword") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateManifestTypes_Oneof(t *testing.T) {
	spec := &PromptSpec{
		ID: "status",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"state": map[string]any{
						"type": "string",
						"enum": []any{"pending", "done"},
					},
				},
				"required": []any{"state"},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(t, out, "oneof=pending done", "expected oneof validate tag from enum")
}

// --- Regression tests ---

func TestGenerateManifestTypes_RequiredBoolWithFalse(t *testing.T) {
	spec := &PromptSpec{
		ID: "flags",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean"},
				},
				"required": []any{"enabled"},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(t, out, "*bool", "required bool must be *bool for presence semantics")
	if !strings.Contains(out, "validate:\"required\"") && !strings.Contains(out, "validate:`required`") {
		t.Error("required bool must have validate required tag")
	}
	if strings.Contains(out, "vars[") {
		t.Error("must not generate legacy vars map assignments")
	}
}

func TestGenerateManifestTypes_MinItemsMaxItems(t *testing.T) {
	spec := &PromptSpec{
		ID: "list",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ids": map[string]any{
						"type":     "array",
						"minItems": 1,
						"maxItems": 10,
						"items":    map[string]any{"type": "string"},
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "min=1") || !strings.Contains(out, "max=10") {
		t.Error("expected minItems/maxItems as min/max validate tags")
	}
}

func TestGenerateManifestTypes_RootObjectWithoutProperties(t *testing.T) {
	spec := &PromptSpec{
		ID: "empty_input",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(
		t,
		out,
		"type EmptyInputInput struct",
		"expected empty Input struct for root object without properties",
	)
	assertEmptyInputRecipeBindingContract(t, out)
}

func TestGenerateManifestTypes_ArrayOfArray(t *testing.T) {
	spec := &PromptSpec{
		ID: "matrix",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"rows": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"val": map[string]any{"type": "integer"},
								},
							},
						},
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(t, out, "[][]", "expected array-of-array nested type")
}

func TestGenerateManifestTypes_SpecialCharsInKeys(t *testing.T) {
	spec := &PromptSpec{
		ID: "special",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user@name": map[string]any{"type": "string"}, // @ -> _
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	// user@name -> sanitize -> user_name -> pascal -> UserName
	if !strings.Contains(out, "UserName") || !strings.Contains(out, "user@name") {
		t.Error("expected sanitized Go name for key with special chars")
	}
}

func TestGenerateManifestTypes_IdWithDigitPrefix(t *testing.T) {
	spec := &PromptSpec{
		ID: "2fa_prompt",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{"type": "string"},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(t, out, "X2faPrompt", "expected id starting with digit to get X prefix")
}

func TestGenerateManifestTypes_OptionalStringNoDefaultElseEmptyString(t *testing.T) {
	spec := &PromptSpec{
		ID: "sales",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string"},
					"subtitle": map[string]any{
						"type": "string",
					},
				},
				"required": []any{"title"},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `vars["subtitle"]`) {
		t.Errorf("must not generate legacy vars map assignment for subtitle; got:\n%s", out)
	}
}

func TestGenerateManifestTypes_OptionalArrayNoDefaultElseNil(t *testing.T) {
	spec := &PromptSpec{
		ID: "tagged",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"tags": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"required": []any{"name"},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `vars["tags"]`) {
		t.Errorf("must not generate legacy vars map assignment for tags; got:\n%s", out)
	}
}

func TestGenerateManifestTypes_Default(t *testing.T) {
	spec := &PromptSpec{
		ID: "greeter",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"greeting": map[string]any{
						"type":    "string",
						"default": "Hello",
					},
				},
				"required": []any{"name"},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	assertGeneratedContains(t, out, "type GreeterInput struct", "expected generated input struct")
	assertGeneratedContains(t, out, "if input.Greeting == nil", "expected default guard for optional greeting")
	if !strings.Contains(out, `v := "Hello"`) {
		t.Error("expected literal default assignment for greeting")
	}
	if strings.Contains(out, `vars["greeting"]`) {
		t.Error("must not generate legacy vars default assignment")
	}
}

func TestGenerateManifestTypes_Default_UnsupportedTypeFailsFast(t *testing.T) {
	spec := &PromptSpec{
		ID: "with_array_default",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tags": map[string]any{
						"type":    "array",
						"items":   map[string]any{"type": "string"},
						"default": []any{"a", "b"},
					},
				},
			}),
		},
	}

	_, err := GenerateManifestTypes(spec, "prompts")
	if err == nil {
		t.Fatal("expected error for unsupported array default")
	}
	if !strings.Contains(err.Error(), `unsupported type "array"`) {
		t.Fatalf("expected unsupported array type error, got: %v", err)
	}
}

func TestGenerateManifestTypes_Default_InvalidScalarLiteralFailsFast(t *testing.T) {
	spec := &PromptSpec{
		ID: "bad_default_literal",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{
						"type":    "boolean",
						"default": "yes",
					},
				},
			}),
		},
	}

	_, err := GenerateManifestTypes(spec, "prompts")
	if err == nil {
		t.Fatal("expected error for invalid boolean default literal")
	}
	if !strings.Contains(err.Error(), "expected boolean default value") {
		t.Fatalf("expected boolean default value error, got: %v", err)
	}
}

func TestGenerateManifestTypes_FormatMessages(t *testing.T) {
	spec := &PromptSpec{
		ID: "message_history_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_history": map[string]any{
						"type":   "array",
						"format": "messages",
					},
					"query": map[string]any{"type": "string"},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ChatHistory []prompty.ChatMessage") {
		t.Errorf("expected []prompty.ChatMessage for format messages, got:\n%s", out)
	}
	if !strings.Contains(out, `prompt:"chat_history"`) {
		t.Error("expected prompt tag chat_history")
	}
}

func TestGenerateManifestTypes_LateInput(t *testing.T) {
	spec := &PromptSpec{
		ID: "main_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_name": map[string]any{"type": "string"},
					"patient_dossier": map[string]any{
						"type": "string",
						"late": true,
					},
				},
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	assertGeneratedContains(t, out, "type MainAgentInput struct", "expected MainAgentInput")
	assertGeneratedContains(t, out, "UserName", "expected UserName on MainAgentInput")
	idx := strings.Index(out, "type MainAgentInput struct")
	lateIdx := strings.Index(out, "type MainAgentLateInput struct")
	if lateIdx < 0 {
		t.Fatal("expected MainAgentLateInput")
	}
	if idx >= 0 && strings.Contains(out[idx:lateIdx], "PatientDossier") {
		t.Error("late field must not appear on MainAgentInput")
	}
	assertGeneratedContains(t, out, "type MainAgentLateInput struct", "expected MainAgentLateInput")
	if !strings.Contains(out, `prompt:"patient_dossier"`) {
		t.Error("expected prompt tag on late field")
	}
	assertGeneratedContains(t, out, "func (r MainAgentRecipe) BindLate(", "expected recipe-owned BindLate")
	assertGeneratedContains(
		t,
		out,
		"PromptRecipe[MainAgentInput, MainAgentLateInput]",
		"expected typed prompt recipe wrapper",
	)
	if strings.Contains(out, "func (p *MainAgentPrompt) WithLate(") {
		t.Error("legacy WithLate(plan, late) must not be generated")
	}
}

func TestGenerateManifestTypes_LateRequiredInput(t *testing.T) {
	spec := &PromptSpec{
		ID: "late_required_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
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
			}),
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	lateIdx := strings.Index(out, "type LateRequiredAgentLateInput struct")
	if lateIdx < 0 {
		t.Fatal("expected LateRequiredAgentLateInput")
	}
	lateBlock := out[lateIdx:]
	if !strings.Contains(lateBlock, "PatientDossier") || !strings.Contains(lateBlock, `validate:"required"`) {
		t.Errorf("expected required late field with validate tag, got:\n%s", lateBlock)
	}
	if !strings.Contains(lateBlock, `prompt:"patient_dossier"`) {
		t.Error("expected prompt tag on required late field")
	}
}

// TestGenerate_GoldenCompare compares generated output to golden files (regression test).
// Run with -golden=<dir> to overwrite golden files in that directory.
func TestGenerate_GoldenCompare(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	goldenDir := filepath.Join(filepath.Dir(file), "..", "testdata")
	if updateDir := goldenFlag(); updateDir != "" {
		goldenDir = updateDir
	}

	spec := &PromptSpec{
		ID: "support_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_query": map[string]any{"type": "string"},
					"bot_name": map[string]any{
						"type":    "string",
						"default": "SupportBot",
					},
				},
				"required": []any{"user_query"},
			}),
		},
	}

	shared, err := GenerateSharedTypes("prompts", []*PromptSpec{sharedTestSpec("support_agent")})
	assertGenerationSucceeded(t, err, "GenerateSharedTypes: %v")
	if goldenFlag() != "" {
		writeGolden(t, shared, filepath.Join(goldenDir, "shared_gen.go.golden"))
	}
	compareGolden(t, goldenDir, "shared_gen.go.golden", func() (string, error) { return renderGoldenSource(shared) })

	manifestFile, err := GenerateManifestTypes(spec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes: %v")
	if goldenFlag() != "" {
		writeGolden(t, manifestFile, filepath.Join(goldenDir, "support_agent_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"support_agent_gen.go.golden",
		func() (string, error) { return renderGoldenSource(manifestFile) },
	)

	consts, err := GenerateConstsPackage("prompts", []string{"support_agent"})
	assertGenerationSucceeded(t, err, "GenerateConstsPackage: %v")
	if goldenFlag() != "" {
		writeGolden(t, consts, filepath.Join(goldenDir, "consts_gen.go.golden"))
	}
	compareGolden(t, goldenDir, "consts_gen.go.golden", func() (string, error) { return renderGoldenSource(consts) })

	lateSpec := &PromptSpec{
		ID: "late_binding_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_query": map[string]any{"type": "string"},
					"patient_dossier": map[string]any{
						"type": "string",
						"late": true,
					},
				},
				"required": []any{"user_query"},
			}),
		},
	}
	lateFile, err := GenerateManifestTypes(lateSpec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes late_binding_agent: %v")
	if goldenFlag() != "" {
		writeGolden(t, lateFile, filepath.Join(goldenDir, "late_binding_agent_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"late_binding_agent_gen.go.golden",
		func() (string, error) { return renderGoldenSource(lateFile) },
	)

	noVarsSpec := &PromptSpec{ID: "no_vars"}
	noVarsFile, err := GenerateManifestTypes(noVarsSpec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes no_vars: %v")
	if goldenFlag() != "" {
		writeGolden(t, noVarsFile, filepath.Join(goldenDir, "no_vars_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"no_vars_gen.go.golden",
		func() (string, error) { return renderGoldenSource(noVarsFile) },
	)

	lateRequiredSpec := loadTestPromptSpec(t, filepath.Join(testdataPromptsDir(t), "late_required_agent.yaml"), nil)
	lateRequiredFile, err := GenerateManifestTypes(lateRequiredSpec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes late_required_agent: %v")
	if goldenFlag() != "" {
		writeGolden(t, lateRequiredFile, filepath.Join(goldenDir, "late_required_agent_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"late_required_agent_gen.go.golden",
		func() (string, error) { return renderGoldenSource(lateRequiredFile) },
	)

	composedMainSpec := loadTestPromptSpec(
		t,
		filepath.Join(testdataPromptsDir(t), "composed_main.yaml"),
		composeTestLoader(t),
	)
	composedMainFile, err := GenerateManifestTypes(composedMainSpec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes composed_main: %v")
	if goldenFlag() != "" {
		writeGolden(t, composedMainFile, filepath.Join(goldenDir, "composed_main_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"composed_main_gen.go.golden",
		func() (string, error) { return renderGoldenSource(composedMainFile) },
	)

	composedChildSpec := loadTestPromptSpec(
		t,
		filepath.Join(testdataPromptsDir(t), "composed_child.yaml"),
		composeTestLoader(t),
	)
	composedChildFile, err := GenerateManifestTypes(composedChildSpec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes composed_child: %v")
	if goldenFlag() != "" {
		writeGolden(t, composedChildFile, filepath.Join(goldenDir, "composed_child_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"composed_child_gen.go.golden",
		func() (string, error) { return renderGoldenSource(composedChildFile) },
	)

	composedConditionalSpec := loadTestPromptSpec(
		t,
		filepath.Join(testdataPromptsDir(t), "composed_conditional_main.yaml"),
		composeTestLoader(t),
	)
	composedConditionalFile, err := GenerateManifestTypes(composedConditionalSpec, "prompts")
	assertGenerationSucceeded(t, err, "GenerateManifestTypes composed_conditional_main: %v")
	if goldenFlag() != "" {
		writeGolden(t, composedConditionalFile, filepath.Join(goldenDir, "composed_conditional_main_gen.go.golden"))
	}
	compareGolden(
		t,
		goldenDir,
		"composed_conditional_main_gen.go.golden",
		func() (string, error) { return renderGoldenSource(composedConditionalFile) },
	)
}

func sharedTestSpec(id string) *PromptSpec {
	return &PromptSpec{
		ID:         id,
		Metadata:   prompty.PromptMetadata{ID: id},
		Descriptor: prompty.ManifestDescriptor{ID: id, Digest: "sha256:test-" + id},
	}
}

func compareGolden(t *testing.T, dir, name string, gen func() (string, error)) {
	t.Helper()
	path := filepath.Join(dir, name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %s not found in %s: %v", name, dir, err)
	}
	got, err := gen()
	if err != nil {
		t.Fatalf("generate %s: %v", name, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

// parseGoldenFlag reads -golden or -golden=path from [os.Args] without [flag.Parse] (avoids conflict with go test -test.* flags).
func parseGoldenFlag() string {
	for i, arg := range os.Args {
		if arg == "-golden" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if after, ok := strings.CutPrefix(arg, "-golden="); ok {
			return after
		}
	}
	return ""
}

func goldenFlag() string { return parseGoldenFlag() }

func writeGolden(t *testing.T, f *jen.File, path string) {
	t.Helper()
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Logf("wrote %s", path)
}

func assertGeneratedContains(t *testing.T, output, expected, message string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Error(message)
	}
}

func renderGoldenSource(file *jen.File) (string, error) {
	var buffer strings.Builder
	if err := file.Render(&buffer); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func assertGenerationSucceeded(t *testing.T, err error, message string) {
	t.Helper()
	if err != nil {
		t.Fatalf(message, err)
	}
}
