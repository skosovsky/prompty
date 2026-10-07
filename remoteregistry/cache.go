package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/skosovsky/prompty"
)

type cacheEntry struct {
	tpl       *prompty.ChatPromptTemplate
	snapshot  *prompty.ManifestSnapshot
	expiresAt time.Time
}

type inflightFetch struct {
	ctx     context.Context
	cancel  context.CancelFunc
	waiters int
	done    chan struct{}
	entry   *cacheEntry
	err     error
}

// CachedRegistry decorates any prompty.Registry with TTL cache and request deduplication.
type CachedRegistry struct {
	base     prompty.Registry
	ttl      time.Duration
	mu       sync.RWMutex
	cache    map[string]*cacheEntry
	inflight map[string]*inflightFetch
}

var (
	_ prompty.Registry         = (*CachedRegistry)(nil)
	_ prompty.Lister           = (*CachedRegistry)(nil)
	_ prompty.Statter          = (*CachedRegistry)(nil)
	_ prompty.ManifestResolver = (*CachedRegistry)(nil)
)

// WithCache wraps base registry with cache + in-flight request dedupe.
// TTL <= 0 means entries never expire.
func WithCache(base prompty.Registry, ttl time.Duration) *CachedRegistry {
	if base == nil {
		panic("remoteregistry: base registry must not be nil")
	}
	return &CachedRegistry{
		base:     base,
		ttl:      ttl,
		mu:       sync.RWMutex{},
		cache:    make(map[string]*cacheEntry),
		inflight: make(map[string]*inflightFetch),
	}
}

func (r *CachedRegistry) cacheEntryValid(ent *cacheEntry, now time.Time) bool {
	return r.ttl <= 0 || now.Before(ent.expiresAt)
}

func (r *CachedRegistry) loadMaterial(
	ctx context.Context,
	id string,
) (*cacheEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	now := time.Now()
	r.mu.RLock()
	ent, ok := r.cache[id]
	if ok && r.cacheEntryValid(ent, now) {
		entry := ent
		r.mu.RUnlock()
		return entry, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	now = time.Now()
	ent, ok = r.cache[id]
	if ok && r.cacheEntryValid(ent, now) {
		entry := ent
		r.mu.Unlock()
		return entry, nil
	}

	if inFlight, exists := r.inflight[id]; exists {
		inFlight.waiters++
		r.mu.Unlock()
		return r.waitForInflight(ctx, id, inFlight)
	}

	sharedCtx, cancel := context.WithCancel(context.Background())
	inFlight := &inflightFetch{
		ctx:     sharedCtx,
		cancel:  cancel,
		waiters: 1,
		done:    make(chan struct{}),
		entry:   nil,
		err:     nil,
	}
	r.inflight[id] = inFlight
	r.mu.Unlock()

	go r.runInflightFetch(id, inFlight)
	return r.waitForInflight(ctx, id, inFlight)
}

// ReadManifestBytes delegates to the base registry when supported.
func (r *CachedRegistry) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if reader, ok := r.base.(prompty.ManifestBytesReader); ok {
		return reader.ReadManifestBytes(ctx, id)
	}
	return nil, prompty.ErrManifestBytesUnavailable
}

// DescribePrompt delegates introspection to the base registry when it implements PromptDescriber.
func (r *CachedRegistry) DescribePrompt(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	if d, ok := r.base.(prompty.PromptDescriber); ok {
		return d.DescribePrompt(ctx, id)
	}
	return prompty.TemplateDescriptor{}, errors.New(
		"remoteregistry: base registry does not implement PromptDescriber",
	)
}

// CaptureManifest selects one cached immutable generation.
func (r *CachedRegistry) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	if _, ok := r.base.(prompty.ManifestSnapshotRegistry); !ok {
		return nil, errors.New("remoteregistry: base does not support captured materialization")
	}
	entry, err := r.loadMaterial(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry.snapshot == nil {
		return nil, errors.New("remoteregistry: missing source snapshot")
	}
	return entry.snapshot, nil
}

// RecommendManifestDescriptor uses the same TTL generation as Plan.
func (r *CachedRegistry) RecommendManifestDescriptor(
	ctx context.Context,
	id string,
) (prompty.ManifestDescriptor, error) {
	snapshot, err := r.CaptureManifest(ctx, id)
	if err != nil {
		return prompty.ManifestDescriptor{}, err
	}
	return snapshot.Descriptor(), nil
}

// VerifyManifestDescriptor compares against the generation selected by cache policy.
func (r *CachedRegistry) VerifyManifestDescriptor(ctx context.Context, desc prompty.ManifestDescriptor) error {
	snapshot, err := r.CaptureManifest(ctx, desc.ID)
	if err != nil {
		return err
	}
	if snapshot.Descriptor() != desc {
		return fmt.Errorf("%w: %q", prompty.ErrManifestDigestMismatch, desc.ID)
	}
	return nil
}

