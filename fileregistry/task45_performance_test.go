package fileregistry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/skosovsky/prompty/parser/yaml"
)

func BenchmarkTask45ConcurrentCatalogCapture(b *testing.B) {
	dir := b.TempDir()
	ids := make([]string, 8)
	for i := range ids {
		ids[i] = "prompt" + strconv.Itoa(i)
		data := fmt.Appendf(nil, "id: %s\nversion: \"1\"\nmessages:\n  - role: user\n    content: known\n", ids[i])
		if err := os.WriteFile(filepath.Join(dir, ids[i]+".yaml"), data, 0600); err != nil {
			b.Fatal(err)
		}
	}
	for _, cold := range []bool{false, true} {
		b.Run("cold="+strconv.FormatBool(cold), func(b *testing.B) {
			registry, err := New(dir, WithParser(yaml.New()))
			if err != nil {
				b.Fatal(err)
			}
			benchmarkCatalogWarmup(b, registry, ids)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if cold {
					registry.Reload()
				}
				benchmarkCatalogBatch(b, registry, ids)
			}
		})
	}
}

func benchmarkCatalogBatch(b *testing.B, registry *Registry, ids []string) {
	b.Helper()
	errors := make(chan error, len(ids))
	for _, id := range ids {
		go func() {
			snapshot, err := registry.CaptureManifest(context.Background(), id)
			if err == nil && snapshot.Descriptor().ID != id {
				err = fmt.Errorf("wrong captured ID: %s", id)
			}
			errors <- err
		}()
	}
	for range ids {
		if err := <-errors; err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCatalogWarmup(b *testing.B, registry *Registry, ids []string) {
	b.Helper()
	for _, id := range ids {
		if _, err := registry.CaptureManifest(context.Background(), id); err != nil {
			b.Fatal(err)
		}
	}
}
