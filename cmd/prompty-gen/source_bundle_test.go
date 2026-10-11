package main

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
)

func bundleFixture(t *testing.T) (string, *Package, []string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("testdata/source_bundle")); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(dir, "prompty.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := &cfg.Packages[0]
	files, err := pkg.ResolveSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, pkg, files
}

func capturedDescriptor(t *testing.T, index *capturedManifestIndex, id string) prompty.ManifestDescriptor {
	t.Helper()
	desc, err := manifest.CheckpointRecommend(t.Context(), id, index)
	if err != nil {
		t.Fatal(err)
	}
	return desc
}

func TestSourceBundleRuntimeParity(t *testing.T) {
	for _, mode := range []string{"configured", "absent", "zero matches"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			dir, pkg, files := bundleFixture(t)
			var partials map[string][]byte
			var resolution map[string]string
			switch mode {
			case "configured":
				data, err := os.ReadFile(filepath.Join(dir, "templates/base.tmpl"))
				if err != nil {
					t.Fatal(err)
				}
				partials = map[string][]byte{"templates/base.tmpl": data}
				resolution = manifest.SourceResolutionConfig("", "templates/*.tmpl", "root.v1", "", "")
			case "absent":
				pkg.SourceBundle = nil
			case "zero matches":
				pkg.SourceBundle.Partials = "templates/missing*.tmpl"
				resolution = maps.Clone(pkg.SourceBundle.Resolution)
			}
			reader, err := indexManifests(files, dir, pkg.Queries)
			if err != nil {
				t.Fatal(err)
			}

			// Act.
			captured, err := capturePackageSources(files, dir, pkg)

			// Assert.
			if err != nil {
				t.Fatal(err)
			}
			assertRuntimeParity(t, captured, reader, partials, resolution)
		})
	}
}

func TestSourceBundleMutationAndIsolation(t *testing.T) {
	for _, change := range []string{"partial", "resolution", "import"} {
		t.Run(change, func(t *testing.T) {
			// Arrange.
			dir, pkg, files := bundleFixture(t)
			captured, err := capturePackageSources(files, dir, pkg)
			if err != nil {
				t.Fatal(err)
			}
			original := map[string]prompty.ManifestDescriptor{}
			for _, id := range []string{"main", "child"} {
				original[id] = capturedDescriptor(t, captured, id)
			}
			mutateBundle(t, dir, pkg, change)

			// Act.
			next, err := capturePackageSources(files, dir, pkg)

			// Assert.
			if err != nil {
				t.Fatal(err)
			}
			assertBundleIsolation(t, captured, next, original, change)
		})
	}
}

