package remoteregistry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/skosovsky/prompty"
)

type cacheEntry struct {
	tpl       *prompty.ChatPromptTemplate
	expiresAt time.Time
}

type inflightFetch struct {
	ctx     context.Context
	cancel  context.CancelFunc
	waiters int
	done    chan struct{}
	tpl     *prompty.ChatPromptTemplate
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
	_ prompty.Registry = (*CachedRegistry)(nil)
	_ prompty.Lister   = (*CachedRegistry)(nil)
	_ prompty.Statter  = (*CachedRegistry)(nil)
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

func (r *CachedRegistry) GetTemplate(
	ctx context.Context,
	id string,
) (*prompty.ChatPromptTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	now := time.Now()
	r.mu.RLock()
	ent, ok := r.cache[id]
	if ok && r.cacheEntryValid(ent, now) {
		tpl := prompty.CloneTemplate(ent.tpl)
		r.mu.RUnlock()
		return tpl, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	now = time.Now()
	ent, ok = r.cache[id]
	if ok && r.cacheEntryValid(ent, now) {
		tpl := prompty.CloneTemplate(ent.tpl)
		r.mu.Unlock()
		return tpl, nil
	}

	if inFlight, exists := r.inflight[id]; exists {
		inFlight.waiters++
		r.mu.Unlock()
		return r.waitForInflight(ctx, id, inFlight)
	}

	//nolint:gosec // cancel is stored in inflight and called by waiter lifecycle / fetch completion.
	sharedCtx, cancel := context.WithCancel(context.Background())
	inFlight := &inflightFetch{
		ctx:     sharedCtx,
		cancel:  cancel,
		waiters: 1,
		done:    make(chan struct{}),
		tpl:     nil,
		err:     nil,
	}
	r.inflight[id] = inFlight
	r.mu.Unlock()

	go r.runInflightFetch(id, inFlight)
	return r.waitForInflight(ctx, id, inFlight)
}

func (r *CachedRegistry) runInflightFetch(id string, inFlight *inflightFetch) {
	defer inFlight.cancel()

	tpl, err := r.base.GetTemplate(inFlight.ctx, id)
	if err == nil {
		if tpl == nil {
			err = fmt.Errorf("remoteregistry: unexpected nil template for id %q", id)
		} else {
			inFlight.tpl = prompty.CloneTemplate(tpl)
		}
	}
	inFlight.err = err

	r.mu.Lock()
	if current, exists := r.inflight[id]; exists && current == inFlight {
		delete(r.inflight, id)
		if inFlight.err == nil {
			stored := prompty.CloneTemplate(inFlight.tpl)
			expiresAt := time.Now().Add(r.ttl)
			if r.ttl <= 0 {
				expiresAt = time.Time{}
			}
			r.cache[id] = &cacheEntry{tpl: stored, expiresAt: expiresAt}
		}
	}
	r.mu.Unlock()

	close(inFlight.done)
}

func (r *CachedRegistry) waitForInflight(
	ctx context.Context,
	id string,
	inFlight *inflightFetch,
) (*prompty.ChatPromptTemplate, error) {
	defer r.releaseInflightWaiter(id, inFlight)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-inFlight.done:
	}

	if inFlight.err != nil {
		return nil, inFlight.err
	}
	if inFlight.tpl == nil {
		return nil, fmt.Errorf("remoteregistry: unexpected nil template for id %q", id)
	}
	return prompty.CloneTemplate(inFlight.tpl), nil
}

func (r *CachedRegistry) releaseInflightWaiter(id string, inFlight *inflightFetch) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, exists := r.inflight[id]
	if !exists || current != inFlight {
		return
	}

	current.waiters--
	if current.waiters <= 0 {
		delete(r.inflight, id)
		current.cancel()
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
	r.mu.Unlock()
}

// EvictAll clears all cache entries.
func (r *CachedRegistry) EvictAll() {
	r.mu.Lock()
	r.cache = make(map[string]*cacheEntry)
	r.mu.Unlock()
}

// Close proxies to base when it supports Close.
func (r *CachedRegistry) Close() error {
	if c, ok := r.base.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}
