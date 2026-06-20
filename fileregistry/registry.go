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
	_ prompty.Registry               = (*Registry)(nil)
	_ prompty.Lister                 = (*Registry)(nil)
	_ prompty.Statter                = (*Registry)(nil)
	_ prompty.ManifestResolver       = (*Registry)(nil)
	_ prompty.ManifestBytesReader    = (*Registry)(nil)
	_ prompty.ManifestComposeChecker = (*Registry)(nil)
	_ prompty.PromptDescriber        = (*Registry)(nil)
)

// Registry loads prompt templates from the filesystem (lazy, cached).
// Resolves id to {dir}/{id}.yaml or {dir}/{id}.yml (id = basename without extension).
// WithEnvironment(env): resolves only {dir}/{id}.{env}.yaml|.yml|.json (no base-id fallback).
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

// WithEnvironment sets env for strict resolution: only {id}.{env} manifest paths are tried.
// Example: id "internal/router", env "prod" -> internal/router.prod.yaml (not internal/router.yaml).
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

// idToPaths returns manifest paths for id (io/fs slash-style id).
// When env is set, only {id}.{env} variants are resolved (no base-id fallback).
func idToPaths(dir, id, env string) []string {
	exts := []string{".yaml", ".yml", ".json"}
	var out []string
	resolvedID := id
	if env != "" {
		resolvedID = insertEnvBeforeExt(id, env)
	}
	for _, ext := range exts {
		path := filepath.FromSlash(resolvedID + ext)
		out = append(out, filepath.Join(dir, path))
	}
	return out
}

func (r *Registry) readManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	for _, path := range idToPaths(r.dir, id, r.env) {
		data, err := os.ReadFile(path) // #nosec G304 -- path is validated by idToPaths
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// ReadManifestBytes returns raw manifest bytes for digest and metadata-only paths.
func (r *Registry) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	return r.readManifestBytes(ctx, id)
}

// LoadByID implements manifest.ManifestLoader for declarative composition.
func (r *Registry) LoadByID(ctx context.Context, id string) (*manifest.RawManifest, error) {
	data, err := r.readManifestBytes(ctx, id)
	if err != nil {
		return nil, err
	}
	var raw manifest.RawManifest
	if unmarshalErr := r.parser.Unmarshal(data, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, unmarshalErr)
	}
	return &raw, nil
}

// ResolveManifest returns manifest metadata without compiling template AST.
// Without ResolveManifestOption values, composed manifests use a conservative union of imports.
func (r *Registry) ResolveManifest(
	ctx context.Context,
	id string,
	opts ...prompty.ResolveManifestOption,
) (prompty.TemplateDescriptor, error) {
	data, err := r.readManifestBytes(ctx, id)
	if err != nil {
		return prompty.TemplateDescriptor{}, err
	}
	ro := prompty.ApplyResolveManifestOptions(opts)
	parseOpts := []manifest.ParseOption{manifest.WithCompose(manifest.ComposeContext{
		Ctx:                         ctx,
		Values:                      prompty.ComposeValues{},
		Loader:                      r,
		AllowMissingConditionValues: true,
	})}
	if values, ok := ro.ComposeValues(); ok {
		parseOpts = append(parseOpts, manifest.WithComposeValues(values))
	}
	return manifest.ParseDescriptor(data, r.parser, parseOpts...)
}

// DescribePrompt returns manifest metadata for routing and introspection.
// It resolves without compose values, so conditional imports/layers use conservative defaults.
// Pass prompty.WithResolveComposeContext to ResolveManifest when runtime compose values are known.
func (r *Registry) DescribePrompt(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	return r.ResolveManifest(ctx, id)
}