func TestSourceBundleMapOrderAndCancellation(t *testing.T) {
	// Arrange.
	dir, pkg, files := bundleFixture(t)
	first, err := capturePackageSources(files, dir, pkg)
	if err != nil {
		t.Fatal(err)
	}
	pkg.SourceBundle.Resolution = map[string]string{"partial_resolution": "root.v1", "partials": "templates/*.tmpl"}
	second, err := capturePackageSources(files, dir, pkg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Act.
	left := capturedDescriptor(t, first, "main")
	right := capturedDescriptor(t, second, "main")
	_, cancelled := first.CaptureManifest(ctx, "main")

	// Assert.
	if left != right {
		t.Fatalf("insertion order changed digest: %v %v", left, right)
	}
	if !errors.Is(cancelled, context.Canceled) {
		t.Fatalf("cancelled capture: %v", cancelled)
	}
}

func TestSourceBundleFailuresPreserveOutput(t *testing.T) {
	for _, failure := range []string{"glob", "read", "escape", "parent", "absolute", "missing root"} {
		t.Run(failure, func(t *testing.T) {
			// Arrange.
			dir, pkg, _ := bundleFixture(t)
			pkg.Path = "generated"
			output := filepath.Join(dir, pkg.Path)
			writeOutputFixture(t, filepath.Join(output, "sentinel.go"), "original")
			switch failure {
			case "glob":
				pkg.SourceBundle.Partials = "["
			case "read":
				if err := os.Mkdir(filepath.Join(dir, "templates/directory.tmpl"), 0750); err != nil {
					t.Fatal(err)
				}
			case "escape":
				outside := t.TempDir()
				writeOutputFixture(t, filepath.Join(outside, "outside.tmpl"), "outside")
				if err := os.Symlink(
					filepath.Join(outside, "outside.tmpl"),
					filepath.Join(dir, "templates/escape.tmpl"),
				); err != nil {
					t.Fatal(err)
				}
			case "parent":
				pkg.SourceBundle.Partials = "../*.tmpl"
			case "absolute":
				pkg.SourceBundle.Partials = "/tmp/*.tmpl"
			case "missing root":
				pkg.SourceBundle.Root = "missing"
			}
			before := snapshotOutput(t, output)

			// Act.
			err := generatePackages(dir, &Config{Version: "1", Packages: []Package{*pkg}})

			// Assert.
			if err == nil {
				t.Fatal("expected source bundle failure")
			}
			if after := snapshotOutput(t, output); !reflect.DeepEqual(before, after) {
				t.Fatalf("output changed: %v", err)
			}
		})
	}
}

func TestSourceBundleStrictConfig(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"valid", "source_bundle:\n      root: .\n      resolution: {custom: value}", true},
		{"unknown nested field", "source_bundle:\n      root: .\n      typo: value", false},
		{"root required", "source_bundle: {}", false},
		{"malformed glob", "source_bundle: {root: ., partials: '['}", false},
		{"parent traversal", "source_bundle: {root: ., partials: '../*.tmpl'}", false},
		{"absolute partials", "source_bundle: {root: ., partials: '/tmp/*.tmpl'}", false},
		{"backslash partials", "source_bundle: {root: ., partials: 'templates\\*.tmpl'}", false},
		{"consts rejected", "mode: consts\n    source_bundle: {root: .}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			file := filepath.Join(t.TempDir(), "prompty.yaml")
			writeOutputFixture(
				t,
				file,
				"packages:\n  - name: prompts\n    path: out\n    queries: [prompts]\n    "+tc.extra+"\n",
			)

			// Act.
			_, err := LoadConfig(file)

			// Assert.
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.name == "unknown nested field" && !strings.Contains(err.Error(), "field typo") {
				t.Fatal(err)
			}
		})
	}
}

func assertRuntimeParity(
	t *testing.T,
	captured, reader *capturedManifestIndex,
	partials map[string][]byte,
	resolution map[string]string,
) {
	t.Helper()
	for _, id := range []string{"main", "child"} {
		snapshot, err := manifest.CaptureSources(t.Context(), id, reader, yaml.New(), partials, resolution)
		if err != nil {
			t.Fatal(err)
		}
		if got := capturedDescriptor(t, captured, id); got != snapshot.Descriptor() {
			t.Fatalf("generator=%v runtime=%v", got, snapshot.Descriptor())
		}
	}
}

func mutateBundle(t *testing.T, dir string, pkg *Package, change string) {
	t.Helper()
	switch change {
	case "partial":
		writeOutputFixture(t, filepath.Join(dir, "templates/base.tmpl"), "changed")
	case "resolution":
		pkg.SourceBundle.Resolution["partial_resolution"] = "changed"
	case "import":
		writeOutputFixture(
			t,
			filepath.Join(dir, "prompts/child.yaml"),
			"id: child\nmessages:\n  - role: system\n    content: changed\n",
		)
	}
}

func assertBundleIsolation(
	t *testing.T,
	captured, next *capturedManifestIndex,
	original map[string]prompty.ManifestDescriptor,
	change string,
) {
	t.Helper()
	for _, id := range []string{"main", "child"} {
		if got := capturedDescriptor(t, captured, id); got != original[id] {
			t.Fatalf("capture changed for %s", id)
		}
		if got := capturedDescriptor(t, next, id); got == original[id] {
			t.Fatalf("new run did not observe %s for %s", change, id)
		}
		if err := manifest.CheckpointVerify(
			t.Context(),
			original[id],
			next,
		); !errors.Is(
			err,
			prompty.ErrManifestDigestMismatch,
		) {
			t.Fatalf("stale descriptor: %v", err)
		}
	}
}
