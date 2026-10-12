package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

type retentionRegistry struct {
	calls atomic.Int64
	fail  atomic.Bool
}

func (r *retentionRegistry) Plan(
	_ context.Context,
	id string,
	input prompty.RegistryPlanInput,
) (*prompty.RenderPlan, error) {
	r.calls.Add(1)
	if r.fail.Load() {
		return nil, errors.New("retention fixture fetch failure")
	}
	tpl, err := prompty.NewChatPromptTemplate(
		[]prompty.MessageTemplate{
			{Role: prompty.RoleUser, Content: prompty.TextContent(id + strings.Repeat("x", 64<<10))},
		},
		prompty.WithMetadata(prompty.PromptMetadata{ID: id, Version: "1"}),
	)
	if err != nil {
		return nil, err
	}
	return prompty.NewRenderPlanFromPlanInput(tpl, input)
}

func expireRetentionEntries(cache *CachedRegistry, preserve string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for id, entry := range cache.cache {
		if id != preserve {
			entry.expiresAt = time.Now().Add(-time.Hour)
		}
	}
}

func retainedExpiredPayload(cache *CachedRegistry) (int, int) {
	var entries, bytes int
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	for _, entry := range cache.cache {
		if cache.cacheEntryValid(entry, time.Now()) {
			continue
		}
		entries++
		if entry.tpl != nil {
			for _, message := range entry.tpl.Messages {
				for _, part := range message.Content {
					bytes += len(part.Text)
				}
			}
		}
	}
	return entries, bytes
}

func TestCacheLookupReleasesExpiredPayload(t *testing.T) {
	for _, count := range []int{16, 128, 512} {
		for _, trigger := range []string{"hit", "miss"} {
			t.Run(fmt.Sprintf("%d/%s", count, trigger), func(t *testing.T) {
				checkExpiredPayloadRelease(t, count, trigger)
			})
		}
	}
}

