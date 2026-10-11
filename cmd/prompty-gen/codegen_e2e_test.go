//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
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

	// Compile and vet the temporary generated output, including its behavioral tests.
	testStandaloneGeneratedPackage(t, filepath.Join(fixture, "gen_out"))

	t.Run("consumer source bundle", func(t *testing.T) {
		// Arrange.
		consumer := t.TempDir()
		if err := os.CopyFS(consumer, os.DirFS("testdata/source_bundle")); err != nil {
			t.Fatal(err)
		}

		// Act.
		err := runGenerate(filepath.Join(consumer, "prompty.yaml"))

		// Assert.
		if err != nil {
			t.Fatal(err)
		}
		testStandaloneGeneratedPackage(t, consumer)
	})
}

func testStandaloneGeneratedPackage(t *testing.T, dir string) {
	t.Helper()
	// Arrange.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	mod := "module consumerfixture\n\ngo 1.27.2\n\nrequire github.com/skosovsky/prompty v0.16.3\n\nreplace github.com/skosovsky/prompty => " + strconv.Quote(
		root,
	) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"mod", "tidy"}, {"test", "-race", "-count=1", "./..."}, {"vet", "./..."}} {
		// Arrange.
		cmd := exec.CommandContext(t.Context(), "go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off")

		// Act.
		output, err := cmd.CombinedOutput()

		// Assert.
		if err != nil {
			t.Fatalf("standalone generated package go %v: %v\n%s", args, err, output)
		}
	}
}
