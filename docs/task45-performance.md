# Task45 measured performance decisions

Local evidence: Apple M1 Max, darwin/arm64, Go 1.27.1. Commands use `go test -run '^$' -bench '^BenchmarkTask45' -benchtime=1x -benchmem . ./adapter/anthropic`. These are single-iteration comparisons, not cross-platform latency promises. Benchmarks retain validity checks and use no provider IO.

| Path | Before | After | Decision |
| --- | --- | --- | --- |
| Structured JSON, ~1 MiB item | 215 ms; 1.08 GB; 1,047,624 allocations | 10.7 ms; 7.35 MB; 66 allocations | Fixed 1 KiB diagnostic ring; same suffix and parsing contract |
| Structured JSON, ~10 MiB item | 1.44 s; 10.83 GB; 10,484,812 allocations | 110 ms; 98.58 MB; 62 allocations | Remove per-byte suffix copying; whole document remains retained |
| Anthropic, 100 × 1 KiB deltas | 13.8 ms; 17.5 MB | 218 µs; 0.79 MB | Private builder, one source materialization at close |
| Anthropic, 1000 × 1 KiB deltas | 1.27 s; 1.70 GB | 2.83 ms; 7.95 MB | Preserve delta/citation delivery and final history fidelity |
| 128 individual part finalizations | 0.30 ms; 0.75 MB | Same algorithm | Retain atomic association/state fold; measure before adding mutable indexes |
| 1024 individual part finalizations | 17.4 ms; 46.39 MB | Same algorithm | Cost scales with retained parts × finalizations; host bounds part counts. This is not linear-time admission |

Map projection measured separately with `-bench '^BenchmarkTask45BoundProjection' -benchtime=200ms -count=3`. Warm pooled 16-key projection is ~390–553 ns with 0 steady allocations versus ~1.04–1.44 µs and 1192 B/3 allocations without pooling. At 1024 keys pooling still removes steady per-call allocation. Retain the pool, but only retain projections with at most 1024 entries. Large 8192-key projections allocate ~656 KB per call after the change rather than retaining large buckets in the pool; this deliberately trades large-request reuse for bounded retained entry count. Entry count is not a heap-byte guarantee, and sync.Pool is GC-reclaimable.

Sources: `task45_performance_test.go`, `adapter/anthropic/task45_performance_test.go`; bounded-suffix regression in `task45_api_regression_test.go` and actual native Anthropic SDK fixtures. Logs: `/tmp/prompty-task45-stage6-performance-before.log`, `/tmp/prompty-task45-stage6-performance-after.log`, `/tmp/prompty-task45-stage6-map-pool.log`, `/tmp/prompty-task45-stage6-map-pool-after.log`.

Ownership paths measured separately with `-benchtime=200ms -count=3`:

| Path | Measured evidence | Decision |
| --- | --- | --- |
| Plan With chain, 1024 nested defaults, one With | Before: 0.58–0.60 ms; ~550.9 KB;7191 allocs. After: 0.96–1.20 µs;856 B;13 allocs | Share the constructor-owned private read-only template; external Template() stays defensive |
| Same plan, eight With calls | Before: 4.45–4.52 ms;~4.41 MB;57528 allocs. After: 7.29–10.75 µs;~6.85 KB;104 allocs | Keep mutable override copies; no generic COW machinery |
| One 1 MiB media Finish frame, full stream lifecycle without frame observer | 0.65–0.66 ms;~6.30 MB;60 allocs | Retain source/consumer/accumulated/status ownership boundaries |
| Same full lifecycle with frame observer | 0.77–0.92 ms;~7.35 MB;65 allocs | Additional observer copy remains isolated; these figures include final result/status copies, not only frame delivery |
| Remove 16 old turns, each 1 KiB user plus short `known answer` assistant, fixed message estimator | 0.40–0.45 ms;~647 KB;5482 allocs | Full candidate validation and non-additive estimation retained |
| Remove 128 old turns under the same estimator | 23.1–25.7 ms;~35.7 MB;~287650 allocs | Document group × execution cost; caller bounds history. No subtraction without an additive contract |

Logs: `/tmp/prompty-task45-stage6-ownership-bench-warm.log`, `/tmp/prompty-task45-stage6-plan-clone-after.log`. Benchmarks are local throughput samples and do not establish caller-estimator runtime bounds or cross-platform guarantees. Root race and generated consumer pass after template sharing, preserving the existing constructor/source/Template()/sibling ownership regressions.

File capture measured with eight independent simple YAML IDs in one concurrent
batch, `-benchtime=200ms -count=3`: warm cached batch 5.60–5.69 µs/~624 B/10 allocs;
cold batch after Reload 0.428–0.439 ms/~129.5 KB/1224 allocs. Figures include worker
and channel overhead; they do not isolate lock waiting or prove a benefit from
per-ID scheduling. Keep the current immutable cache and document serialization.
Embedded construction eagerly captures available selected IDs; this file benchmark
does not claim embedded construction timing. Evidence:
`fileregistry/task45_performance_test.go`,
`/tmp/prompty-task45-stage6-catalog-bench.log`.

Current committed-fixture candidate: eight `*_gen.go` files,71751bytes,1937lines,
32exported types and142exported functions/methods (Go AST declarations, not a
regex estimate). The two primary type emitters `types_manifest.go`/`types_shared.go`
contain30137/21215bytes and1002/651lines respectively; this is a scoped maintenance
measure, not the size of the whole generator. Keep statically typed/NoLate public
variants and shared emitters/core admission; no reflection/runtime framework is
introduced to compress emitted APIs. Regeneration remains deterministic and the
explicit generated consumer compiles. Evidence:
`/tmp/prompty-task45-stage6-codegen-size.log`, the two emitters and fixture files.

Typed tool admission for one object with a1KiB string measured with
`-benchtime=200ms -count=3`: direct Invoke (schema validation + validation decode +
handler decode) 14.6–15.3µs/~22.5KB/59allocs. Additional whole-batch registry
preflight followed by the same Invoke: 26.2–26.4µs/~38.2KB/103allocs. This measures
the typed helper stages, not all ExecuteWithTypedTools orchestration/profile work.
Keep independent invocation validation and preflight; no prepared-value lifetime
or trusted-validation token is introduced without a corresponding public contract.
Evidence: task45_tool_performance_test.go and
`/tmp/prompty-task45-stage6-tool-bench.log`, including the runnable caller-port example.

Existing reference-origin diagnostics measured with `-benchtime=1x`: local failing
1000-edge graph 64.2 ms/~45.7 MB;10000-edge graph 2.07s/~449.8 MB. Valid baseline
1000/10000 graphs took 12.7/543ms and ~9.05/87.8MB. These single-iteration figures
show admission cost, not a proposed universal complexity bound. Keep engine-owned
resource provenance and failure semantics; host bounds schema graphs before compile.
Evidence: BenchmarkDiagnosticReferenceOrigins and
`/tmp/prompty-task45-stage6-reference-bench.log` (PASS, positive and negative controls).
