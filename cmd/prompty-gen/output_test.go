package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeOutputFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0640); err != nil {
		t.Fatal(err)
	}
}

func outputFixture(t *testing.T, names, ids []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for i, name := range names {
		writeOutputFixture(
			t,
			filepath.Join(dir, "prompts", name+".yaml"),
			fmt.Sprintf("id: %s\nversion: '1'\nmessages:\n  - role: user\n    content: hello\ninputs: {}\n", ids[i]),
		)
	}
	cfg := filepath.Join(dir, "prompty.yaml")
	writeOutputFixture(t, cfg, "version: '1'\npackages:\n  - name: prompts\n    path: out\n    queries: [prompts/]\n")
	return dir, cfg
}

func snapshotOutput(t *testing.T, dir string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return result
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result[entry.Name()] = string(data)
	}
	return result
}

func TestOutputCollisionsLeaveConsumerUntouched(t *testing.T) {
	for _, test := range []struct {
		name       string
		files, ids []string
		evidence   string
	}{
		{"normalized filenames", []string{"a/my-prompt", "b/my_prompt"}, []string{"alpha", "beta"}, "output collision"},
		{"shared filename", []string{"prompts_shared"}, []string{"alpha"}, "output collision"},
		{"catalog Index field and method", []string{"alpha"}, []string{"index"}, "collision"},
		{"catalog Descriptor field and method", []string{"alpha"}, []string{"descriptor"}, "collision"},
		{"normalized Go IDs", []string{"alpha", "beta"}, []string{"foo-bar", "foo_bar"}, "collision"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: a consumer-owned file and invalid output inventory sources.
			dir, cfg := outputFixture(t, test.files, test.ids)
			consumer := filepath.Join(dir, "out", "consumer.go")
			writeOutputFixture(t, consumer, "package prompts\nconst Kept = 1\n")
			// Act.
			err := runGenerate(cfg)
			// Assert: not even shared output is published after failure.
			if err == nil || !strings.Contains(err.Error(), test.evidence) {
				t.Fatalf("want %q error, got %v", test.evidence, err)
			}
			files := snapshotOutput(t, filepath.Join(dir, "out"))
			if len(files) != 1 || files["consumer.go"] != "package prompts\nconst Kept = 1\n" {
				t.Fatalf("partial/changed output: %v", files)
			}
		})
	}
}

func TestOutputOwnershipCleanupAndDeterminism(t *testing.T) {
	// Arrange: unrelated consumer file and two valid manifests.
	dir, cfg := outputFixture(t, []string{"alpha", "beta"}, []string{"alpha", "beta"})
	out := filepath.Join(dir, "out")
	writeOutputFixture(t, filepath.Join(out, "consumer.go"), "package prompts\nconst Kept = 1\n")
	// Act: create and regenerate.
	if err := runGenerate(cfg); err != nil {
		t.Fatal(err)
	}
	before := snapshotOutput(t, out)
	if err := runGenerate(cfg); err != nil {
		t.Fatal(err)
	}
	// Assert deterministic bytes, including the ownership inventory.
	after := snapshotOutput(t, out)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatal("regeneration changed files")
	}
	// Act: delete a source, then switch to consts.
	if err := os.Remove(filepath.Join(dir, "prompts", "beta.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := runGenerate(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "beta_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("stale file remains: %v", err)
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeOutputFixture(t, cfg, string(data)+"    mode: consts\n")
	if err := runGenerate(cfg); err != nil {
		t.Fatal(err)
	}
	files := snapshotOutput(t, out)
	if len(files) != 3 || files["consumer.go"] != before["consumer.go"] || files["prompts_consts_gen.go"] == "" {
		t.Fatalf("mode cleanup invalid: %v", files)
	}
}

func TestOutputRejectsConsumerFileAndEditedOwnedFile(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(strconv.FormatBool(owned), func(t *testing.T) {
			// Arrange a colliding user file or edited previously owned output.
			dir, cfg := outputFixture(t, []string{"alpha"}, []string{"alpha"})
			if owned {
				if err := runGenerate(cfg); err != nil {
					t.Fatal(err)
				}
			}
			filename := filepath.Join(dir, "out", "alpha_gen.go")
			writeOutputFixture(t, filename, "package prompts\n// consumer edit\n")
			before := snapshotOutput(t, filepath.Join(dir, "out"))
			// Act.
			err := runGenerate(cfg)
			// Assert.
			if err == nil {
				t.Fatal("overwrote consumer file")
			}
			if fmt.Sprint(before) != fmt.Sprint(snapshotOutput(t, filepath.Join(dir, "out"))) {
				t.Fatal("output changed on ownership failure")
			}
		})
	}
}