var _ prompty.ManifestCheckpointRegistry = (*CachedRegistry)(nil)

// ResolveManifest delegates metadata resolution to the base registry when supported.
func (r *CachedRegistry) ResolveManifest(
	ctx context.Context,
	id string,
	opts ...prompty.ResolveManifestOption,
) (prompty.TemplateDescriptor, error) {
	if resolver, ok := r.base.(prompty.ManifestResolver); ok {
		return resolver.ResolveManifest(ctx, id, opts...)
	}
	return prompty.TemplateDescriptor{}, errors.New(
		"remoteregistry: base registry does not implement ManifestResolver",
	)
}

// Plan uses the complete cached bundle; runtime values are evaluated within it.
func (r *CachedRegistry) Plan(
	ctx context.Context,
	id string,
	input prompty.RegistryPlanInput,
) (*prompty.RenderPlan, error) {
	if _, ok := r.base.(prompty.ManifestSnapshotRegistry); !ok && input.ComposeValues().IsSet() {
		return r.base.Plan(ctx, id, input)
	}
	entry, err := r.loadMaterial(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry.snapshot != nil {
		return entry.snapshot.Plan(ctx, input)
	}
	return prompty.NewRenderPlanFromPlanInput(entry.tpl, input)
}

func (r *CachedRegistry) runInflightFetch(id string, inFlight *inflightFetch) {
	defer inFlight.cancel()

	entry := &cacheEntry{tpl: nil, snapshot: nil, expiresAt: time.Time{}}
	var err error
	if source, ok := r.base.(prompty.ManifestSnapshotRegistry); ok {
		entry.snapshot, err = source.CaptureManifest(inFlight.ctx, id)
		if err == nil && (entry.snapshot == nil || entry.snapshot.Descriptor().ID != id) {
			err = errors.New("remoteregistry: nil captured snapshot")
		}
	} else {
		var plan *prompty.RenderPlan
		plan, err = r.base.Plan(inFlight.ctx, id, prompty.RegistryPlanInput{})
		if err == nil {
			entry.tpl, err = cachedPlanTemplate(plan, id)
		}
	}
	inFlight.entry = entry
	inFlight.err = err

	r.mu.Lock()
	if current, exists := r.inflight[id]; exists && current == inFlight {
		delete(r.inflight, id)
		if inFlight.err == nil {
			expiresAt := time.Now().Add(r.ttl)
			if r.ttl <= 0 {
				expiresAt = time.Time{}
			}
			entry.expiresAt = expiresAt
			r.cache[id] = entry
		}
	}
	r.mu.Unlock()

	close(inFlight.done)
}

func (r *CachedRegistry) waitForInflight(
	ctx context.Context,
	id string,
	inFlight *inflightFetch,
) (*cacheEntry, error) {
	defer r.releaseInflightWaiter(id, inFlight)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-inFlight.done:
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if inFlight.err != nil {
		return nil, inFlight.err
	}
	if inFlight.entry == nil {
		return nil, fmt.Errorf("remoteregistry: unexpected nil template for id %q", id)
	}
	return inFlight.entry, nil
}

func (r *CachedRegistry) releaseInflightWaiter(id string, inFlight *inflightFetch) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inFlight.waiters--
	if inFlight.waiters <= 0 {
		if current := r.inflight[id]; current == inFlight {
			delete(r.inflight, id)
		}
		inFlight.cancel()
	}
}

// List proxies to base when supported.
func (r *CachedRegistry) List(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if lister, ok := r.base.(prompty.Lister); ok {
		return lister.List(ctx)
	}
	return nil, nil
}

// Stat proxies to base when supported.
func (r *CachedRegistry) Stat(ctx context.Context, id string) (prompty.TemplateInfo, error) {
	if statter, ok := r.base.(prompty.Statter); ok {
		return statter.Stat(ctx, id)
	}
	return prompty.TemplateInfo{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
}

// Evict removes one entry from cache.
func (r *CachedRegistry) Evict(id string) {
	if err := ValidateID(id); err != nil {
		return
	}
	r.mu.Lock()
	delete(r.cache, id)
	delete(r.inflight, id) // Existing waiters retain their detached generation.
	r.mu.Unlock()
}

// EvictAll clears all cache entries.
func (r *CachedRegistry) EvictAll() {
	r.mu.Lock()
	r.cache = make(map[string]*cacheEntry)
	r.inflight = make(map[string]*inflightFetch)
	r.mu.Unlock()
}

// Close proxies to base when it supports Close.
func (r *CachedRegistry) Close() error {
	if c, ok := r.base.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

func cachedPlanTemplate(plan *prompty.RenderPlan, id string) (*prompty.ChatPromptTemplate, error) {
	if plan == nil || plan.Template() == nil {
		return nil, fmt.Errorf("remoteregistry: nil template for %q", id)
	}
	return plan.Template(), nil
}