func checkExpiredPayloadRelease(t *testing.T, count int, trigger string) {
	t.Helper()
	// Arrange: load distinct 64KiB templates and keep one unrelated fresh ID.
	base := &retentionRegistry{}
	cache := WithCache(base, time.Hour)
	for i := range count {
		if _, fetchErr := cache.Plan(t.Context(), strconv.Itoa(i), prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
	}
	if _, fetchErr := cache.Plan(t.Context(), "fresh", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	expireRetentionEntries(cache, "fresh")
	before := base.calls.Load()
	id := "fresh"
	if trigger == "miss" {
		id = "unrelated"
	}
	// Act: a lookup, including a hit, is the documented cleanup trigger.
	if _, fetchErr := cache.Plan(t.Context(), id, prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	// Assert: no expired template payload remains reachable through the cache.
	entries, bytes := retainedExpiredPayload(cache)
	if entries != 0 || bytes != 0 {
		t.Fatalf("expired entries=%d reachable bytes=%d", entries, bytes)
	}
	wantCalls := before
	if trigger == "miss" {
		wantCalls++
	}
	if got := base.calls.Load(); got != wantCalls {
		t.Fatalf("fetches=%d want=%d", got, wantCalls)
	}
}

func TestCacheIdleExpiryWaitsForLookup(t *testing.T) {
	// Arrange: one materialized entry whose deadline is deterministically expired.
	cache := WithCache(&retentionRegistry{}, time.Hour)
	if _, fetchErr := cache.Plan(t.Context(), "idle", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	// Act: expiry alone changes freshness; there is no background eviction worker.
	expireRetentionEntries(cache, "")
	// Assert: idle cache retains the generation until the next cleanup trigger.
	entries, bytes := retainedExpiredPayload(cache)
	if entries != 1 || bytes < 64<<10 {
		t.Fatalf("expired entries=%d reachable bytes=%d", entries, bytes)
	}
}

func TestCacheNonpositiveTTLRemainsFresh(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Hour} {
		t.Run(ttl.String(), func(t *testing.T) {
			// Arrange: a nonexpiring entry, with an adversarial past deadline.
			base := &retentionRegistry{}
			cache := WithCache(base, ttl)
			if _, fetchErr := cache.Plan(t.Context(), "infinite", prompty.RegistryPlanInput{}); fetchErr != nil {
				t.Fatal(fetchErr)
			}
			expireRetentionEntries(cache, "")
			// Act: lookup still hits; TTL <= 0 controls freshness.
			if _, fetchErr := cache.Plan(t.Context(), "infinite", prompty.RegistryPlanInput{}); fetchErr != nil {
				t.Fatal(fetchErr)
			}
			// Assert.
			if got := base.calls.Load(); got != 1 {
				t.Fatalf("fetches=%d want=1", got)
			}
		})
	}
}

func TestCacheExpiredFetchErrorDoesNotFallback(t *testing.T) {
	// Arrange: expire a successful generation, then fail every fetch.
	base := &retentionRegistry{}
	cache := WithCache(base, time.Hour)
	if _, fetchErr := cache.Plan(t.Context(), "failed", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	expireRetentionEntries(cache, "")
	base.fail.Store(true)
	// Act.
	plan, err := cache.Plan(t.Context(), "failed", prompty.RegistryPlanInput{})
	// Assert: neither stale fallback nor failed fetch is stored.
	if err == nil || plan != nil {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.cache) != 0 {
		t.Fatalf("cache entries=%d want=0", len(cache.cache))
	}
}

func TestCacheCleanupPreservesHeldPlan(t *testing.T) {
	// Arrange: caller holds a generation selected before expiry.
	cache := WithCache(&retentionRegistry{}, time.Hour)
	plan, err := cache.Plan(t.Context(), "held", prompty.RegistryPlanInput{})
	if err != nil {
		t.Fatal(err)
	}
	expireRetentionEntries(cache, "")
	// Act: cleanup drops cache ownership while caller still owns its plan.
	if _, fetchErr := cache.Plan(t.Context(), "fresh", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	exec, err := plan.Execute(t.Context())
	// Assert: the held generation still renders its original content and identity.
	if err != nil {
		t.Fatal(err)
	}
	if got := exec.Messages[0].Content[0].(prompty.TextPart).Text; got != "held"+strings.Repeat("x", 64<<10) {
		t.Fatal("held plan content changed")
	}
	if plan.Template().Metadata.ID != "held" {
		t.Fatal("held plan identity changed")
	}
}

func TestCacheCleanupPreservesHeldSnapshot(t *testing.T) {
	// Arrange: capture immutable sources and remember their descriptor.
	fetcher := &mockFetcher{data: map[string][]byte{
		"held": []byte(
			`{"id":"held","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"old generation"}]}]}`,
		),
		"fresh": []byte(
			`{"id":"fresh","version":"2","messages":[{"role":"user","content":[{"type":"text","text":"fresh generation"}]}]}`,
		),
	}}
	base, err := New(fetcher, WithParser(manifest.NewJSONParser()))
	if err != nil {
		t.Fatal(err)
	}
	cache := WithCache(base, time.Hour)
	snapshot, err := cache.CaptureManifest(t.Context(), "held")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := snapshot.Descriptor()
	expireRetentionEntries(cache, "")
	fetcher.data["held"] = []byte(
		`{"id":"held","version":"2","messages":[{"role":"user","content":[{"type":"text","text":"new generation"}]}]}`,
	)
	// Act: drop cache ownership and load the replacement generation.
	if _, captureErr := cache.CaptureManifest(t.Context(), "fresh"); captureErr != nil {
		t.Fatal(captureErr)
	}
	replacement, err := cache.CaptureManifest(t.Context(), "held")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := snapshot.Plan(t.Context(), prompty.RegistryPlanInput{})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := plan.Execute(t.Context())
	// Assert: external snapshot ownership survives cleanup and refresh.
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Descriptor() != descriptor || replacement.Descriptor() == descriptor {
		t.Fatal("snapshot generations were not isolated")
	}
	if got := exec.Messages[0].Content[0].(prompty.TextPart).Text; got != "old generation" {
		t.Fatalf("held snapshot text=%q", got)
	}
}

func BenchmarkRegistryCacheHit(b *testing.B) {
	cache := WithCache(&retentionRegistry{}, time.Hour)
	if _, err := cache.Plan(b.Context(), "hit", prompty.RegistryPlanInput{}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := cache.Plan(b.Context(), "hit", prompty.RegistryPlanInput{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegistryCacheMiss(b *testing.B) {
	cache := WithCache(&retentionRegistry{}, time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cache.Evict("miss")
		if _, err := cache.Plan(b.Context(), "miss", prompty.RegistryPlanInput{}); err != nil {
			b.Fatal(err)
		}
	}
}

func assertRetentionIndex(t *testing.T, cache *CachedRegistry) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	seen := make(map[*cacheEntry]bool)
	var previous *cacheEntry
	for entry := cache.oldest; entry != nil; entry = entry.next {
		if seen[entry] {
			t.Fatal("FIFO cycle")
		}
		seen[entry] = true
		if entry.prev != previous || cache.cache[entry.id] != entry {
			t.Fatal("FIFO membership/backlink mismatch")
		}
		previous = entry
	}
	if previous != cache.newest || len(seen) != len(cache.cache) {
		t.Fatalf("FIFO nodes=%d stored entries=%d", len(seen), len(cache.cache))
	}
	if cache.maxEntries > 0 && len(cache.cache) > cache.maxEntries {
		t.Fatalf("stored=%d limit=%d", len(cache.cache), cache.maxEntries)
	}
}

func TestCacheLimitCardinalityAndIndexLifecycle(t *testing.T) {
	for _, limit := range []int{1, 16, 128} {
		for _, ttl := range []time.Duration{time.Hour, 0, -time.Hour} {
			t.Run(fmt.Sprintf("%d/%s", limit, ttl), func(t *testing.T) {
				checkCapacityIndexLifecycle(t, limit, ttl)
			})
		}
	}
}

func checkCapacityIndexLifecycle(t *testing.T, limit int, ttl time.Duration) {
	t.Helper()
	// Arrange: opt-in capacity with expiring or nonexpiring entries.
	cache := WithCacheLimit(&retentionRegistry{}, ttl, limit)
	// Act: repeatedly publish more IDs than fit in cache.
	for i := range limit*3 + 17 {
		if _, fetchErr := cache.Plan(t.Context(), strconv.Itoa(i), prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
		// Assert after every completed publication.
		assertRetentionIndex(t, cache)
	}
	cache.mu.RLock()
	if len(cache.cache) != limit {
		t.Errorf("entries=%d want=%d", len(cache.cache), limit)
	}
	cache.mu.RUnlock()
	// Act: EvictAll must release both map ownership and auxiliary node ownership.
	cache.EvictAll()
	// Assert.
	assertRetentionIndex(t, cache)
	if cache.oldest != nil || cache.newest != nil || len(cache.cache) != 0 {
		t.Fatal("EvictAll retained index or entries")
	}
}

func TestCacheConstructorInvalidArguments(t *testing.T) {
	for _, build := range []struct {
		name string
		fn   func()
	}{
		{"nil/base", func() { WithCache(nil, time.Hour) }},
		{"limit/nil-base", func() { WithCacheLimit(nil, time.Hour, 1) }},
		{"limit/zero", func() { WithCacheLimit(&retentionRegistry{}, time.Hour, 0) }},
		{"limit/negative", func() { WithCacheLimit(&retentionRegistry{}, time.Hour, -1) }},
	} {
		t.Run(build.name, func(t *testing.T) {
			// Arrange/Act: invalid constructor arguments fail explicitly.
			defer func() {
				if recover() == nil {
					t.Fatal("constructor did not panic")
				}
			}()
			build.fn()
		})
	}
}

func TestCacheUnlimitedPreservesMoreThan512FreshGenerations(t *testing.T) {
	for _, ttl := range []time.Duration{time.Hour, 0, -time.Hour} {
		t.Run(ttl.String(), func(t *testing.T) {
			// Arrange: legacy API has no cardinality cap.
			base := &retentionRegistry{}
			cache := WithCache(base, ttl)
			// Act: load >512 distinct IDs and then select the earliest again.
			for i := range 530 {
				if _, fetchErr := cache.Plan(
					t.Context(),
					strconv.Itoa(i),
					prompty.RegistryPlanInput{},
				); fetchErr != nil {
					t.Fatal(fetchErr)
				}
			}
			base.fail.Store(true)
			plan, err := cache.Plan(t.Context(), "0", prompty.RegistryPlanInput{})
			// Assert: all fresh generations remain stored and the oldest lookup still hits.
			if err != nil {
				t.Fatal(err)
			}
			if plan.Template().Metadata.ID != "0" || base.calls.Load() != 530 {
				t.Fatal("legacy cache evicted a fresh generation")
			}
			cache.mu.RLock()
			if len(cache.cache) != 530 {
				t.Errorf("entries=%d want=530", len(cache.cache))
			}
			cache.mu.RUnlock()
			assertRetentionIndex(t, cache)
		})
	}
}

func TestCacheLimitFIFOOrdering(t *testing.T) {
	// Arrange: publications select FIFO order; hits do not promote entries.
	cache := WithCacheLimit(&retentionRegistry{}, time.Hour, 2)
	for _, id := range []string{"a", "b", "a", "c"} {
		if _, fetchErr := cache.Plan(t.Context(), id, prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
	}
	// Assert: a hit on a did not protect it from capacity eviction.
	cache.mu.RLock()
	_, a := cache.cache["a"]
	b := cache.cache["b"]
	c := cache.cache["c"]
	cache.mu.RUnlock()
	if a || b == nil || c == nil {
		t.Fatal("hit changed FIFO publication order")
	}
	assertRetentionIndex(t, cache)
	// Act: b expires and refresh publication joins the tail; then d displaces c.
	cache.mu.Lock()
	cache.cache["b"].expiresAt = time.Now().Add(-time.Hour)
	cache.mu.Unlock()
	for _, id := range []string{"b", "d"} {
		if _, fetchErr := cache.Plan(t.Context(), id, prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
	}
	// Assert.
	cache.mu.RLock()
	_, cPresent := cache.cache["c"]
	_, bPresent := cache.cache["b"]
	_, dPresent := cache.cache["d"]
	cache.mu.RUnlock()
	if cPresent || !bPresent || !dPresent {
		t.Fatal("refresh did not append a new FIFO publication")
	}
	if b.prev != nil || b.next != nil || c.prev != nil || c.next != nil {
		t.Fatal("detached entries retain FIFO payload chain")
	}
	assertRetentionIndex(t, cache)
}

func TestCacheRefreshEvictChurnReleasesIndexNodes(t *testing.T) {
	// Arrange: retain old entries solely for inspecting their detached links.
	cache := WithCacheLimit(&retentionRegistry{}, time.Hour, 16)
	var detached []*cacheEntry
	// Act: repeatedly refresh and explicitly evict the same ID.
	for i := range 200 {
		if _, fetchErr := cache.Plan(t.Context(), "churn", prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
		cache.mu.RLock()
		detached = append(detached, cache.cache["churn"])
		cache.mu.RUnlock()
		if i%2 == 0 {
			expireRetentionEntries(cache, "")
		} else {
			cache.Evict("churn")
		}
		assertRetentionIndex(t, cache)
	}
	cache.EvictAll()
	// Assert: map/index storage tracks current entries without historical node tails.
	assertRetentionIndex(t, cache)
	for _, entry := range detached {
		if entry.prev != nil || entry.next != nil {
			t.Fatal("detached entry retains index nodes")
		}
	}
}

func TestCacheLimitFetchErrorPreservesFreshVictim(t *testing.T) {
	// Arrange: fill capacity before an unrelated failing fetch.
	base := &retentionRegistry{}
	cache := WithCacheLimit(base, time.Hour, 1)
	if _, fetchErr := cache.Plan(t.Context(), "fresh", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	base.fail.Store(true)
	// Act.
	if _, err := cache.Plan(t.Context(), "failed", prompty.RegistryPlanInput{}); err == nil {
		t.Fatal("expected fetch error")
	}
	// Assert: failed material never triggers capacity eviction.
	if _, fetchErr := cache.Plan(t.Context(), "fresh", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	assertRetentionIndex(t, cache)
}

type gatedRetentionRegistry struct {
	retentionRegistry

	gated   atomic.Bool
	started chan string
	release chan struct{}
}

func (r *gatedRetentionRegistry) Plan(
	ctx context.Context,
	id string,
	input prompty.RegistryPlanInput,
) (*prompty.RenderPlan, error) {
	if r.gated.Load() {
		r.started <- id
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-r.release:
		}
	}
	return r.retentionRegistry.Plan(ctx, id, input)
}

func TestCacheLimitConcurrentCompletions(t *testing.T) {
	// Arrange: many different fetches wait to publish into an already full cache.
	const limit = 16
	const fetches = 32
	base := &gatedRetentionRegistry{started: make(chan string, fetches), release: make(chan struct{})}
	cache := WithCacheLimit(base, time.Hour, limit)
	for i := range limit {
		if _, fetchErr := cache.Plan(
			t.Context(),
			fmt.Sprintf("initial%d", i),
			prompty.RegistryPlanInput{},
		); fetchErr != nil {
			t.Fatal(fetchErr)
		}
	}
	base.gated.Store(true)
	results := make(chan error, fetches)
	var wait sync.WaitGroup
	for i := range fetches {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			id := fmt.Sprintf("concurrent%d", i)
			plan, err := cache.Plan(t.Context(), id, prompty.RegistryPlanInput{})
			if err == nil && (plan == nil || plan.Template().Metadata.ID != id) {
				err = errors.New("fetch result identity mismatch")
			}
			cache.mu.RLock()
			if err == nil && len(cache.cache) > limit {
				err = fmt.Errorf("stored entries=%d limit=%d", len(cache.cache), limit)
			}
			cache.mu.RUnlock()
			results <- err
		}(i)
	}
	for range fetches {
		<-base.started
	}
	// Act: release all fetches simultaneously.
	close(base.release)
	wait.Wait()
	close(results)
	// Assert: every waiter receives its own material and every observed mutation is bounded.
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
	if got := base.calls.Load(); got != limit+fetches {
		t.Fatalf("fetches=%d want=%d", got, limit+fetches)
	}
	assertRetentionIndex(t, cache)
}

func TestCacheLimitCancellationDoesNotEvictFreshVictim(t *testing.T) {
	// Arrange: a fresh entry fills the cache while a different ID fetch is blocked.
	base := &gatedRetentionRegistry{started: make(chan string, 1), release: make(chan struct{})}
	cache := WithCacheLimit(base, time.Hour, 1)
	if _, fetchErr := cache.Plan(t.Context(), "fresh", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	base.gated.Store(true)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, err := cache.Plan(ctx, "cancelled", prompty.RegistryPlanInput{}); result <- err }()
	<-base.started
	cache.mu.RLock()
	flight := cache.inflight["cancelled"]
	cache.mu.RUnlock()
	// Act: cancel the sole waiter, then allow the detached fetch to finish.
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fetch err=%v", err)
	}
	close(base.release)
	<-flight.done
	// Assert: failed/cancelled fetch cannot consume capacity at the expense of fresh material.
	if _, fetchErr := cache.Plan(t.Context(), "fresh", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	assertRetentionIndex(t, cache)
}

func TestCacheExpirationBoundary(t *testing.T) {
	// Arrange: a positive TTL entry whose exact expiry is known.
	cache := WithCache(&retentionRegistry{}, time.Hour)
	if _, fetchErr := cache.Plan(t.Context(), "boundary", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry := cache.cache["boundary"]
	boundary := entry.expiresAt
	// Act/Assert: freshness is strict now < expiry; equality expires the generation.
	if !cache.cacheEntryValid(entry, boundary.Add(-time.Nanosecond)) || cache.cacheEntryValid(entry, boundary) ||
		cache.cacheEntryValid(entry, boundary.Add(time.Nanosecond)) {
		t.Fatal("expiration boundary changed")
	}
	cache.removeExpired(boundary.Add(-time.Nanosecond))
	if len(cache.cache) != 1 {
		t.Fatal("cleanup removed fresh entry")
	}
	cache.removeExpired(boundary)
	if len(cache.cache) != 0 || cache.oldest != nil || cache.newest != nil {
		t.Fatal("cleanup retained entry at exact expiry")
	}
	if entry.prev != nil || entry.next != nil {
		t.Fatal("expired node remained linked")
	}
}

func TestCacheMaintenanceDoesNotScanFreshIndex(t *testing.T) {
	// Arrange: white-box sentinel deliberately violates production deadline ordering.
	// The earliest publication stays fresh; only an interior deadline is poisoned.
	// Its removal would demonstrate a full cache scan rather than prefix-index work.
	for _, count := range []int{16, 128, 512, 4096} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			checkFreshIndexMaintenance(t, count)
		})
	}
}

func checkFreshIndexMaintenance(t *testing.T, count int) {
	t.Helper()
	cache := WithCacheLimit(&retentionRegistry{}, time.Hour, count)
	for i := range count {
		if _, fetchErr := cache.Plan(t.Context(), strconv.Itoa(i), prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
	}
	cache.mu.Lock()
	sentinel := cache.newest
	sentinel.expiresAt = time.Now().Add(-time.Hour)
	oldest := cache.oldest
	cache.mu.Unlock()
	// Act: hit maintenance followed by successful full-cache insertion.
	if _, fetchErr := cache.Plan(t.Context(), "0", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	cache.mu.RLock()
	hitSentinelPresent := cache.cache[sentinel.id] == sentinel
	cache.mu.RUnlock()
	if !hitSentinelPresent {
		t.Fatal("hit maintenance scanned interior deadlines")
	}
	if _, fetchErr := cache.Plan(t.Context(), "new", prompty.RegistryPlanInput{}); fetchErr != nil {
		t.Fatal(fetchErr)
	}
	// Assert: unvisited interior sentinel stays stored and FIFO evicts exactly its head.
	cache.mu.RLock()
	sentinelPresent := cache.cache[sentinel.id] == sentinel
	oldestPresent := cache.cache[oldest.id] == oldest
	cache.mu.RUnlock()
	if !sentinelPresent || oldestPresent {
		t.Fatal("maintenance scanned interior deadlines or capacity victim was not FIFO head")
	}
	assertRetentionIndex(t, cache)
}

func TestCacheCapacityPreservesHeldSnapshot(t *testing.T) {
	// Arrange: nonexpiring snapshots in a capacity-one registry.
	fetcher := &mockFetcher{data: map[string][]byte{
		"old": []byte(
			`{"id":"old","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"old generation"}]}]}`,
		),
		"new": []byte(
			`{"id":"new","version":"2","messages":[{"role":"user","content":[{"type":"text","text":"new generation"}]}]}`,
		),
	}}
	base, err := New(fetcher, WithParser(manifest.NewJSONParser()))
	if err != nil {
		t.Fatal(err)
	}
	cache := WithCacheLimit(base, 0, 1)
	held, err := cache.CaptureManifest(t.Context(), "old")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := held.Descriptor()
	// Act: a successful new publication evicts the old generation from cache only.
	if _, captureErr := cache.CaptureManifest(t.Context(), "new"); captureErr != nil {
		t.Fatal(captureErr)
	}
	plan, err := held.Plan(t.Context(), prompty.RegistryPlanInput{})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := plan.Execute(t.Context())
	// Assert: caller-owned snapshots and descriptor remain valid after eviction.
	if err != nil {
		t.Fatal(err)
	}
	if held.Descriptor() != descriptor || exec.Messages[0].Content[0].(prompty.TextPart).Text != "old generation" {
		t.Fatal("capacity eviction damaged caller-owned snapshot")
	}
	assertRetentionIndex(t, cache)
}

func TestCacheCleanupVisitsExpiredPrefix(t *testing.T) {
	// Arrange: a small expired prefix followed by thousands of fresh publications.
	const count = 4096
	const expired = 7
	cache := WithCache(&retentionRegistry{}, time.Hour)
	for i := range count {
		if _, fetchErr := cache.Plan(t.Context(), strconv.Itoa(i), prompty.RegistryPlanInput{}); fetchErr != nil {
			t.Fatal(fetchErr)
		}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := time.Now()
	removed := make([]*cacheEntry, 0, expired)
	for entry := cache.oldest; len(removed) < expired; entry = entry.next {
		entry.expiresAt = now.Add(-time.Hour)
		removed = append(removed, entry)
	}
	freshHead := removed[len(removed)-1].next
	// An out-of-order tail expiry is white-box instrumentation for unwanted scans.
	tail := cache.newest
	tail.expiresAt = now.Add(-time.Hour)
	// Act: cleanup should visit exactly the expired prefix and stop at fresh head.
	cache.removeExpired(now)
	// Assert: prefix work progresses fully without touching the fresh suffix/interior sentinel.
	if len(cache.cache) != count-expired || cache.oldest != freshHead || cache.cache[tail.id] != tail {
		t.Fatal("cleanup did not process exactly expired prefix")
	}
	for _, entry := range removed {
		if cache.cache[entry.id] != nil || entry.prev != nil || entry.next != nil {
			t.Fatal("removed prefix entry retains cache/index ownership")
		}
	}
}

func TestCacheLimitDetachedEviction(t *testing.T) {
	t.Parallel()
	for _, all := range []bool{false, true} {
		for _, oldFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("all=%t/oldFirst=%t", all, oldFirst), func(t *testing.T) {
				checkCacheEviction(t, all, oldFirst, true)
			})
		}
	}
}

func TestCacheLimitWaiterCancellationIsolation(t *testing.T) {
	// Arrange: the first request and a joined waiter share one gated fetch.
	base := &gatedRetentionRegistry{started: make(chan string, 1), release: make(chan struct{})}
	base.gated.Store(true)
	cache := WithCacheLimit(base, time.Hour, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() { _, err := cache.Plan(ctx, "shared", prompty.RegistryPlanInput{}); first <- err }()
	<-base.started
	joinCtx := &admittedWaiterContext{Context: t.Context(), admitted: make(chan struct{})}
	go func() {
		plan, err := cache.Plan(joinCtx, "shared", prompty.RegistryPlanInput{})
		if err == nil && plan.Template().Metadata.ID != "shared" {
			err = errors.New("wrong shared result")
		}
		second <- err
	}()
	<-joinCtx.admitted
	// Act: cancel only the first waiter, then complete the shared fetch.
	cancel()
	firstErr := <-first
	close(base.release)
	secondErr := <-second
	// Assert: waiter cancellation is isolated and the successful generation is stored once.
	if !errors.Is(firstErr, context.Canceled) || secondErr != nil {
		t.Fatalf("first=%v second=%v", firstErr, secondErr)
	}
	if base.calls.Load() != 1 {
		t.Fatalf("fetches=%d want=1", base.calls.Load())
	}
	assertRetentionIndex(t, cache)
}
