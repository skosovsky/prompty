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
	f, err := GenerateSharedTypes("prompts", []string{"support_agent", "greeter"})
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
	if !strings.Contains(out, "type Prompts struct") {
		t.Error("expected Prompts struct")
	}
	if !strings.Contains(out, "func NewPrompts(") {
		t.Error("expected NewPrompts")
	}
	if !strings.Contains(out, "func AllPromptIDs()") {
		t.Error("expected AllPromptIDs")
	}
	if strings.Contains(out, "LLMClient") {
		t.Error("DoD: must not contain LLMClient")
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_name":   map[string]any{"type": "string"},
					"user_query": map[string]any{"type": "string"},
				},
				"required": []any{"user_query"},
			},
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
	if !strings.Contains(out, "func (p *Prompts) RenderSupportAgent(") {
		t.Error("expected RenderSupportAgent method")
	}
	if !strings.Contains(out, "validate.Struct") {
		t.Error("expected input validation")
	}
	if !strings.Contains(out, "p.registry.GetTemplate") {
		t.Error("expected GetTemplate call")
	}
	if !strings.Contains(out, "string(SupportAgent)") {
		t.Error("DoD: GetTemplate must receive string(PromptID) for Registry interface")
	}
	if !strings.Contains(out, "tmpl.Format") {
		t.Error("expected Format call")
	}
	if strings.Contains(out, "LLMClient") {
		t.Error("DoD: must not contain LLMClient")
	}
	if strings.Contains(out, "ExecuteWithStructuredOutput") {
		t.Error("DoD: must not contain ExecuteWithStructuredOutput")
	}
}

func TestGenerateManifestTypes_NoResponseFormat(t *testing.T) {
	spec := &PromptSpec{
		ID: "simple",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "string"},
				},
				"required": []any{"q"},
			},
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			},
		},
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
				"required": []any{"message"},
			},
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
	// Output is generated for downstream use (e.g. prompty.Execute), but Render returns *PromptExecution
	if !strings.Contains(out, "(*prompty.PromptExecution, error)") {
		t.Error("Render must return (*PromptExecution, error)")
	}
}

// --- task17-3: nested type naming regression tests ---

func TestGenerateManifestTypes_NestedObjectInOutput(t *testing.T) {
	spec := &PromptSpec{
		ID: "router",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entities": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{"type": "string"},
						},
					},
				},
			},
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payload": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			},
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
}

func TestGenerateManifestTypes_ArrayOfObjectInOutput(t *testing.T) {
	spec := &PromptSpec{
		ID: "router",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: map[string]any{
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
			},
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
	if !strings.Contains(out, "func (p *Prompts) RenderNoVars(") {
		t.Error("expected RenderNoVars")
	}
}

// --- Schema regression tests (task15 DoD) ---

func TestGenerateManifestTypes_Dive(t *testing.T) {
	spec := &PromptSpec{
		ID: "orders",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
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
			},
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"extra": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
			},
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

func TestGenerateManifestTypes_OptionalAny(t *testing.T) {
	spec := &PromptSpec{
		ID: "flex",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payload": map[string]any{}, // no type -> any
				},
			},
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "any") {
		t.Error("expected any type for untyped optional property")
	}
	if strings.Contains(out, "*any") {
		t.Error("optional any must not use *any (redundant)")
	}
}

func TestGenerateManifestTypes_Oneof(t *testing.T) {
	spec := &PromptSpec{
		ID: "status",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"state": map[string]any{
						"type": "string",
						"enum": []any{"pending", "done"},
					},
				},
				"required": []any{"state"},
			},
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean"},
				},
				"required": []any{"enabled"},
			},
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
	if !strings.Contains(out, "*input.Enabled") {
		t.Error("required bool must be passed to vars as value (dereferenced *bool)")
	}
}

func TestGenerateManifestTypes_MinItemsMaxItems(t *testing.T) {
	spec := &PromptSpec{
		ID: "list",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ids": map[string]any{
						"type":      "array",
						"minItems":  1,
						"maxItems":  10,
						"items":     map[string]any{"type": "string"},
					},
				},
			},
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
			Schema: map[string]any{
				"type": "object",
			},
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
			Schema: map[string]any{
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
			},
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user@name": map[string]any{"type": "string"}, // @ -> _
				},
			},
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
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{"type": "string"},
				},
			},
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

