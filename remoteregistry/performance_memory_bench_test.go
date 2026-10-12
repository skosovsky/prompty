package remoteregistry

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/prompty"
)

// cacheBenchmarkRegistry selects already constructed immutable templates. Remote
// I/O and template parsing are deliberately separate from cache bookkeeping.
type cacheBenchmarkRegistry struct {
	templates map[string]*prompty.ChatPromptTemplate
}

func (r *cacheBenchmarkRegistry) Plan(
	_ context.Context,
	id string,
	input prompty.RegistryPlanInput,
) (*prompty.RenderPlan, error) {
	return prompty.NewRenderPlanFromPlanInput(r.templates[id], input)
}

func cacheBenchmarkFixture(b *testing.B, count int) (*cacheBenchmarkRegistry, []string) {
	b.Helper()
	base := &cacheBenchmarkRegistry{templates: make(map[string]*prompty.ChatPromptTemplate, count)}
	ids := make([]string, count)
	for i := range count {
		ids[i] = fmt.Sprintf("prompt/%d", i)
		tpl, err := prompty.NewChatPromptTemplate(
			[]prompty.MessageTemplate{{Role: prompty.RoleUser, Content: prompty.TextContent("benchmark")}},
			prompty.WithMetadata(prompty.PromptMetadata{ID: ids[i], Version: "1"}),
		)
		if err != nil {
			b.Fatal(err)
		}
		base.templates[ids[i]] = tpl
	}
	return base, ids
}

func warmCacheBenchmark(b *testing.B, cache *CachedRegistry, ids []string) {
	b.Helper()
	for _, id := range ids {
		if _, err := cache.loadMaterial(b.Context(), id); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCacheFreshHit isolates loadMaterial and all lookup/maintenance work.
// PublicPlan adds the normal public result construction to that same lookup.
func BenchmarkCacheFreshHit(b *testing.B) {
	for _, count := range []int{16, 128, 512, 4096} {
		for _, public := range []bool{false, true} {
			name := "Material"
			if public {
				name = "PublicPlan"
			}
			b.Run(fmt.Sprintf("%s/N%d", name, count), func(b *testing.B) {
				benchmarkFreshCacheHit(b, count, public)
			})
		}
	}
}

func benchmarkFreshCacheHit(b *testing.B, count int, public bool) {
	b.Helper()
	base, ids := cacheBenchmarkFixture(b, count)
	cache := WithCache(base, time.Hour)
	warmCacheBenchmark(b, cache, ids)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if public {
			if _, err := cache.Plan(b.Context(), ids[0], prompty.RegistryPlanInput{}); err != nil {
				b.Fatal(err)
			}
		} else if _, err := cache.loadMaterial(b.Context(), ids[0]); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCacheIdleBacklogCleanup times the first lookup after an idle period.
// Each iteration creates and expires a new lifecycle fixture outside the timer;
// the complete lookup and cleanup of its N expired entries remain inside. Use
// -benchtime=20x (or another fixed iteration count) to bound untimed fixture work.
func BenchmarkCacheIdleBacklogCleanup(b *testing.B) {
	for _, count := range []int{16, 128, 512, 4096} {
		b.Run(fmt.Sprintf("N%d", count), func(b *testing.B) {
			base, ids := cacheBenchmarkFixture(b, count+1)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				b.StopTimer()
				cache := WithCache(base, time.Hour)
				warmCacheBenchmark(b, cache, ids)
				cache.mu.Lock()
				deadline := time.Now().Add(-time.Hour)
				for _, id := range ids[:count] {
					cache.cache[id].expiresAt = deadline
				}
				cache.mu.Unlock()
				b.StartTimer()
				if _, err := cache.loadMaterial(b.Context(), ids[count]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCacheRefresh(b *testing.B) {
	for _, count := range []int{16, 128, 512, 4096} {
		b.Run(fmt.Sprintf("N%d", count), func(b *testing.B) {
			base, ids := cacheBenchmarkFixture(b, count)
			cache := WithCache(base, time.Hour)
			warmCacheBenchmark(b, cache, ids)
			i := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				id := ids[i%count]
				i++
				// Expire the oldest publication, maintaining the production FIFO
				// deadline invariant. Invalidation itself is included in timing.
				cache.mu.Lock()
				cache.cache[id].expiresAt = time.Now().Add(-time.Hour)
				cache.mu.Unlock()
				if _, err := cache.loadMaterial(b.Context(), id); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCacheCapacityEviction cycles N+1 IDs through a capacity-N cache.
// Incoming-ID Evict is timed in both trees so the baseline WithCache counterpart
// also always fetches. Baseline has no capacity API: a disposable benchmark shim
// maps WithCacheLimit to unbounded WithCache, and is explicitly labelled in logs.
// Candidate still performs its real capacity admission and victim eviction.
func BenchmarkCacheCapacityEviction(b *testing.B) {
	for _, count := range []int{16, 128, 512, 4096} {
		b.Run(fmt.Sprintf("N%d", count), func(b *testing.B) {
			base, ids := cacheBenchmarkFixture(b, count+1)
			cache := WithCacheLimit(base, time.Hour, count)
			warmCacheBenchmark(b, cache, ids[:count])
			i := count
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				id := ids[i%(count+1)]
				i++
				cache.Evict(id)
				if _, err := cache.loadMaterial(b.Context(), id); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCacheConcurrentHit(b *testing.B) {
	for _, count := range []int{16, 128, 512, 4096} {
		b.Run(fmt.Sprintf("N%d", count), func(b *testing.B) {
			base, ids := cacheBenchmarkFixture(b, count)
			cache := WithCache(base, time.Hour)
			warmCacheBenchmark(b, cache, ids)
			var worker atomic.Uint64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := int(worker.Add(1) - 1)
				for pb.Next() {
					if _, err := cache.loadMaterial(b.Context(), ids[i%count]); err != nil {
						b.Error(err)
						return
					}
					i++
				}
			})
		})
	}
}
