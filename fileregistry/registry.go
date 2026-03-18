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

// Ensures Registry implements prompty.Registry, Lister, and Statter.
var (
	_ prompty.Registry = (*Registry)(nil)
	_ prompty.Lister   = (*Registry)(nil)
	_ prompty.Statter  = (*Registry)(nil)
)

// Registry loads prompt templates from the filesystem (lazy, cached).
// Resolves id to {dir}/{id}.yaml or {dir}/{id}.yml (id = basename without extension).
// WithEnvironment(env): tries {dir}/{id}.{env}.yaml first, then {dir}/{id}.yaml.
// Parser is required; use WithParser when creating the registry.
type Registry struct {
	dir             string
	env             string // e.g. "prod"; env inserted before extension: internal/router.prod.yaml
	partialsPattern string // e.g. "_partials/*.tmpl"; resolved relative to manifest dir when loading
	parser          manifest.Unmarshaler
	mu              sync.RWMutex
	cache           map[string]*prompty.ChatPromptTemplate
}

// New creates a Registry that reads manifests from dir. Parser is required (use WithParser).
func New(dir string, opts ...Option) (*Registry, error) {
	r := &Registry{
		dir:   dir,
		cache: make(map[string]*prompty.ChatPromptTemplate),
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.parser == nil {
		return nil, prompty.ErrNoParser
	}
	return r, nil
}

// Option configures a Registry.
type Option func(*Registry)

// WithPartials sets a relative pattern for partials (e.g. "_partials/*.tmpl"), resolved against the manifest directory when loading.
func WithPartials(relativePattern string) Option {
	return func(r *Registry) { r.partialsPattern = relativePattern }
}

// WithEnvironment sets env for fallback resolution: tries {id}.{env}.yaml first, then {id}.yaml.
// Example: id "internal/router", env "prod" -> internal/router.prod.yaml, then internal/router.yaml.
func WithEnvironment(env string) Option {
	return func(r *Registry) { r.env = env }
}

// WithParser sets the manifest parser (required). Use manifest.NewJSONParser() or parser/yaml for YAML.
func WithParser(u manifest.Unmarshaler) Option {
	return func(r *Registry) { r.parser = u }
}

// insertEnvBeforeExt returns base with env inserted before extension: "internal/router" + "prod" -> "internal/router.prod".
func insertEnvBeforeExt(base, env string) string {
	if env == "" {
		return base
	}
	return base + "." + env
}

// idToPaths returns candidate paths for id in resolution order (io/fs slash-style id).
// When env != "", tries {id}.{env}.yaml first, then base paths.
// Uses filepath.FromSlash(id) for Windows filesystem compatibility.
func idToPaths(dir, id, env string) []string {
	exts := []string{".yaml", ".yml", ".json"}
	var out []string
	base := insertEnvBeforeExt(id, env)
	if env != "" {
		for _, ext := range exts {
			path := filepath.FromSlash(base + ext)
			out = append(out, filepath.Join(dir, path))
		}
	}
	for _, ext := range exts {
		path := filepath.FromSlash(id + ext)
		out = append(out, filepath.Join(dir, path))
	}
	return out
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
			return manifest.ParseFile(path, r.parser, manifest.WithPartialsGlob(glob))
		}
		return manifest.ParseFile(path, r.parser)
	}
	for _, path := range idToPaths(r.dir, id, r.env) {
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

// baseIDFromPath converts a manifest path to base ID (slash format, no env suffix).
// Example: internal/router.prod.yaml -> internal/router. Algorithm: strip extension,
// then on basename drop everything after first dot as env suffix (router.prod -> router).
func baseIDFromPath(path string) string {
	slash := filepath.ToSlash(path)
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		slash = strings.TrimSuffix(slash, ext)
	}
	base := filepath.Base(slash)
	if idx := strings.Index(base, "."); idx > 0 {
		base = base[:idx]
	}
	dir := filepath.Dir(slash)
	if dir == "." {
		return base
	}
	return filepath.ToSlash(filepath.Join(dir, base))
}

// underPartialsDir reports whether path is under the partials pattern directory (to exclude from List).
func underPartialsDir(path, partialsPattern string) bool {
	if partialsPattern == "" {
		return false
	}
	partialsDir := filepath.ToSlash(filepath.Dir(partialsPattern))
	if partialsDir == "." {
		return false
	}
	p := filepath.ToSlash(path)
	return p == partialsDir || strings.HasPrefix(p, partialsDir+"/")
}

// List returns all template ids (base slash path, env suffix stripped) under r.dir, unique and sorted.
// agent.prod.yaml and agent.yaml both yield "agent"; internal/router.prod.yaml yields "internal/router".
// Paths under partials directory are excluded.
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
		slashPath := filepath.ToSlash(path)
		hasManifestExt := false
		for _, ext := range []string{".yaml", ".yml", ".json"} {
			if strings.HasSuffix(slashPath, ext) {
				hasManifestExt = true
				break
			}
		}
		if !hasManifestExt {
			return nil
		}
		if underPartialsDir(path, r.partialsPattern) {
			return nil
		}
		id := baseIDFromPath(path)
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
	for _, path := range idToPaths(r.dir, id, r.env) {
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