func TestGenerateManifestTypes_Default(t *testing.T) {
	spec := &PromptSpec{
		ID: "greeter",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string"},
					"greeting": map[string]any{
						"type":    "string",
						"default": "Hello",
					},
				},
				"required": []any{"name"},
			},
		},
	}
	f, err := GenerateManifestTypes(spec, "prompts")
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	_ = f.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, `"Hello"`) {
		t.Error("expected default value Hello in vars else block")
	}
}

// --- Golden test ---

func TestGenerate_Golden(t *testing.T) {
	if goldenFlag() == "" {
		t.Skip("skip golden test unless -golden is set")
	}

	spec := &PromptSpec{
		ID: "support_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_query": map[string]any{"type": "string"},
					"bot_name": map[string]any{
						"type":    "string",
						"default": "SupportBot",
					},
				},
				"required": []any{"user_query"},
			},
		},
	}

	// Shared
	shared, err := GenerateSharedTypes("prompts", []string{"support_agent"})
	if err != nil {
		t.Fatalf("GenerateSharedTypes: %v", err)
	}
	sharedPath := filepath.Join(goldenFlag(), "shared_gen.go.golden")
	writeGolden(t, shared, sharedPath)

	// Manifest
	manifest, err := GenerateManifestTypes(spec, "prompts")
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	manifestPath := filepath.Join(goldenFlag(), "support_agent_gen.go.golden")
	writeGolden(t, manifest, manifestPath)

	// Consts
	consts, err := GenerateConstsPackage("prompts", []string{"support_agent"})
	if err != nil {
		t.Fatalf("GenerateConstsPackage: %v", err)
	}
	constsPath := filepath.Join(goldenFlag(), "consts_gen.go.golden")
	writeGolden(t, consts, constsPath)
}

// TestGenerate_GoldenCompare compares generated output to golden files (regression test).
// Run with -golden=<dir> to overwrite golden files.
func TestGenerate_GoldenCompare(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	goldenDir := filepath.Join(filepath.Dir(file), "..", "testdata")
	if goldenFlag() != "" {
		t.Skip("skip compare when -golden is set (use TestGenerate_Golden to overwrite)")
	}

	spec := &PromptSpec{
		ID: "support_agent",
		InputSchema: &prompty.SchemaDefinition{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_query": map[string]any{"type": "string"},
					"bot_name": map[string]any{
						"type":    "string",
						"default": "SupportBot",
					},
				},
				"required": []any{"user_query"},
			},
		},
	}

	compareGolden(t, goldenDir, "shared_gen.go.golden", func() (string, error) {
		f, err := GenerateSharedTypes("prompts", []string{"support_agent"})
		if err != nil {
			return "", err
		}
		var b strings.Builder
		_ = f.Render(&b)
		return b.String(), nil
	})
	compareGolden(t, goldenDir, "support_agent_gen.go.golden", func() (string, error) {
		f, err := GenerateManifestTypes(spec, "prompts")
		if err != nil {
			return "", err
		}
		var b strings.Builder
		_ = f.Render(&b)
		return b.String(), nil
	})
	compareGolden(t, goldenDir, "consts_gen.go.golden", func() (string, error) {
		f, err := GenerateConstsPackage("prompts", []string{"support_agent"})
		if err != nil {
			return "", err
		}
		var b strings.Builder
		_ = f.Render(&b)
		return b.String(), nil
	})
}

func compareGolden(t *testing.T, dir, name string, gen func() (string, error)) {
	t.Helper()
	path := filepath.Join(dir, name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("golden file %s not found (run with -golden=%s): %v", name, dir, err)
	}
	got, err := gen()
	if err != nil {
		t.Fatalf("generate %s: %v", name, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

// parseGoldenFlag reads -golden or -golden=path from os.Args without flag.Parse (avoids conflict with go test -test.* flags).
func parseGoldenFlag() string {
	for i, arg := range os.Args {
		if arg == "-golden" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "-golden=") {
			return strings.TrimPrefix(arg, "-golden=")
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
