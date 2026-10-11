package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"
)

// consumerSnapshot mirrors the caller-owned immutable Git/Postgres registry capture.
// The application owns revision selection; only root-relative names enter CaptureSources.
type consumerSnapshot struct {
	sources    map[string][]byte
	partials   map[string][]byte
	resolution map[string]string
}

func (s consumerSnapshot) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := s.sources[id]
	if !ok {
		return nil, prompty.ErrTemplateNotFound
	}
	return append([]byte(nil), data...), nil
}
func (s consumerSnapshot) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	return manifest.CaptureSources(ctx, id, s, yaml.New(), s.partials, s.resolution)
}

func readConsumerSnapshot(t *testing.T) consumerSnapshot {
	t.Helper()
	s := consumerSnapshot{
		sources: make(map[string][]byte), partials: make(map[string][]byte),
		resolution: manifest.SourceResolutionConfig("", "templates/*.tmpl", "root.v1", "", ""),
	}
	for _, id := range []string{"main", "child"} {
		data, err := os.ReadFile("prompts/" + id + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		s.sources[id] = data
	}
	names, err := fs.Glob(os.DirFS("."), "templates/*.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		s.partials[name] = data
	}
	return s
}

func TestConsumerGeneratedContract(t *testing.T) {
	// Arrange.
	runtime := readConsumerSnapshot(t)
	index := NewPromptIndex(nil)
	for _, id := range []PromptID{Main, Child} {
		t.Run(string(id), func(t *testing.T) {
			// Arrange.
			entry, ok := index.Lookup(id)
			if !ok {
				t.Fatal("missing generated entry")
			}
			descriptor := entry.Descriptor()
			payload, err := json.Marshal(prompty.PromptRecipeNoLateCheckpoint[MainInput]{Descriptor: descriptor, Input: MainInput{}})
			if err != nil {
				t.Fatal(err)
			}

			// Act.
			verifyErr := manifest.CheckpointVerify(t.Context(), descriptor, runtime)
			recipe, known, decodeErr := index.DecodeRecipeCheckpoint(id, payload)

			// Assert.
			if verifyErr != nil || !known || decodeErr != nil || recipe == nil {
				t.Fatalf("verify=%v known=%v decode=%v recipe=%v", verifyErr, known, decodeErr, recipe)
			}
			if _, err := recipe.ExecuteWithContract(t.Context(), runtime, nil); err != nil {
				t.Fatalf("execute captured recipe: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name    string
		id      PromptID
		payload []byte
		known   bool
	}{
		{"bad known payload", Main, []byte(`{`), true},
		{"unknown id", PromptID("unknown"), []byte(`{`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			// Act.
			recipe, known, err := index.DecodeRecipeCheckpoint(tc.id, tc.payload)
			// Assert.
			if recipe != nil || known != tc.known || (err != nil) != tc.known {
				t.Fatalf("recipe=%v known=%v error=%v", recipe, known, err)
			}
		})
	}
}

func TestConsumerStaleAndForgedCheckpoint(t *testing.T) {
	for _, change := range []string{"partial", "resolution", "import", "forged"} {
		t.Run(change, func(t *testing.T) {
			// Arrange.
			runtime := readConsumerSnapshot(t)
			entry, _ := NewPromptIndex(nil).Lookup(Main)
			desc := entry.Descriptor()
			switch change {
			case "partial":
				runtime.partials["templates/base.tmpl"] = []byte("changed")
			case "resolution":
				runtime.resolution["partial_resolution"] = "changed"
			case "import":
				runtime.sources["child"] = append(runtime.sources["child"], []byte("\n# changed\n")...)
			case "forged":
				desc.Digest = "source-bundle.v1:sha256:0000000000000000000000000000000000000000000000000000000000000000"
			}
			payload, err := json.Marshal(prompty.PromptRecipeNoLateCheckpoint[MainInput]{Descriptor: desc, Input: MainInput{}})
			if err != nil {
				t.Fatal(err)
			}
			recipe, known, err := NewPromptIndex(nil).DecodeRecipeCheckpoint(Main, payload)
			if !known || err != nil {
				t.Fatalf("decode: %v", err)
			}

			// Act.
			verifyErr := manifest.CheckpointVerify(t.Context(), desc, runtime)
			_, executeErr := recipe.ExecuteWithContract(t.Context(), runtime, nil)

			// Assert.
			if !errors.Is(verifyErr, prompty.ErrManifestDigestMismatch) || !errors.Is(executeErr, prompty.ErrManifestDigestMismatch) {
				t.Fatalf("verify=%v execute=%v", verifyErr, executeErr)
			}
		})
	}
}
