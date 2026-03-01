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

// Ensures Registry implements prompty.PromptRegistry.
var _ prompty.PromptRegistry = (*Registry)(nil)

// Registry loads all YAML manifests from an fs.FS at construction (eager). No mutex. Holds parsed templates.
type Registry struct {
	cache           map[string]*prompty.ChatPromptTemplate
	root            string
	partialsPattern string // e.g. "partials/*.tmpl"; relative to root, one shared partials dir for all manifests
}

// New walks fsys, parses every .yaml file under the given root, and returns a Registry.
// Key format: "name" for "name.yaml", "name:env" for "name.env.yaml". Same name with different envs overwrite by last.
func New(fsys fs.FS, root string, opts ...Option) (*Registry, error) {
	r := &Registry{cache: make(map[string]*prompty.ChatPromptTemplate), root: root}
	for _, opt := range opts {
		opt(r)
	}
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		var tpl *prompty.ChatPromptTemplate
		if r.partialsPattern != "" {
			partialsPath := filepath.Join(r.root, r.partialsPattern)
			tpl, err = manifest.ParseFSWithOptions(fsys, path, manifest.WithPartialsFS(fsys, partialsPath))
		} else {
			tpl, err = manifest.ParseFS(fsys, path)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		base := filepath.Base(path)
		name := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			env := name[idx+1:]
			name = name[:idx]
			tpl.Metadata.Environment = env
			r.cache[name+":"+env] = tpl
		} else {
			tpl.Metadata.Environment = ""
			r.cache[name+":"] = tpl
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Option configures a Registry.
type Option func(*Registry)

// WithPartials sets a pattern relative to root for partials (e.g. "partials/*.tmpl"); one shared partials dir for all manifests.
func WithPartials(pattern string) Option {
	return func(r *Registry) { r.partialsPattern = pattern }
}

// GetTemplate returns a template by name and env. O(1) map lookup.
// Prefer name:env key; if missing, fallback to name: (base file).
// Check order: name validation, then context, then cache (aligned with other registries).
func (r *Registry) GetTemplate(ctx context.Context, name, env string) (*prompty.ChatPromptTemplate, error) {
	if err := prompty.ValidateName(name, env); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	key := name + ":" + env
	if tpl, ok := r.cache[key]; ok {
		return prompty.CloneTemplate(tpl), nil
	}
	if tpl, ok := r.cache[name+":"]; ok {
		clone := prompty.CloneTemplate(tpl)
		clone.Metadata.Environment = env
		return clone, nil
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, name)
}
