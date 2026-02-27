package embedregistry

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

// Registry loads all YAML manifests from an fs.FS at construction (eager). No mutex.
var _ prompty.PromptRegistry = (*Registry)(nil)

type Registry struct {
	cache map[string]*prompty.ChatPromptTemplate
}

// New walks fsys, parses every .yaml file under the given root, and returns a Registry.
// Key format: "name" for "name.yaml", "name:env" for "name.env.yaml". Same name with different envs overwrite by last.
func New(fsys fs.FS, root string, _ ...Option) (*Registry, error) {
	r := &Registry{cache: make(map[string]*prompty.ChatPromptTemplate)}
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		tpl, err := manifest.ParseFS(fsys, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		base := filepath.Base(path)
		name := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			env := name[idx+1:]
			name = name[:idx]
			r.cache[name+":"+env] = tpl
		} else {
			r.cache[name+":"] = tpl
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Option is a functional option for Registry (reserved for future use).
type Option func(*Registry)

// GetTemplate returns a template by name and env. O(1) map lookup.
// Prefer name:env key; if missing, fallback to name: (base file).
func (r *Registry) GetTemplate(ctx context.Context, name, env string) (*prompty.ChatPromptTemplate, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	key := name + ":" + env
	if tpl, ok := r.cache[key]; ok {
		return prompty.CloneTemplate(tpl), nil
	}
	if tpl, ok := r.cache[name+":"]; ok {
		return prompty.CloneTemplate(tpl), nil
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, name)
}