// loadTemplate returns a template by id. Lazy-loads and caches. After load, enriches tpl.Metadata.Version from Stat if empty.
func (r *Registry) loadTemplate(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
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
		opts := r.parseOptionsForPath(ctx, path, prompty.ComposeValues{})
		return manifest.ParseFile(path, r.parser, opts...)
	}
	for _, path := range idToPaths(r.dir, id, r.env) {
		tpl, err := parseFile(path)
		if err == nil {
			info, _ := r.Stat(ctx, id)
			if info.Version != "" && tpl.Metadata.Version == "" {
				tpl.Metadata.Version = info.Version
			}
			tpl.Metadata.Environment = "" // id-based; env expressed via id (e.g. doctor.prod)
			if !r.pathUsesCompose(path) {
				r.cache[id] = tpl
			}
			return prompty.CloneTemplate(tpl), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// Plan returns a deferred render plan for the selected prompt id.
func (r *Registry) Plan(ctx context.Context, id string, input prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	tpl, err := r.templateForPlan(ctx, id, input.ComposeValues())
	if err != nil {
		return nil, err
	}
	return prompty.NewRenderPlanFromPlanInput(tpl, input)
}

func (r *Registry) pathUsesCompose(path string) bool {
	data, err := os.ReadFile(path) // #nosec G304 -- path from idToPaths
	if err != nil {
		return false
	}
	uses, peekErr := manifest.PeekComposeOrError(data, r.parser)
	if peekErr != nil {
		return true // skip cache when peek is ambiguous
	}
	return uses
}

// ManifestUsesComposeE reports whether raw manifest bytes declare imports or layers.
func (r *Registry) ManifestUsesComposeE(ctx context.Context, id string) (bool, error) {
	data, err := r.ReadManifestBytes(ctx, id)
	if err != nil {
		if errors.Is(err, prompty.ErrTemplateNotFound) {
			return false, nil
		}
		return false, err
	}
	uses, peekErr := manifest.PeekComposeOrError(data, r.parser)
	if peekErr != nil {
		return false, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
	}
	return uses, nil
}

func (r *Registry) parseOptionsForPath(
	ctx context.Context,
	path string,
	values prompty.ComposeValues,
) []manifest.ParseOption {
	var opts []manifest.ParseOption
	if r.partialsPattern != "" {
		glob := filepath.Join(filepath.Dir(path), r.partialsPattern)
		opts = append(opts, manifest.WithPartialsGlob(glob))
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path from idToPaths
	if err == nil {
		usesCompose, peekErr := manifest.PeekComposeOrError(data, r.parser)
		if peekErr == nil && usesCompose {
			opts = append(opts, manifest.WithCompose(manifest.ComposeContext{
				Ctx:                         ctx,
				Values:                      values,
				Loader:                      r,
				AllowMissingConditionValues: false,
			}))
		}
	}
	return opts
}

func (r *Registry) templateForPlan(
	ctx context.Context,
	id string,
	values prompty.ComposeValues,
) (*prompty.ChatPromptTemplate, error) {
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	for _, path := range idToPaths(r.dir, id, r.env) {
		data, err := os.ReadFile(path) // #nosec G304 -- path from idToPaths
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		usesCompose, peekErr := manifest.PeekComposeFieldsE(data, r.parser)
		if peekErr != nil {
			return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
		}
		if usesCompose {
			return manifest.Parse(data, r.parser, r.parseOptionsForPath(ctx, path, values)...)
		}
		break
	}
	return r.loadTemplate(ctx, id)
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

// RecommendManifestDescriptor selects single-file or compose-closure checkpoint descriptor.
func (r *Registry) RecommendManifestDescriptor(ctx context.Context, id string) (prompty.ManifestDescriptor, error) {
	return manifest.CheckpointRecommend(ctx, id, r, r.parser)
}

// VerifyManifestDescriptor verifies a checkpoint descriptor against current manifest bytes.
func (r *Registry) VerifyManifestDescriptor(ctx context.Context, desc prompty.ManifestDescriptor) error {
	return manifest.CheckpointVerify(ctx, desc, r, r.parser)
}

// Reload clears the cache (for hot-reload in development).
func (r *Registry) Reload() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*prompty.ChatPromptTemplate)
}

var _ prompty.ManifestCheckpointRegistry = (*Registry)(nil)
