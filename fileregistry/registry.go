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
	dir   string
	mu    sync.RWMutex
	cache map[string]*prompty.ChatPromptTemplate
}

// New creates a Registry that reads YAML manifests from dir.
func New(dir string, _ ...Option) *Registry {
	return &Registry{
		dir:   dir,
		cache: make(map[string]*prompty.ChatPromptTemplate),
	}
}

// Option is a functional option for Registry (reserved for future use).
type Option func(*Registry)

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
	extensions := []string{".yaml", ".yml"}
	if env != "" {
		for _, ext := range extensions {
			path := filepath.Join(r.dir, name+"."+env+ext)
			tpl, err := manifest.ParseFile(path)
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
		tpl, err := manifest.ParseFile(path)
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
