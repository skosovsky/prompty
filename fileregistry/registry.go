package fileregistry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

// Ensures Registry implements prompty.PromptRegistry.
var _ prompty.PromptRegistry = (*Registry)(nil)

// Registry loads prompt templates from the filesystem (lazy, cached).
// Resolves name+env to {dir}/{name}.{env}.yaml with fallback to {dir}/{name}.yaml.
type Registry struct {
	dir             string
	partialsPattern string // e.g. "_partials/*.tmpl"; resolved relative to manifest dir when loading
	mu              sync.RWMutex
	cache           map[string]*prompty.ChatPromptTemplate
}

// New creates a Registry that reads YAML manifests from dir.
func New(dir string, opts ...Option) *Registry {
	r := &Registry{
		dir:   dir,
		cache: make(map[string]*prompty.ChatPromptTemplate),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Option configures a Registry.
type Option func(*Registry)

// WithPartials sets a relative pattern for partials (e.g. "_partials/*.tmpl"), resolved against the manifest directory when loading.
func WithPartials(relativePattern string) Option {
	return func(r *Registry) { r.partialsPattern = relativePattern }
}

// GetTemplate returns a template by name and env. Lazy-loads and caches.
// File resolution: {dir}/{name}.{env}.yaml or .yml, fallback {dir}/{name}.yaml or .yml.
func (r *Registry) GetTemplate(ctx context.Context, name, env string) (*prompty.ChatPromptTemplate, error) {
	if err := prompty.ValidateName(name, env); err != nil {
		return nil, err
	}
	key := name + ":" + env
	r.mu.RLock()
	tpl, ok := r.cache[key]
	r.mu.RUnlock()
	if ok {
		return prompty.CloneTemplate(tpl), nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tpl, ok = r.cache[key]
	if ok {
		return prompty.CloneTemplate(tpl), nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	parseFile := func(path string) (*prompty.ChatPromptTemplate, error) {
		if r.partialsPattern != "" {
			glob := filepath.Join(filepath.Dir(path), r.partialsPattern)
			return manifest.ParseFileWithOptions(path, manifest.WithPartialsGlob(glob))
		}
		return manifest.ParseFile(path)
	}
	extensions := []string{".yaml", ".yml"}
	if env != "" {
		for _, ext := range extensions {
			path := filepath.Join(r.dir, name+"."+env+ext)
			tpl, err := parseFile(path)
			if err == nil {
				tpl.Metadata.Environment = env
				r.cache[key] = tpl
				return prompty.CloneTemplate(tpl), nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	for _, ext := range extensions {
		path := filepath.Join(r.dir, name+ext)
		tpl, err := parseFile(path)
		if err == nil {
			tpl.Metadata.Environment = env
			r.cache[key] = tpl
			return prompty.CloneTemplate(tpl), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, name)
}

// Reload clears the cache (for hot-reload in development).
func (r *Registry) Reload() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*prompty.ChatPromptTemplate)
}
