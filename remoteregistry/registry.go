package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

// Registry loads templates via Fetcher without internal cache/state.
// WithEnvironment(env): fetch resolves only id.env (no base-id fallback).
// Parser is required; use WithParser when creating the registry.
type Registry struct {
	fetcher Fetcher
	env     string // e.g. "prod"; Fetch tries id.env first
	parser  manifest.Unmarshaler

	composeMu   sync.RWMutex
	composeByID map[string]bool // last fetch: manifest declares imports/layers
}

// New creates a stateless Registry. Panics if fetcher is nil.
// Returns error when parser is not set.
func New(fetcher Fetcher, opts ...Option) (*Registry, error) {
	if fetcher == nil {
		panic("remoteregistry: Fetcher must not be nil")
	}
	r := &Registry{fetcher: fetcher}
	for _, opt := range opts {
		opt(r)
	}
	if r.parser == nil {
		return nil, prompty.ErrNoParser
	}
	return r, nil
}

// fetchCandidateIDs returns the manifest id to fetch (env-qualified when configured).
func fetchCandidateIDs(id, env string) []string {
	if env != "" {
		return []string{id + "." + env}
	}
	return []string{id}
}

// ReadManifestBytes fetches raw manifest bytes for digest computation.
func (r *Registry) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	for _, cid := range fetchCandidateIDs(id, r.env) {
		data, err := r.fetcher.Fetch(ctx, cid)
		if err == nil {
			if uses, peekErr := manifest.PeekComposeFieldsE(data, r.parser); peekErr == nil {
				r.recordComposeFlag(id, uses)
			}
			return data, nil
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// LoadByID implements manifest.ManifestLoader for declarative composition.
func (r *Registry) LoadByID(ctx context.Context, id string) (*manifest.RawManifest, error) {
	data, err := r.ReadManifestBytes(ctx, id)
	if err != nil {
		return nil, err
	}
	var raw manifest.RawManifest
	if unmarshalErr := r.parser.Unmarshal(data, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, unmarshalErr)
	}
	return &raw, nil
}

// ResolveManifest fetches manifest bytes and returns metadata without compiling template AST.
func (r *Registry) ResolveManifest(
	ctx context.Context,
	id string,
	opts ...prompty.ResolveManifestOption,
) (prompty.TemplateDescriptor, error) {
	if err := ValidateID(id); err != nil {
		return prompty.TemplateDescriptor{}, err
	}
	ro := prompty.ApplyResolveManifestOptions(opts)
	for _, cid := range fetchCandidateIDs(id, r.env) {
		data, err := r.fetcher.Fetch(ctx, cid)
		if err == nil {
			if uses, peekErr := manifest.PeekComposeFieldsE(data, r.parser); peekErr == nil {
				r.recordComposeFlag(id, uses)
			}
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
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			return prompty.TemplateDescriptor{}, err
		}
	}
	return prompty.TemplateDescriptor{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// DescribePrompt returns manifest metadata for routing and introspection.
// It resolves without compose values, so conditional imports/layers use conservative defaults.
// Pass prompty.WithResolveComposeContext to ResolveManifest when runtime compose values are known.
func (r *Registry) DescribePrompt(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	return r.ResolveManifest(ctx, id)
}

// Plan returns a deferred render plan for the selected prompt id.
func (r *Registry) Plan(ctx context.Context, id string, input prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	tpl, err := r.templateForPlan(ctx, id, input.ComposeValues())
	if err != nil {
		return nil, err
	}
	return prompty.NewRenderPlanFromPlanInput(tpl, input)
}

func (r *Registry) recordComposeFlag(logicalID string, usesCompose bool) {
	r.composeMu.Lock()
	defer r.composeMu.Unlock()
	if r.composeByID == nil {
		r.composeByID = make(map[string]bool)
	}
	r.composeByID[logicalID] = usesCompose
}

// ManifestUsesComposeE reports whether manifest bytes declare imports or layers.
// Always fetches current bytes; does not use cached compose hints (avoids stale flags after remote changes).
func (r *Registry) ManifestUsesComposeE(ctx context.Context, id string) (bool, error) {
	if err := ValidateID(id); err != nil {
		return false, err
	}
	return r.probeComposeFieldsE(ctx, id)
}

func (r *Registry) probeComposeFieldsE(ctx context.Context, id string) (bool, error) {
	var lastErr error
	for _, cid := range fetchCandidateIDs(id, r.env) {
		data, err := r.fetcher.Fetch(ctx, cid)
		if err == nil {
			uses, peekErr := manifest.PeekComposeOrError(data, r.parser)
			if peekErr != nil {
				return false, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
			}
			return uses, nil
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			lastErr = err
		}
	}
	if lastErr != nil {
		return false, lastErr
	}
	return false, nil
}

// KnownManifestUsesCompose returns a recorded compose flag without fetching.
func (r *Registry) KnownManifestUsesCompose(id string) (bool, bool) {
	r.composeMu.RLock()
	defer r.composeMu.RUnlock()
	if r.composeByID == nil {
		return false, false
	}
	v, ok := r.composeByID[id]
	return v, ok
}

func (r *Registry) getTemplateByIDWithComposeValues(
	ctx context.Context,
	fetchID string,
	logicalID string,
	values prompty.ComposeValues,
) (*prompty.ChatPromptTemplate, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := r.fetcher.Fetch(ctx, fetchID)
	if err != nil {
		return nil, err
	}
	usesCompose, peekErr := manifest.PeekComposeFieldsE(data, r.parser)
	if peekErr != nil {
		return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
	}
	r.recordComposeFlag(logicalID, usesCompose)
	var opts []manifest.ParseOption
	if usesCompose {
		opts = append(opts, manifest.WithCompose(manifest.ComposeContext{
			Ctx:                         ctx,
			Values:                      values,
			Loader:                      r,
			AllowMissingConditionValues: false,
		}))
	}
	tpl, err := manifest.Parse(data, r.parser, opts...)
	if err != nil {
		return nil, err
	}
	tpl.Metadata.Environment = ""
	if statter, ok := r.fetcher.(Statter); ok {
		if info, statErr := statter.Stat(ctx, fetchID); statErr == nil &&
			info.Version != "" && tpl.Metadata.Version == "" {
			tpl.Metadata.Version = info.Version
		}
	}
	return prompty.CloneTemplate(tpl), nil
}

func (r *Registry) templateForPlan(
	ctx context.Context,
	id string,
	values prompty.ComposeValues,
) (*prompty.ChatPromptTemplate, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	for _, cid := range fetchCandidateIDs(id, r.env) {
		tpl, err := r.getTemplateByIDWithComposeValues(ctx, cid, id, values)
		if err == nil {
			return tpl, nil
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// List returns ids from Fetcher if it implements Lister.
func (r *Registry) List(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if lister, ok := r.fetcher.(Lister); ok {
		return lister.ListIDs(ctx)
	}
	return nil, nil
}

// Stat returns metadata from Fetcher if it implements Statter.
func (r *Registry) Stat(ctx context.Context, id string) (prompty.TemplateInfo, error) {
	if err := ValidateID(id); err != nil {
		return prompty.TemplateInfo{}, err
	}
	if ctx.Err() != nil {
		return prompty.TemplateInfo{}, ctx.Err()
	}
	if statter, ok := r.fetcher.(Statter); ok {
		return statter.Stat(ctx, id)
	}
	return prompty.TemplateInfo{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// Close calls Close on the underlying Fetcher if it supports it.
func (r *Registry) Close() error {
	if c, ok := r.fetcher.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

// RecommendManifestDescriptor selects single-file or compose-closure checkpoint descriptor.
func (r *Registry) RecommendManifestDescriptor(ctx context.Context, id string) (prompty.ManifestDescriptor, error) {
	return manifest.CheckpointRecommend(ctx, id, r, r.parser)
}

// VerifyManifestDescriptor verifies a checkpoint descriptor against current manifest bytes.
func (r *Registry) VerifyManifestDescriptor(ctx context.Context, desc prompty.ManifestDescriptor) error {
	return manifest.CheckpointVerify(ctx, desc, r, r.parser)
}

var _ prompty.ManifestCheckpointRegistry = (*Registry)(nil)