func TestValidGeneratedConsumerCompiles(t *testing.T) {
	// Arrange valid noncolliding sources with an explicit standalone consumer module.
	dir, cfg := outputFixture(t, []string{"alpha", "beta"}, []string{"alpha", "beta"})
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	writeOutputFixture(
		t,
		filepath.Join(out, "go.mod"),
		fmt.Sprintf(
			"module fixture\n\ngo 1.26.3\nrequire github.com/skosovsky/prompty v0.0.0\nreplace github.com/skosovsky/prompty => %q\n",
			root,
		),
	)
	// The consumer uses the repository's pinned dependency checksums, without
	// requiring writes to a user's sumdb cache during standalone compilation.
	sums, readErr := os.ReadFile(filepath.Join(root, "go.sum"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	writeOutputFixture(t, filepath.Join(out, "go.sum"), string(sums))
	// Act.
	if generateErr := runGenerate(cfg); generateErr != nil {
		t.Fatal(generateErr)
	}
	cmd := exec.Command("go", "test", "-mod=mod", "./...")
	cmd.Dir = out
	cmd.Env = append(
		os.Environ(),
		"GOWORK=off",
		"GOTOOLCHAIN=local",
		"GOFLAGS=-modcacherw",
		"GOMODCACHE="+filepath.Join(t.TempDir(), "modcache"),
	)
	output, err := cmd.CombinedOutput()
	// Assert actual consumer compilation, not just syntax.
	if err != nil {
		t.Fatalf("consumer: %v\n%s", err, output)
	}
}

func TestOutputSymbolInventoryIncludesMethodsAndHelpers(t *testing.T) {
	// Arrange collisions that are distinct from filename normalization.
	for _, sources := range []map[string][]byte{
		{"a.go": []byte("package p; type PromptID string"), "b.go": []byte("package p; const PromptID = 1")},
		{"a.go": []byte("package p; type T struct{}; func (T) ID(){}"), "b.go": []byte("package p; func (*T) ID(){}")},
	} {
		// Act.
		err := validateOutputSymbols(sources)
		// Assert.
		if err == nil || !strings.Contains(err.Error(), "collision") {
			t.Fatalf("expected collision: %v", err)
		}
	}
}

func TestConfigRequiresOneSupportedCompleteDocument(t *testing.T) {
	valid := "version: '1'\npackages:\n  - name: prompts\n    path: out\n    queries: [prompts/]\n"
	for _, data := range []string{
		strings.Replace(valid, "version: '1'", "version: '2'", 1),
		valid + "---\nversion: '1'\n",
		strings.Replace(valid, "name: prompts", "name: for", 1),
		strings.Replace(valid, "path: out", "path: ''", 1),
		strings.Replace(valid, "queries: [prompts/]", "queries: []", 1),
		"version: '1'\npackages: []\n",
	} {
		// Arrange.
		cfg := filepath.Join(t.TempDir(), "prompty.yaml")
		writeOutputFixture(t, cfg, data)
		// Act.
		_, err := LoadConfig(cfg)
		// Assert.
		if err == nil {
			t.Fatalf("accepted invalid config: %s", data)
		}
	}
	// Valid control.
	cfg := filepath.Join(t.TempDir(), "prompty.yaml")
	writeOutputFixture(t, cfg, valid)
	if _, err := LoadConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestOutputRollbackPreservesEarlierPackage(t *testing.T) {
	// Arrange a valid first output and an uncreatable second directory.
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	writeOutputFixture(t, filepath.Join(first, "kept_gen.go"), "old")
	blocker := filepath.Join(dir, "blocker")
	writeOutputFixture(t, blocker, "file")
	outputs := []outputPackage{
		{
			dir:      first,
			files:    map[string][]byte{"kept_gen.go": []byte("new")},
			previous: map[string][]byte{"kept_gen.go": []byte("old")},
		},
		{
			dir:      filepath.Join(blocker, "out"),
			files:    map[string][]byte{"new_gen.go": []byte("new")},
			previous: map[string][]byte{},
		},
	}
	// Act.
	err := publishOutputs(outputs)
	// Assert rollback after an actual filesystem publication error.
	data, readErr := os.ReadFile(filepath.Join(first, "kept_gen.go"))
	if err == nil || readErr != nil || !bytes.Equal(data, []byte("old")) {
		t.Fatalf("rollback: %v, read: %v, bytes: %s", err, readErr, data)
	}
}

func TestOutputDirectoryAliasesFailBeforeMutation(t *testing.T) {
	for _, leaf := range []string{"", "/future"} {
		t.Run(leaf, func(t *testing.T) {
			// Arrange two package destinations resolving to the same directory,
			// including a not-yet-created leaf below an existing symlink parent.
			dir, cfg := outputFixture(t, []string{"alpha"}, []string{"alpha"})
			actual := filepath.Join(dir, "out")
			if mkdirErr := os.Mkdir(actual, 0750); mkdirErr != nil {
				t.Fatal(mkdirErr)
			}
			if linkErr := os.Symlink(actual, filepath.Join(dir, "alias")); linkErr != nil {
				t.Fatal(linkErr)
			}
			writeOutputFixture(
				t,
				cfg,
				"version: '1'\npackages:\n  - name: alpha\n    path: out"+leaf+"\n    queries: [prompts/]\n  - name: beta\n    path: alias"+leaf+"\n    queries: [prompts/]\n",
			)
			// Act.
			err := runGenerate(cfg)
			// Assert no first-package publication occurred.
			if err == nil || !strings.Contains(err.Error(), "directory collision") {
				t.Fatalf("want collision: %v", err)
			}
			entries, readErr := os.ReadDir(actual)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("output changed: %v, %v", entries, readErr)
			}
		})
	}
}
