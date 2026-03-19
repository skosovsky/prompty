package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIdFromRelativePath(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "proj")
	promptsDir := filepath.Join(configDir, "prompts")
	cacheDir := filepath.Join(configDir, ".prompts_cache")
	if err := os.MkdirAll(filepath.Join(promptsDir, "workers"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, "internal"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(promptsDir, "foo"), 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		fpath     string
		configDir string
		queries   []string
		want      string
	}{
		{
			name:      "explicit nested path",
			fpath:     filepath.Join(promptsDir, "workers", "image_analyze.yaml"),
			configDir: configDir,
			queries:   []string{"prompts"},
			want:      "workers/image_analyze",
		},
		{
			name:      "internal router",
			fpath:     filepath.Join(cacheDir, "internal", "router.yaml"),
			configDir: configDir,
			queries:   []string{".prompts_cache"},
			want:      "internal/router",
		},
		{
			name:      "flat file in prompts",
			fpath:     filepath.Join(promptsDir, "support_agent.yaml"),
			configDir: configDir,
			queries:   []string{"prompts"},
			want:      "support_agent",
		},
		{
			name:      "nested foo bar",
			fpath:     filepath.Join(promptsDir, "foo", "bar.yaml"),
			configDir: configDir,
			queries:   []string{"prompts"},
			want:      "foo/bar",
		},
		{
			name:      "path outside queries returns empty (no basename fallback)",
			fpath:     filepath.Join(configDir, "other", "x.yaml"),
			configDir: configDir,
			queries:   []string{"prompts"},
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idFromRelativePath(tt.fpath, tt.configDir, tt.queries)
			if got != tt.want {
				t.Errorf("idFromRelativePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunGenerate_Testdata(t *testing.T) {
	configPath := "testdata/prompty.yaml"
	if _, err := os.Stat(configPath); err != nil {
		t.Skipf("testdata not found: %v", err)
	}

	if err := runGenerate(configPath); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}

	// mode=types: verify shared and per-manifest output
	sharedPath := "testdata/gen_out/prompts_shared_gen.go"
	if _, err := os.Stat(sharedPath); err != nil {
		t.Errorf("expected shared file %s: %v", sharedPath, err)
	}
	manifestPath := "testdata/gen_out/support_agent_gen.go"
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("expected manifest file %s: %v", manifestPath, err)
	}
	content, _ := os.ReadFile(manifestPath)
	c := string(content)
	if strings.Contains(c, "LLMClient") || strings.Contains(c, "ExecuteWithStructuredOutput") {
		t.Error("DoD: generated full-mode code must not contain LLMClient or ExecuteWithStructuredOutput")
	}
	if !strings.Contains(c, "string(SupportAgent)") {
		t.Error("DoD: GetTemplate must receive string(PromptID) for Registry interface")
	}
}

func TestLoadSpec_DualSchemaFixture(t *testing.T) {
	manifestPath := "testdata/dual_schema_manifest.yaml"
	spec, err := loadSpec(manifestPath, ".", []string{"testdata"})
	if err != nil {
		t.Fatalf("loadSpec: %v", err)
	}
	if spec.ResponseFormat == nil || spec.ResponseFormat.Schema == nil {
		t.Fatal("expected response_format schema")
	}

	fixtureData, err := os.ReadFile("testdata/dual_schema_fixture.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var raw any
	if err := json.Unmarshal(fixtureData, &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	got := normalizeSchemaValue(spec.ResponseFormat.Schema)
	want := normalizeSchemaValue(raw)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("response_format schema mismatch\nwant: %#v\ngot: %#v", want, got)
	}
}

func normalizeSchemaValue(value any) any {
	switch x := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, item := range x {
			out[key] = normalizeSchemaValue(item)
		}
		if required, ok := out["required"].([]any); ok {
			names := make([]string, 0, len(required))
			for _, item := range required {
				if name, ok := item.(string); ok {
					names = append(names, name)
				}
			}
			out["required"] = names
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeSchemaValue(item)
		}
		return out
	default:
		return value
	}
}

func TestRunGenerate_ModeConsts(t *testing.T) {
	tmp := t.TempDir()
	promptsDir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	// v2.0 manifest: messages and input_schema required for consts mode
	v2Manifest := `id: legacy_only
version: "1"
messages:
  - role: system
    content: "Hi"
input_schema:
  type: object
  properties: {}
`
	if err := os.WriteFile(filepath.Join(promptsDir, "legacy.yaml"), []byte(v2Manifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	configPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["prompts/*.yaml"]
    package: pkg
    mode: consts
`
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := runGenerate(configPath); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}

	constsPath := filepath.Join(tmp, "out", "pkg_consts_gen.go")
	data, err := os.ReadFile(constsPath)
	if err != nil {
		t.Fatalf("expected consts file %s: %v", constsPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "LegacyOnly") {
		t.Error("expected LegacyOnly const in consts output")
	}
	if !strings.Contains(content, "AllPromptIDs") {
		t.Error("expected AllPromptIDs in consts output")
	}
	if strings.Contains(content, "type Prompts struct") {
		t.Error("consts mode must not generate Prompts struct")
	}
	if strings.Contains(content, "Render") {
		t.Error("consts mode must not generate Render methods")
	}
}

// TestRunGenerate_ModeConsts_LegacyFails verifies legacy manifests (no messages/input_schema) fail in consts mode.
func TestRunGenerate_ModeConsts_LegacyFails(t *testing.T) {
	tmp := t.TempDir()
	promptsDir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	legacyManifest := "id: legacy_only\nversion: \"1\"\n"
	if err := os.WriteFile(filepath.Join(promptsDir, "legacy.yaml"), []byte(legacyManifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	configPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["prompts/*.yaml"]
    package: pkg
    mode: consts
`
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	err := runGenerate(configPath)
	if err == nil {
		t.Fatal("expected error for legacy manifest in consts mode (v2.0 required)")
	}
	if !strings.Contains(err.Error(), "messages") && !strings.Contains(err.Error(), "input_schema") {
		t.Errorf("expected messages or input_schema error, got: %v", err)
	}
}

func TestRunGenerate_DuplicateID(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two manifests with same id - v2.0 required for consts
	v2Body := `id: same_id
version: "1"
messages:
  - role: system
    content: "Hi"
input_schema:
  type: object
  properties: {}
`
	for _, name := range []string{"a.yaml", "b.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v2Body), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	cfgPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["prompts/*.yaml"]
    package: pkg
    mode: consts
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	err := runGenerate(cfgPath)
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("expected duplicate id error, got: %v", err)
	}
}

func TestLoadManifestID_ExplicitAndFallback(t *testing.T) {
	tmp := t.TempDir()
	promptsDir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Explicit id (canonical slash format) - v2.0 manifest required
	explicitManifest := `id: internal/router
version: "1"
messages:
  - role: system
    content: "Hi"
input_schema:
  type: object
  properties: {}
`
	explicitPath := filepath.Join(promptsDir, "my_prompt.yaml")
	if err := os.WriteFile(explicitPath, []byte(explicitManifest), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := loadManifestID(explicitPath, tmp, []string{"prompts"})
	if err != nil {
		t.Fatalf("loadManifestID explicit: %v", err)
	}
	if id != "internal/router" {
		t.Errorf("explicit id = %q, want internal/router", id)
	}

	// Fallback from path (no id field) - slash format
	fallbackPath := filepath.Join(promptsDir, "workers", "image_analyze.yaml")
	if err := os.MkdirAll(filepath.Dir(fallbackPath), 0755); err != nil {
		t.Fatal(err)
	}
	fallbackManifest := `version: "1"
messages:
  - role: system
    content: "Hi"
input_schema:
  type: object
  properties: {}
`
	if err := os.WriteFile(fallbackPath, []byte(fallbackManifest), 0644); err != nil {
		t.Fatal(err)
	}
	id, err = loadManifestID(fallbackPath, tmp, []string{"prompts"})
	if err != nil {
		t.Fatalf("loadManifestID fallback: %v", err)
	}
	if id != "workers/image_analyze" {
		t.Errorf("fallback id = %q, want workers/image_analyze", id)
	}
}

func TestRunGenerate_ExplicitSlashID_GeneratesConst(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `id: internal/router
version: "1"
messages:
  - role: system
    content: "Hi"
input_schema:
  type: object
  properties: {}
`
	if err := os.WriteFile(filepath.Join(dir, "router.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["prompts"]
    package: pkg
    mode: consts
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runGenerate(cfgPath); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	constsPath := filepath.Join(tmp, "out", "pkg_consts_gen.go")
	data, err := os.ReadFile(constsPath)
	if err != nil {
		t.Fatalf("read consts: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `InternalRouter PromptID = "internal/router"`) {
		t.Errorf("expected const InternalRouter PromptID = \"internal/router\", got:\n%s", content)
	}
}

func TestRunGenerate_TypesMode_RequiresInputSchema(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Manifest with messages but no input_schema (legacy) - must fail in types mode
	body := `id: legacy_no_schema
version: "1"
messages:
  - role: system
    content: "Hello"
`
	if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfgPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["prompts"]
    package: pkg
    mode: types
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	err := runGenerate(cfgPath)
	if err == nil {
		t.Fatal("expected error for manifest without input_schema in types mode")
	}
	if !strings.Contains(err.Error(), "input_schema") {
		t.Errorf("expected input_schema error, got: %v", err)
	}
}

func TestRunGenerate_ReservedID(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `id: prompts
version: "1"
messages:
  - role: system
    content: "Hi"
input_schema:
  type: object
  properties: {}
`
	if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfgPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["prompts/*.yaml"]
    package: pkg
    mode: consts
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	err := runGenerate(cfgPath)
	if err == nil {
		t.Fatal("expected error for reserved id 'prompts'")
	}
	if !strings.Contains(err.Error(), "reserved") && !strings.Contains(err.Error(), "Prompts") {
		t.Errorf("expected reserved/prompts error, got: %v", err)
	}
}
