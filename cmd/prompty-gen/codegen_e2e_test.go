//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/skosovsky/prompty/cmd/prompty-gen/gen"
)

func TestE2ELoadSpec_RequiredTools_FromYAML_E2ECodegen(t *testing.T) {
	manifestPath := "testdata/doctor_agent_required_tools.yaml"
	spec, err := loadSpec(manifestPath, ".", []string{"testdata"}, nil)
	if err != nil {
		t.Fatalf("loadSpec: %v", err)
	}
	want := []string{"doctor_search_knowledge_base", "get_current_time"}
	if !reflect.DeepEqual(want, spec.RequiredTools) {
		t.Fatalf("RequiredTools = %#v, want %#v", spec.RequiredTools, want)
	}

	f, err := gen.GenerateManifestTypes(spec, "prompts")
	if err != nil {
		t.Fatalf("GenerateManifestTypes: %v", err)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "func (p *DoctorAgentPrompt) RequiredTools()") {
		t.Error("expected RequiredTools method on DoctorAgentPrompt")
	}
	for _, tool := range want {
		if !strings.Contains(out, `"`+tool+`"`) {
			t.Errorf("expected %q in RequiredTools return literal", tool)
		}
	}
}

func TestE2EGeneratedPackage(t *testing.T) {
	// Arrange.
	fixture := t.TempDir()
	if err := os.CopyFS(fixture, os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	want := snapshotOutput(t, "testdata/gen_out")

	// Act.
	if err := runGenerate(filepath.Join(fixture, "prompty.yaml")); err != nil {
		t.Fatalf("regenerate fixtures: %v", err)
	}
	got := snapshotOutput(t, filepath.Join(fixture, "gen_out"))

	// Assert.
	if !reflect.DeepEqual(want, got) {
		t.Fatal("generated fixtures differ; regenerate with: go run . -config testdata/prompty.yaml generate")
	}

	// Arrange.
	cmd := exec.CommandContext(t.Context(), "go", "test", "-race", "-count=1", "./testdata/gen_out")

	// Act.
	output, err := cmd.CombinedOutput()

	// Assert.
	if err != nil {
		t.Fatalf("generated package tests: %v\n%s", err, output)
	}
}
