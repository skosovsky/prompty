package embedregistry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

// Ensures Registry implements prompty.Registry, Lister, and Statter.
var (
	_ prompty.Registry            = (*Registry)(nil)
	_ prompty.Lister              = (*Registry)(nil)
	_ prompty.Statter             = (*Registry)(nil)
	_ prompty.ManifestResolver    = (*Registry)(nil)
	_ prompty.ManifestBytesReader = (*Registry)(nil)
	_ prompty.PromptDescriber     = (*Registry)(nil)
)

// Registry loads all manifests from an [fs.FS] at construction (eager). No mutex. Holds parsed templates by id.
// WithEnvironment(env): Plan resolves only id.env (e.g. internal/router.prod), not base id.
// Parser is required; use WithParser when creating the registry.
type Registry struct {
	fsys            fs.FS
	cache           map[string]*prompty.ChatPromptTemplate
	ids             []string // ordered list of ids for List()
	root            string
	env             string // e.g. "prod"; Plan tries id.env first
	partialsPattern string // e.g. "partials/*.tmpl"; relative to root
	parser          manifest.Unmarshaler
	version         string // optional build/git version from WithVersion
}

// baseIDFromPath converts a manifest path (slash, no ext) to base ID: drops env suffix from basename.
// Example: "internal/router.prod" -> "internal/router".
func baseIDFromPath(slashPath string) string {
	base := filepath.Base(slashPath)
	if idx := strings.Index(base, "."); idx > 0 {
		base = base[:idx]
	}
	dir := filepath.Dir(slashPath)
	if dir == "." {
		return base
	}
	return filepath.ToSlash(filepath.Join(dir, base))
}

// underPartialsDir reports whether relPath (relative to walk root) is under partials pattern dir.
func underPartialsDir(relPath, partialsPattern string) bool {
	if partialsPattern == "" {
		return false
	}
	partialsDir := filepath.ToSlash(filepath.Dir(partialsPattern))
	if partialsDir == "." {
		return false
	}
	p := filepath.ToSlash(relPath)
	return p == partialsDir || strings.HasPrefix(p, partialsDir+"/")
}

// New walks fsys, parses every .yaml/.yml/.json under root, and returns a Registry.
// Cache keys are full id (agent, agent.prod); List returns base IDs only (agent).
// Parser is required (use WithParser).
func New(fsys fs.FS, root string, opts ...Option) (*Registry, error) {
	r := &Registry{
		fsys:  fsys,
		cache: make(map[string]*prompty.ChatPromptTemplate),
		root:  root,
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.parser == nil {
		return nil, prompty.ErrNoParser
	}
	seenID := make(map[string]bool)
	seenBaseID := make(map[string]bool)
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() ||
			(!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".json")) {
			return nil
		}
		relPath := path
		if root != "" && strings.HasPrefix(path, root+"/") {
			relPath = strings.TrimPrefix(path, root+"/")
		} else if root != "" && path == root {
			return nil
		}
		if underPartialsDir(relPath, r.partialsPattern) {
			return nil
		}
		var tpl *prompty.ChatPromptTemplate
		if r.partialsPattern != "" {
			partialsPath := filepath.Join(r.root, r.partialsPattern)
			tpl, err = manifest.ParseFS(fsys, path, r.parser, manifest.WithPartialsFS(fsys, partialsPath))
		} else {
			tpl, err = manifest.ParseFS(fsys, path, r.parser)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		slashPath := filepath.ToSlash(relPath)
		id := slashPath
		for _, ext := range []string{".yaml", ".yml", ".json"} {
			id = strings.TrimSuffix(id, ext)
		}
		tpl.Metadata.Environment = ""
		r.cache[id] = tpl
		if !seenID[id] {
			seenID[id] = true
			baseID := baseIDFromPath(id)
			if !seenBaseID[baseID] {
				seenBaseID[baseID] = true
				r.ids = append(r.ids, baseID)
			}
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

// WithEnvironment sets env for strict resolution: only id.env manifests are loaded.
func WithEnvironment(env string) Option {
	return func(r *Registry) { r.env = env }
}

// WithParser sets the manifest parser (required). Use manifest.NewJSONParser() or parser from github.com/skosovsky/prompty/parser/yaml for YAML.
func WithParser(u manifest.Unmarshaler) Option {
	return func(r *Registry) { r.parser = u }
}

// WithVersion sets a build or git version (e.g. from -ldflags). Stat returns it as TemplateInfo.Version.
func WithVersion(version string) Option {
	return func(r *Registry) { r.version = version }
}

// candidateIDs returns the manifest id to resolve (env-qualified when configured).
func candidateIDs(id, env string) []string {
	if env != "" {
		return []string{id + "." + env}
	}
	return []string{id}
}

// loadTemplate returns a template by id. O(1) map lookup. With env, only id.env is resolved. Enriches tpl.Metadata.Version from Stat if empty.
func (r *Registry) loadTemplate(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	for _, cid := range candidateIDs(id, r.env) {
		if tpl, ok := r.cache[cid]; ok {
			clone := prompty.CloneTemplate(tpl)
			info, _ := r.Stat(ctx, cid)
			if info.Version != "" && clone.Metadata.Version == "" {
				clone.Metadata.Version = info.Version
			}
			return clone, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

func (r *Registry) readManifestBytes(_ context.Context, id string) ([]byte, error) {
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	exts := []string{".yaml", ".yml", ".json"}
	for _, cid := range candidateIDs(id, r.env) {
		for _, ext := range exts {
			path := filepath.Join(r.root, filepath.FromSlash(cid+ext))
			data, err := fs.ReadFile(r.fsys, path)
			if err == nil {
				return data, nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// ReadManifestBytes returns raw manifest bytes for digest computation.
func (r *Registry) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	return r.readManifestBytes(ctx, id)
}

// ResolveManifest returns manifest metadata without compiling template AST.
func (r *Registry) ResolveManifest(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	data, err := r.readManifestBytes(ctx, id)
	if err != nil {
		return prompty.TemplateDescriptor{}, err
	}
	return manifest.ParseDescriptor(data, r.parser)
}

// DescribePrompt returns manifest metadata for routing and introspection.
func (r *Registry) DescribePrompt(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	return r.ResolveManifest(ctx, id)
}

// Plan returns a deferred render plan for the selected prompt id.
func (r *Registry) Plan(ctx context.Context, id string, input prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	tpl, err := r.loadTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return prompty.NewRenderPlanFromRegistryInput(tpl, input)
}

// List returns all template ids (order from walk).
func (r *Registry) List(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return append([]string(nil), r.ids...), nil
}

// Stat returns metadata for id without parsing. Uses the same strict env-qualified id as Plan.
// Version from WithVersion; UpdatedAt is zero for embed.
func (r *Registry) Stat(_ context.Context, id string) (prompty.TemplateInfo, error) {
	if err := prompty.ValidateID(id); err != nil {
		return prompty.TemplateInfo{}, err
	}
	for _, cid := range candidateIDs(id, r.env) {
		if _, ok := r.cache[cid]; ok {
			return prompty.TemplateInfo{
				ID:        id,
				Version:   r.version,
				UpdatedAt: time.Time{},
			}, nil
		}
	}
	return prompty.TemplateInfo{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}
