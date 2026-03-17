package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_ValidYAML(t *testing.T) {
	configPath := "testdata/prompty.yaml"
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Version != "1" {
		t.Errorf("expected Version 1, got %q", cfg.Version)
	}
	if len(cfg.Packages) == 0 {
		t.Fatal("expected at least one package")
	}
	pkg := cfg.Packages[0]
	if pkg.Name != "prompts" {
		t.Errorf("expected name prompts, got %q", pkg.Name)
	}
	if pkg.Path != "gen_out" {
		t.Errorf("expected path gen_out, got %q", pkg.Path)
	}
	if len(pkg.Queries) == 0 {
		t.Error("expected non-empty queries")
	}
	if pkg.PackageName != "prompts" {
		t.Errorf("expected package prompts, got %q", pkg.PackageName)
	}
	if pkg.Mode != "types" {
		t.Errorf("expected mode types (default), got %q", pkg.Mode)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfig_InvalidMode(t *testing.T) {
	// Create temp config with invalid mode
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["./*.yaml"]
    mode: invalid
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Errorf("expected 'invalid mode' in error, got: %v", err)
	}
}

func TestLoadConfig_UnknownFieldsFails(t *testing.T) {
	// KnownFields(true) must hard-fail on legacy max_retries
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "prompty.yaml")
	cfg := `version: "1"
packages:
  - name: pkg
    path: out
    queries: ["./*.yaml"]
    max_retries: 3
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for unknown field max_retries")
	}
	if !strings.Contains(err.Error(), "max_retries") && !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Errorf("expected unknown/max_retries in error, got: %v", err)
	}
}

func TestLoadConfig_ModeDefaultTypes(t *testing.T) {
	tmp, err := os.CreateTemp("", "prompty-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(`
version: "1"
packages:
  - name: pkg1
    path: out
    queries: ["./*.yaml"]
`)
	tmp.Close()

	cfg, err := LoadConfig(tmp.Name())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Packages[0].Mode != "types" {
		t.Errorf("expected default mode types, got %q", cfg.Packages[0].Mode)
	}
}

func TestPackage_ResolveSources(t *testing.T) {
	absTestdata, _ := filepath.Abs("testdata")
	pkg := Package{
		Name:    "prompts",
		Queries: []string{"prompts/*.yaml"},
	}
	files, err := pkg.ResolveSources(absTestdata)
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	found := false
	for _, f := range files {
		if filepath.Base(f) == "support_agent.yaml" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find support_agent.yaml")
	}
}

func TestPackage_IsConsts_IsTypes(t *testing.T) {
	t.Run("consts", func(t *testing.T) {
		p := Package{Mode: "consts"}
		if !p.IsConsts() {
			t.Error("mode consts should be IsConsts")
		}
		if p.IsTypes() {
			t.Error("mode consts should not be IsTypes")
		}
	})
	t.Run("types", func(t *testing.T) {
		p := Package{Mode: "types"}
		if p.IsConsts() {
			t.Error("mode types should not be IsConsts")
		}
		if !p.IsTypes() {
			t.Error("mode types should be IsTypes")
		}
	})
	t.Run("empty_defaults_types", func(t *testing.T) {
		p := Package{Mode: ""}
		if p.IsConsts() {
			t.Error("empty mode should not be IsConsts")
		}
		if !p.IsTypes() {
			t.Error("empty mode should be IsTypes")
		}
	})
}

func TestLoadConfig_ModeLiteFullHardFail(t *testing.T) {
	for _, mode := range []string{"lite", "full"} {
		tmp := t.TempDir()
		cfgPath := filepath.Join(tmp, "prompty.yaml")
		cfg := "version: \"1\"\npackages:\n  - name: pkg\n    path: out\n    queries: [\"./*.yaml\"]\n    mode: " + mode + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
			t.Fatalf("write temp config: %v", err)
		}
		_, err := LoadConfig(cfgPath)
		if err == nil {
			t.Fatalf("expected error for legacy mode %q", mode)
		}
		if !strings.Contains(err.Error(), "invalid mode") {
			t.Errorf("expected 'invalid mode' in error, got: %v", err)
		}
	}
}
