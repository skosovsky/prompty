package remoteregistry

import (
	"context"
	"errors"
	"fmt"

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

// Registry loads templates via Fetcher without internal cache/state.
// WithEnvironment(env): fetch resolves only id.env (no base-id fallback).
// Parser is required; use WithParser when creating the registry.
type Registry struct {
	fetcher Fetcher
	env     string // e.g. "prod"; Fetch tries id.env first
	parser  manifest.Unmarshaler
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

// loadTemplate returns a template by id (env-qualified when configured).
func (r *Registry) loadTemplate(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	candidates := fetchCandidateIDs(id, r.env)
	for _, cid := range candidates {
		tpl, err := r.getTemplateByID(ctx, cid)
		if err == nil {
			return tpl, nil
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// ReadManifestBytes fetches raw manifest bytes for digest computation.
func (r *Registry) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	for _, cid := range fetchCandidateIDs(id, r.env) {
		data, err := r.fetcher.Fetch(ctx, cid)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// ResolveManifest fetches manifest bytes and returns metadata without compiling template AST.
func (r *Registry) ResolveManifest(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	if err := ValidateID(id); err != nil {
		return prompty.TemplateDescriptor{}, err
	}
	for _, cid := range fetchCandidateIDs(id, r.env) {
		data, err := r.fetcher.Fetch(ctx, cid)
		if err == nil {
			return manifest.ParseDescriptor(data, r.parser)
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, prompty.ErrTemplateNotFound) {
			return prompty.TemplateDescriptor{}, err
		}
	}
	return prompty.TemplateDescriptor{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
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
	return prompty.NewRenderPlanFromPlanInput(tpl, input)
}

func (r *Registry) getTemplateByID(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := r.fetcher.Fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	tpl, err := manifest.Parse(data, r.parser)
	if err != nil {
		return nil, err
	}
	tpl.Metadata.Environment = ""
	if statter, ok := r.fetcher.(Statter); ok {
		if info, statErr := statter.Stat(ctx, id); statErr == nil && info.Version != "" && tpl.Metadata.Version == "" {
			tpl.Metadata.Version = info.Version
		}
	}
	return prompty.CloneTemplate(tpl), nil
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
