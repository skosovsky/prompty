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
	if err != nil {
		t.Fatalf("GenerateConstsPackage: %v", err)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "type PromptID string") {
		t.Error("expected type PromptID string")
	}
	if !strings.Contains(out, "Greeter") {
		t.Error("expected Greeter const")
	}
	if !strings.Contains(out, "SupportAgent") {
		t.Error("expected SupportAgent const")
	}
	if !strings.Contains(out, "func AllPromptIDs()") {
		t.Error("expected AllPromptIDs func")
	}
	// Sorted order: greeter before support_agent
	if strings.Index(out, "Greeter") > strings.Index(out, "SupportAgent") {
		t.Error("expected deterministic sorted order (Greeter before SupportAgent); got SupportAgent before Greeter")
	}
}

// --- Shared types tests ---

func TestGenerateSharedTypes(t *testing.T) {
	specs := []*PromptSpec{{ID: "support_agent"}, {ID: "greeter"}}
	f, err := GenerateSharedTypes("prompts", specs)
	if err != nil {
		t.Fatalf("GenerateSharedTypes: %v", err)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "type PromptID string") {
		t.Error("expected type PromptID string")
	}
	if !strings.Contains(out, "var validate = validator.New()") {
		t.Error("expected package-level validate singleton")
	}
	if !strings.Contains(out, "type PromptCatalog interface") {
		t.Error("expected PromptCatalog interface")
	}
	if !strings.Contains(out, "RenderSupportAgent") {
		t.Error("expected typed RenderSupportAgent method in catalog")
	}
	if !strings.Contains(out, "Descriptor(ctx context.Context, id PromptID)") {
		t.Error("expected Descriptor method in PromptCatalog")
	}
	if !strings.Contains(out, "RenderGreeter") {
		t.Error("expected typed RenderGreeter method in catalog")
	}
	if !strings.Contains(out, "func NewPromptCatalog(") {
		t.Error("expected NewPromptCatalog")
	}
	if strings.Contains(out, "RenderByID") {
		t.Error("legacy RenderByID must not be generated")
	}
	if strings.Contains(out, "renderFromAny") {
		t.Error("legacy renderFromAny must not be generated")
	}
	if !strings.Contains(out, "DescribingRegistry") {
		t.Error("expected NewPromptCatalog to accept DescribingRegistry")
	}
	if !strings.Contains(out, "func AllPromptIDs()") {
		t.Error("expected AllPromptIDs")
	}
	if strings.Contains(out, legacyClientTypeName) {
		t.Error("DoD: must not contain the legacy client type")
	}
	if strings.Contains(out, "Execute(") {
		t.Error("DoD: must not contain Execute (legacy agent API)")
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "const SupportAgent PromptID = \"support_agent\"") {
		t.Error("expected SupportAgent const")
	}
	if !strings.Contains(out, "type SupportAgentInput struct") {
		t.Error("expected SupportAgentInput struct")
	}
	if !strings.Contains(out, "type SupportAgentPrompt struct") {
		t.Error("expected SupportAgentPrompt type")
	}
	if !strings.Contains(out, "func (p *SupportAgentPrompt) Render(") {
		t.Error("expected Render method on SupportAgentPrompt")
	}
	if !strings.Contains(out, "func (p *SupportAgentPrompt) RequiredTools()") {
		t.Error("expected RequiredTools method on SupportAgentPrompt")
	}
	if !strings.Contains(out, "func (p *SupportAgentPrompt) ID()") {
		t.Error("expected ID method on SupportAgentPrompt")
	}
	if !strings.Contains(out, "validate.Struct") {
		t.Error("expected input validation")
	}
	if !strings.Contains(out, "p.registry.Plan") {
		t.Error("expected Plan call")
	}
	if !strings.Contains(out, "string(SupportAgent)") {
		t.Error("DoD: Plan must receive string(PromptID) for Registry interface")
	}
	if !strings.Contains(out, "build render plan") {
		t.Error("expected render-plan build error wrapping")
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}

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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "type GreeterOutput struct") {
		t.Error("expected GreeterOutput when response_format present")
	}
	// Output is generated for downstream use, while Render returns deferred plan.
	if !strings.Contains(out, "(*prompty.RenderPlan, error)") {
		t.Error("Render must return (*prompty.RenderPlan, error)")
	}
}

