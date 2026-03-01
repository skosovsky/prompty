package fileregistry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

// Ensures Registry implements prompty.Registry.
var _ prompty.Registry = (*Registry)(nil)

// Registry loads prompt templates from the filesystem (lazy, cached).
// Resolves id to {dir}/{id}.yaml or {dir}/{id}.yml (id = basename without extension).
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

// idToPaths returns candidate paths for id in resolution order: id.yaml, id.yml.
func idToPaths(dir, id string) []string {
	return []string{
		filepath.Join(dir, id+".yaml"),
		filepath.Join(dir, id+".yml"),
	}
}

// GetTemplate returns a template by id. Lazy-loads and caches. After load, enriches tpl.Metadata.Version from Stat if empty.
func (r *Registry) GetTemplate(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	r.mu.RLock()
	tpl, ok := r.cache[id]
	r.mu.RUnlock()
	if ok {
		return prompty.CloneTemplate(tpl), nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tpl, ok = r.cache[id]
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
	for _, path := range idToPaths(r.dir, id) {
		tpl, err := parseFile(path)
		if err == nil {
			info, _ := r.Stat(ctx, id)
			if info.Version != "" && tpl.Metadata.Version == "" {
				tpl.Metadata.Version = info.Version
			}
			tpl.Metadata.Environment = "" // id-based; env expressed via id (e.g. doctor.prod)
			r.cache[id] = tpl
			return prompty.CloneTemplate(tpl), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// List returns all template ids (basename without .yaml/.yml) under r.dir, unique and sorted.
func (r *Registry) List(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	seen := make(map[string]bool)
	var ids []string
	err := fs.WalkDir(os.DirFS(r.dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		id := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
		if id == base {
			return nil
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fileregistry list: %w", err)
	}
	slices.Sort(ids)
	return ids, nil
}

// Stat returns metadata for id without parsing the manifest body. Version is file ModTime in RFC3339; UpdatedAt is file ModTime.
func (r *Registry) Stat(_ context.Context, id string) (prompty.TemplateInfo, error) {
	if err := prompty.ValidateID(id); err != nil {
		return prompty.TemplateInfo{}, err
	}
	for _, path := range idToPaths(r.dir, id) {
		fi, err := os.Stat(path)
		if err == nil {
			mod := fi.ModTime()
			return prompty.TemplateInfo{
				ID:        id,
				Version:   mod.Format(time.RFC3339),
				UpdatedAt: mod,
			}, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return prompty.TemplateInfo{}, err
		}
	}
	return prompty.TemplateInfo{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// Reload clears the cache (for hot-reload in development).
func (r *Registry) Reload() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*prompty.ChatPromptTemplate)
}
