package embedregistry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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

// Registry seals source bytes from an [fs.FS] at construction and caches immutable snapshots.
// WithEnvironment(env): Plan resolves only id.env (e.g. internal/router.prod), not base id.
// Parser is required; use WithParser when creating the registry.
type Registry struct {
	sources         map[string][]byte
	cache           map[string]*prompty.ManifestSnapshot
	mu              sync.Mutex
	ids             []string
	root            string
	env             string
	partialsPattern string
	parser          manifest.Unmarshaler
	version         string
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
// List returns base IDs; snapshot capture applies strict environment resolution.
// Parser is required (use WithParser).
func New(fsys fs.FS, root string, opts ...Option) (*Registry, error) {
	r := &Registry{root: root, sources: make(map[string][]byte), cache: make(map[string]*prompty.ManifestSnapshot)}
	for _, opt := range opts {
		opt(r)
	}
	if r.parser == nil {
		return nil, prompty.ErrNoParser
	}
	seen := make(map[string]bool)
	err := fs.WalkDir(fsys, root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(name, strings.TrimSuffix(root, "/")+"/")
		if root == "." {
			relative = name
		}
		isManifest := !underPartialsDir(relative, r.partialsPattern) &&
			(strings.HasSuffix(relative, ".yaml") || strings.HasSuffix(relative, ".yml") || strings.HasSuffix(relative, ".json"))
		isPartial := false
		if r.partialsPattern != "" {
			var matchErr error
			isPartial, matchErr = filepath.Match(filepath.Join(root, r.partialsPattern), name)
			if matchErr != nil {
				return matchErr
			}
		}
		if !isManifest && !isPartial {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		r.sources[name] = append([]byte(nil), data...)
		if !isManifest {
			return nil
		}
		for _, ext := range []string{".yaml", ".yml", ".json"} {
			if strings.HasSuffix(relative, ext) {
				var raw manifest.RawManifest
				if err := r.parser.Unmarshal(append([]byte(nil), data...), &raw); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				base := baseIDFromPath(strings.TrimSuffix(relative, ext))
				if !seen[base] {
					seen[base] = true
					r.ids = append(r.ids, base)
				}
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(r.ids)
	// Validate available selected manifests eagerly; missing environment variants remain unavailable.
	for _, id := range r.ids {
		if _, err := r.readManifestBytes(context.Background(), id); errors.Is(err, prompty.ErrTemplateNotFound) {
			continue
		}
		snapshot, err := r.CaptureManifest(context.Background(), id)
		if err != nil {
			return nil, err
		}
		// Conservative descriptor parsing validates composition without demanding runtime conditions.
		if err := manifest.ValidateCapturedSources(context.Background(), id, snapshot.Sources(), r.parser); err != nil {
			return nil, err
		}
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

func (r *Registry) readManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	exts := []string{".yaml", ".yml", ".json"}
	for _, cid := range candidateIDs(id, r.env) {
		for _, ext := range exts {
			path := filepath.Join(r.root, filepath.FromSlash(cid+ext))
			if data, ok := r.sources[path]; ok {
				return append([]byte(nil), data...), nil
			}
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// ReadManifestBytes returns raw manifest bytes for digest computation.
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
	if unmarshalErr := r.parser.Unmarshal(append([]byte(nil), data...), &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, unmarshalErr)
	}
	return &raw, nil
}

// ResolveManifest returns manifest metadata without compiling template AST.
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

// CaptureManifest returns a snapshot from bytes sealed at construction.
func (r *Registry) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snapshot, ok := r.cache[id]; ok {
		return snapshot, nil
	}
	partials := make(map[string][]byte)
	if r.partialsPattern != "" {
		pattern := filepath.ToSlash(filepath.Join(r.root, r.partialsPattern))
		for name, data := range r.sources {
			matches, err := filepath.Match(pattern, name)
			if err != nil {
				return nil, err
			}
			if matches {
				partials[name] = data
			}
		}
		if len(partials) == 0 {
			return nil, fmt.Errorf("%w: no matching partials %q", prompty.ErrTemplateParse, r.partialsPattern)
		}
	}
	snapshot, err := manifest.CaptureSources(
		ctx,
		id,
		r,
		r.parser,
		partials,
		manifest.SourceResolutionConfig(r.env, r.partialsPattern, "relative-root.v1", r.version, ""),
	)
	if err != nil {
		return nil, err
	}
	r.cache[id] = snapshot
	return snapshot, nil
}

// Plan builds from captured sources only.
func (r *Registry) Plan(ctx context.Context, id string, input prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	snapshot, err := r.CaptureManifest(ctx, id)
	if err != nil {
		return nil, err
	}
	return snapshot.Plan(ctx, input)
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
func (r *Registry) Stat(ctx context.Context, id string) (prompty.TemplateInfo, error) {
	if err := prompty.ValidateID(id); err != nil {
		return prompty.TemplateInfo{}, err
	}
	_, readErr := r.readManifestBytes(ctx, id)
	if contextErr := ctx.Err(); contextErr != nil {
		return prompty.TemplateInfo{}, errors.Join(contextErr, readErr)
	}
	if readErr != nil {
		return prompty.TemplateInfo{}, readErr
	}
	if err := ctx.Err(); err != nil {
		return prompty.TemplateInfo{}, err
	}
	return prompty.TemplateInfo{ID: id, Version: r.version, UpdatedAt: time.Time{}}, nil
}

// ManifestUsesComposeE reports whether raw manifest bytes declare imports or layers.
func (r *Registry) ManifestUsesComposeE(ctx context.Context, id string) (bool, error) {
	data, err := r.readManifestBytes(ctx, id)
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

// RecommendManifestDescriptor returns the identity selected by capture policy.
func (r *Registry) RecommendManifestDescriptor(ctx context.Context, id string) (prompty.ManifestDescriptor, error) {
	return manifest.CheckpointRecommend(ctx, id, r)
}

// VerifyManifestDescriptor verifies against the generation selected by capture policy.
func (r *Registry) VerifyManifestDescriptor(ctx context.Context, desc prompty.ManifestDescriptor) error {
	return manifest.CheckpointVerify(ctx, desc, r)
}

var _ prompty.ManifestCheckpointRegistry = (*Registry)(nil)