// --- task17-3: nested type naming regression tests ---

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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "type RouterOutputEntities struct") {
		t.Error("expected type RouterOutputEntities struct for nested object in Output")
	}
	if !strings.Contains(out, "Entities *RouterOutputEntities") {
		t.Error("expected field Entities *RouterOutputEntities (parent-based naming)")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "type RouterInputPayload struct") {
		t.Error("expected type RouterInputPayload struct for nested object in Input")
	}
	if !strings.Contains(out, "Payload *RouterInputPayload") {
		t.Error("expected field Payload *RouterInputPayload (parent-based naming)")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "type RouterOutputUsersItem struct") {
		t.Error("expected type RouterOutputUsersItem struct for array-of-object in Output")
	}
	if !strings.Contains(out, "Users []RouterOutputUsersItem") {
		t.Error("expected field Users []RouterOutputUsersItem (not ...OutputItemUsers)")
	}
}

func TestGenerateManifestTypes_EmptyInputSchema(t *testing.T) {
	spec := &PromptSpec{
		ID: "no_vars",
		// No InputSchema
	}

	f, err := GenerateManifestTypes(spec, "prompts")
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}

	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "type NoVarsInput struct") {
		t.Error("expected empty Input struct")
	}
	if !strings.Contains(out, "func (p *NoVarsPrompt) Render(") {
		t.Error("expected Render on NoVarsPrompt")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
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
}

// --- Schema regression tests (task15 DoD) ---

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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "dive") {
		t.Error("expected validate dive tag for array of objects")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "map[string]string") {
		t.Error("expected map[string]string for additionalProperties with type string")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "oneof=pending done") {
		t.Error("expected oneof validate tag from enum")
	}
}

// --- Regression tests (task15) ---

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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "*bool") {
		t.Error("required bool must be *bool for presence semantics")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "type EmptyInputInput struct") {
		t.Error("expected empty Input struct for root object without properties")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "[][]") {
		t.Error("expected array-of-array nested type")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "X2faPrompt") {
		t.Error("expected id starting with digit to get X prefix")
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
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
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "type GreeterInput struct") {
		t.Error("expected generated input struct")
	}
	if !strings.Contains(out, "if input.Greeting == nil") {
		t.Error("expected default guard for optional greeting")
	}
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

	shared, err := GenerateSharedTypes("prompts", []*PromptSpec{{ID: "support_agent"}})
	if err != nil {
		t.Fatalf("GenerateSharedTypes: %v", err)
	}
	if goldenFlag() != "" {
		writeGolden(t, shared, filepath.Join(goldenDir, "shared_gen.go.golden"))
	}
	compareGolden(t, goldenDir, "shared_gen.go.golden", func() (string, error) {
		var b strings.Builder
		if renderErr := shared.Render(&b); renderErr != nil {
			return "", renderErr
		}
		return b.String(), nil
	})

	manifestFile, err := GenerateManifestTypes(spec, "prompts")
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	if goldenFlag() != "" {
		writeGolden(t, manifestFile, filepath.Join(goldenDir, "support_agent_gen.go.golden"))
	}
	compareGolden(t, goldenDir, "support_agent_gen.go.golden", func() (string, error) {
		var b strings.Builder
		if renderErr := manifestFile.Render(&b); renderErr != nil {
			return "", renderErr
		}
		return b.String(), nil
	})

	consts, err := GenerateConstsPackage("prompts", []string{"support_agent"})
	if err != nil {
		t.Fatalf("GenerateConstsPackage: %v", err)
	}
	if goldenFlag() != "" {
		writeGolden(t, consts, filepath.Join(goldenDir, "consts_gen.go.golden"))
	}
	compareGolden(t, goldenDir, "consts_gen.go.golden", func() (string, error) {
		var b strings.Builder
		if renderErr := consts.Render(&b); renderErr != nil {
			return "", renderErr
		}
		return b.String(), nil
	})
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
