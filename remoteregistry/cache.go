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
	id        string
	prev      *cacheEntry
	next      *cacheEntry
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
	base           prompty.Registry
	ttl            time.Duration
	mu             sync.RWMutex
	cache          map[string]*cacheEntry
	inflight       map[string]*inflightFetch
	maxEntries     int
	oldest, newest *cacheEntry
}

var (
	_ prompty.Registry         = (*CachedRegistry)(nil)
	_ prompty.Lister           = (*CachedRegistry)(nil)
	_ prompty.Statter          = (*CachedRegistry)(nil)
	_ prompty.ManifestResolver = (*CachedRegistry)(nil)
)

// WithCache wraps base with unbounded cache and in-flight request deduplication.
// TTL <= 0 means entries never expire. Positive-TTL entries are removed on
// material lookup and successful fetch publication, including unrelated IDs.
// Idle entries remain until the next lookup or explicit eviction; no worker runs.
func WithCache(base prompty.Registry, ttl time.Duration) *CachedRegistry {
	if base == nil {
		panic("remoteregistry: base registry must not be nil")
	}
	return &CachedRegistry{
		base:       base,
		ttl:        ttl,
		mu:         sync.RWMutex{},
		maxEntries: 0,
		oldest:     nil,
		newest:     nil,
		cache:      make(map[string]*cacheEntry),
		inflight:   make(map[string]*inflightFetch),
	}
}

// WithCacheLimit opts into a limit on retained entries, using FIFO successful
// publication order. Hits do not change order or TTL. TTL <= 0 disables time
// expiration, but capacity eviction can select a new generation on a later fetch.
// Caller-held snapshots and in-flight fetches are not bounded by maxEntries.
// It panics for nil base (checked first) or maxEntries <= 0.
func WithCacheLimit(base prompty.Registry, ttl time.Duration, maxEntries int) *CachedRegistry {
	r := WithCache(base, ttl)
	if maxEntries <= 0 {
		panic("remoteregistry: maxEntries must be positive")
	}
	r.maxEntries = maxEntries
	return r
}

func (r *CachedRegistry) cacheEntryValid(ent *cacheEntry, now time.Time) bool {
	return r.ttl <= 0 || now.Before(ent.expiresAt)
}

// removeEntry requires mu and clears both index links. It drops only cache
// ownership; callers and detached waiters retain their immutable generation.
func (r *CachedRegistry) removeEntry(entry *cacheEntry) {
	delete(r.cache, entry.id)
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		r.oldest = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		r.newest = entry.prev
	}
	entry.prev, entry.next = nil, nil
}

// Fixed TTL and serialized publication make FIFO order expiration order too.
// Maintenance visits only the expired prefix and at most one fresh head.
func (r *CachedRegistry) removeExpired(now time.Time) {
	if r.ttl <= 0 {
		return
	}
	removed := false
	for r.oldest != nil && !r.cacheEntryValid(r.oldest, now) {
		r.removeEntry(r.oldest)
		removed = true
	}
	if removed && len(r.cache) == 0 {
		r.cache = make(map[string]*cacheEntry)
	}
}

