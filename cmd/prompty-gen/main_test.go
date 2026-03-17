package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRunGenerate_ModeConsts(t *testing.T) {
	tmp := t.TempDir()
	promptsDir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	manifestPath := filepath.Join(promptsDir, "legacy.yaml")
	if err := os.WriteFile(manifestPath, []byte("id: legacy_only\nversion: \"1\"\n"), 0644); err != nil {
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

func TestRunGenerate_DuplicateID(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two manifests with same id
	for _, name := range []string{"a.yaml", "b.yaml"} {
		body := "id: same_id\nversion: \"1\"\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
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

func TestRunGenerate_ReservedID(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "id: prompts\nversion: \"1\"\n"
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
