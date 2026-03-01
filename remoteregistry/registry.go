package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"

	"golang.org/x/sync/singleflight"
)

const defaultTTL = 5 * time.Minute

// detachCancel returns a context that is not cancelled when parent is cancelled,
// but still respects parent's deadline so fetches (e.g. git clone) do not hang.
// The caller should call the returned cancel when done to release the deadline timer.
func detachCancel(parent context.Context) (context.Context, context.CancelFunc) {
	ctx := context.WithoutCancel(parent)
	if dl, ok := parent.Deadline(); ok {
		return context.WithDeadline(ctx, dl)
	}
	return context.WithCancel(ctx) // no-op cancel when no deadline, but same signature
}

// Ensures Registry implements prompty.Registry.
var _ prompty.Registry = (*Registry)(nil)

type cacheEntry struct {
	tpl       *prompty.ChatPromptTemplate
	expiresAt time.Time
}

// cacheEntryValid reports whether the entry is still valid at the given time.
func (r *Registry) cacheEntryValid(ent *cacheEntry, now time.Time) bool {
	return r.ttl <= 0 || now.Before(ent.expiresAt)
}

// Registry loads prompt templates via a Fetcher and caches them with TTL.
// Implements prompty.Registry. GetTemplate returns a cloned template.
type Registry struct {
	fetcher Fetcher
	ttl     time.Duration
	mu      sync.RWMutex
	cache   map[string]*cacheEntry
	sf      singleflight.Group
}

// New creates a Registry that uses the given Fetcher. Options (e.g. WithTTL) configure cache behavior.
// Panics if fetcher is nil.
func New(fetcher Fetcher, opts ...Option) *Registry {
	if fetcher == nil {
		panic("remoteregistry: Fetcher must not be nil")
	}
	r := &Registry{
		fetcher: fetcher,
		ttl:     defaultTTL,
		cache:   make(map[string]*cacheEntry),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// GetTemplate returns a template by id. Uses TTL cache; on miss or expiry, fetches via Fetcher.
// id must pass ValidateID. Returns a cloned template; enriches tpl.Metadata.Version from Stat if Fetcher implements Statter.
func (r *Registry) GetTemplate(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
	if err := ValidateID(id); err != nil {
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
	if ctx.Err() != nil {
		r.mu.Unlock()
		return nil, ctx.Err()
	}
	r.mu.Unlock()

	v, err, _ := r.sf.Do(id, func() (any, error) {
		fetchCtx, cancel := detachCancel(ctx)
		defer cancel()
		data, err := r.fetcher.Fetch(fetchCtx, id)
		if err != nil {
			return nil, err
		}
		tpl, err := manifest.ParseBytes(data)
		if err != nil {
			return nil, err
		}
		tpl.Metadata.Environment = ""
		if statter, ok := r.fetcher.(Statter); ok {
			if info, statErr := statter.Stat(fetchCtx, id); statErr == nil && info.Version != "" && tpl.Metadata.Version == "" {
				tpl.Metadata.Version = info.Version
			}
		}
		return tpl, nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
		}
		return nil, err
	}
	tpl := v.(*prompty.ChatPromptTemplate)

	r.mu.Lock()
	now = time.Now()
	expiresAt := now.Add(r.ttl)
	if r.ttl <= 0 {
		expiresAt = time.Time{}
	}
	r.cache[id] = &cacheEntry{tpl: tpl, expiresAt: expiresAt}
	r.mu.Unlock()
	return prompty.CloneTemplate(tpl), nil
}

// List returns template ids from the Fetcher if it implements Lister; otherwise returns nil, nil.
func (r *Registry) List(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if lister, ok := r.fetcher.(Lister); ok {
		return lister.ListIDs(ctx)
	}
	return nil, nil
}

// Stat returns template metadata from the Fetcher if it implements Statter; otherwise returns ErrTemplateNotFound.
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

// Evict removes one template from the cache by id. Safe for concurrent use.
func (r *Registry) Evict(id string) {
	if err := ValidateID(id); err != nil {
		return
	}
	r.mu.Lock()
	delete(r.cache, id)
	r.mu.Unlock()
}

// EvictAll clears the entire cache. Safe for concurrent use.
func (r *Registry) EvictAll() {
	r.mu.Lock()
	r.cache = make(map[string]*cacheEntry)
	r.mu.Unlock()
}

// Close calls Close on the underlying Fetcher if it implements the interface.
// Use this to clean up resources (e.g. git.Fetcher removes the local clone).
func (r *Registry) Close() error {
	if c, ok := r.fetcher.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}