func (r *CachedRegistry) publishEntry(id string, entry *cacheEntry) {
	if previous := r.cache[id]; previous != nil {
		r.removeEntry(previous)
	}
	if r.maxEntries > 0 && len(r.cache) >= r.maxEntries {
		r.removeEntry(r.oldest)
	}
	entry.id, entry.prev = id, r.newest
	if r.newest != nil {
		r.newest.next = entry
	} else {
		r.oldest = entry
	}
	r.newest = entry
	r.cache[id] = entry
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
	ent := r.cache[id]
	var expiresAt, oldestExpiresAt time.Time
	hasOldest := r.oldest != nil
	if r.ttl > 0 {
		if ent != nil {
			expiresAt = ent.expiresAt
		}
		if hasOldest {
			oldestExpiresAt = r.oldest.expiresAt
		}
	}
	r.mu.RUnlock()
	// Compare copied deadlines outside the shared lock. A concurrent eviction
	// cannot invalidate this captured immutable generation. The slow path below
	// recomputes time and membership under the write lock before maintenance.
	if ent != nil && (r.ttl <= 0 || now.Before(expiresAt) &&
		(!hasOldest || now.Before(oldestExpiresAt))) {
		return ent, nil
	}

	r.mu.Lock()
	now = time.Now()
	r.removeExpired(now)
	if ent := r.cache[id]; ent != nil {
		if r.cacheEntryValid(ent, now) {
			r.mu.Unlock()
			return ent, nil
		}
		r.removeEntry(ent)
		if len(r.cache) == 0 {
			r.cache = make(map[string]*cacheEntry)
		}
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader, ok := r.base.(prompty.ManifestBytesReader); ok {
		return reader.ReadManifestBytes(ctx, id)
	}
	return nil, prompty.ErrManifestBytesUnavailable
}

// DescribePrompt delegates introspection to the base registry when it implements PromptDescriber.
func (r *CachedRegistry) DescribePrompt(ctx context.Context, id string) (prompty.TemplateDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return prompty.TemplateDescriptor{}, err
	}
	if d, ok := r.base.(prompty.PromptDescriber); ok {
		return d.DescribePrompt(ctx, id)
	}
	return prompty.TemplateDescriptor{}, fmt.Errorf("%w: DescribePrompt", prompty.ErrRegistryCapabilityUnavailable)
}

// CaptureManifest selects one cached immutable generation.
func (r *CachedRegistry) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := r.base.(prompty.ManifestSnapshotRegistry); !ok {
		return nil, fmt.Errorf("%w: CaptureManifest", prompty.ErrRegistryCapabilityUnavailable)
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
	if err := ctx.Err(); err != nil {
		return prompty.TemplateDescriptor{}, err
	}
	if resolver, ok := r.base.(prompty.ManifestResolver); ok {
		return resolver.ResolveManifest(ctx, id, opts...)
	}
	return prompty.TemplateDescriptor{}, fmt.Errorf("%w: ResolveManifest", prompty.ErrRegistryCapabilityUnavailable)
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

	entry := &cacheEntry{id: "", prev: nil, next: nil, tpl: nil, snapshot: nil, expiresAt: time.Time{}}
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
			now := time.Now()
			r.removeExpired(now)
			expiresAt := now.Add(r.ttl)
			if r.ttl <= 0 {
				expiresAt = time.Time{}
			}
			entry.expiresAt = expiresAt
			r.publishEntry(id, entry)
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
	return nil, fmt.Errorf("%w: List", prompty.ErrRegistryCapabilityUnavailable)
}

// Stat proxies to base when supported.
func (r *CachedRegistry) Stat(ctx context.Context, id string) (prompty.TemplateInfo, error) {
	if err := ctx.Err(); err != nil {
		return prompty.TemplateInfo{}, err
	}
	if statter, ok := r.base.(prompty.Statter); ok {
		return statter.Stat(ctx, id)
	}
	return prompty.TemplateInfo{}, fmt.Errorf("%w: Stat", prompty.ErrRegistryCapabilityUnavailable)
}

// Evict removes one entry from cache.
func (r *CachedRegistry) Evict(id string) {
	if err := ValidateID(id); err != nil {
		return
	}
	r.mu.Lock()
	if entry := r.cache[id]; entry != nil {
		r.removeEntry(entry)
		if len(r.cache) == 0 {
			r.cache = make(map[string]*cacheEntry)
		}
	}
	delete(r.inflight, id) // Existing waiters retain their detached generation.
	r.mu.Unlock()
}

// EvictAll clears all cache entries.
func (r *CachedRegistry) EvictAll() {
	r.mu.Lock()
	for r.oldest != nil {
		r.removeEntry(r.oldest)
	}
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
