package gen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
)

type testManifestMemoryLoader struct {
	byID map[string]*manifest.RawManifest
}

func (l *testManifestMemoryLoader) LoadByID(_ context.Context, id string) (*manifest.RawManifest, error) {
	if l == nil || l.byID == nil {
		return nil, errors.New("compose: unknown manifest id")
	}
	raw, ok := l.byID[id]
	if !ok || raw == nil {
		return nil, errors.New("compose: unknown manifest id")
	}
	return raw, nil
}

func testdataPromptsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "testdata", "prompts")
}

func readTestRawManifest(t *testing.T, path string) *manifest.RawManifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw manifest.RawManifest
	if err := yaml.New().Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return &raw
}

func loadTestPromptSpec(t *testing.T, manifestPath string, loader manifest.ManifestLoader) *PromptSpec {
	t.Helper()
	raw := readTestRawManifest(t, manifestPath)
	if raw.ID == "" {
		t.Fatalf("manifest %s has no id", manifestPath)
	}
	if raw.InputSchema == nil {
		t.Fatalf("manifest %s missing inputs block", manifestPath)
	}
	if len(raw.Messages) == 0 && len(raw.Layers) == 0 {
		t.Fatalf("manifest %s missing messages or layers", manifestPath)
	}
	composeConditions, err := ComposeConditionsFromRawManifest(context.Background(), raw, loader)
	if err != nil {
		t.Fatalf("ComposeConditionsFromRawManifest: %v", err)
	}
	effectiveSchema, err := manifest.ResolveEffectiveInputSchema(context.Background(), raw, loader)
	if err != nil {
		t.Fatalf("ResolveEffectiveInputSchema: %v", err)
	}
	raw.InputSchema = effectiveSchema
	if expandErr := manifest.ExpandRawManifest(raw, manifest.ComposeContext{
		Ctx:                         context.Background(),
		Loader:                      loader,
		Values:                      prompty.ComposeValues{},
		AllowMissingConditionValues: true,
	}); expandErr != nil {
		t.Fatalf("ExpandRawManifest: %v", expandErr)
	}
	if len(raw.Messages) == 0 {
		t.Fatalf("manifest %s missing messages after composition", manifestPath)
	}
	tpl, err := manifest.BuildFromRaw(raw, nil)
	if err != nil {
		t.Fatalf("BuildFromRaw: %v", err)
	}
	requiredTools := tpl.RequiredTools
	if requiredTools == nil {
		requiredTools = []string{}
	}
	return &PromptSpec{
		ID:                tpl.Metadata.ID,
		Metadata:          tpl.Metadata,
		RequiredTools:     requiredTools,
		InputSchema:       tpl.InputSchema,
		ResponseFormat:    tpl.ResponseFormat,
		ComposeConditions: composeConditions,
	}
}

func composeTestLoader(t *testing.T) manifest.ManifestLoader {
	t.Helper()
	dir := testdataPromptsDir(t)
	loader := &testManifestMemoryLoader{byID: map[string]*manifest.RawManifest{}}
	for _, name := range []string{"composed_main.yaml", "composed_child.yaml", "composed_conditional_main.yaml"} {
		raw := readTestRawManifest(t, filepath.Join(dir, name))
		loader.byID[raw.ID] = raw
	}
	return loader
}

func TestComposeConditionsFromRawManifest_TransitiveImports(t *testing.T) {
	t.Parallel()
	main := &manifest.RawManifest{
		ID: "main",
		Imports: []manifest.RawImport{
			{ID: "child"},
		},
	}
	child := &manifest.RawManifest{
		ID: "child",
		Imports: []manifest.RawImport{
			{
				ID: "grandchild",
				Condition: &manifest.RawCondition{Match: map[string]any{
					"capabilities.child_enabled": true,
				}},
			},
		},
	}
	grandchild := &manifest.RawManifest{ID: "grandchild"}
	loader := &testManifestMemoryLoader{byID: map[string]*manifest.RawManifest{
		"child":      child,
		"grandchild": grandchild,
	}}

	conditions, err := ComposeConditionsFromRawManifest(context.Background(), main, loader)
	if err != nil {
		t.Fatalf("ComposeConditionsFromRawManifest: %v", err)
	}
	if len(conditions) != 1 {
		t.Fatalf("expected one transitive condition, got %#v", conditions)
	}
	if conditions[0].Key != "capabilities.child_enabled" || conditions[0].Kind != "bool" {
		t.Fatalf("unexpected transitive condition: %#v", conditions[0])
	}
}
