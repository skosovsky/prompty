# Frozen acceptance requirements

Each numbered item is one mandatory PASS; denominator 34 (expanded before capacity implementation to reflect the user's explicit task clarification on 2026-10-12). Partial/NOT RUN/FAIL is not PASS. Final reviews target identical implementation tree fingerprint and HEAD SHA. No threshold changes.

1. Contracts documented before implementation: immutable metadata, validation ordering, agreed retention API/TTL/lifecycle.
2. Validation 1MiB and 8MiB <=16384 B/op; difference <=4096; allocations independent of payload size.
3. Value/pointer/typed-nil/unknown/malformed content retains baseline errors and ordering.
4. Mixed/nested media and nested policies/controls retain paths and validation.
5. Validation does not mutate caller content or policies.
6. Metadata 1024 keys <=20 allocs/op and <=16856 B/op (35% of 48160).
7. Zero-key warm median <=110% of same-machine measured baseline; >=5 samples, repeat noise if necessary.
8. Canonical normalization, constructor validation and exact json.Number preserved.
9. Adversarial isolation for constructor input/public Messages/CloneTemplate/two rendered outputs.
10. Serialization happens only at construction, never each Plan/Execute; cold and warm benchmarks separate.
11. Expired 16/128/512 IDs unreachable through cache after cleanup trigger; deterministic entries/payload assertions.
12. Opt-in cache bounded by documented caller-supplied limit, with correct prompt/version; WithCache remains unlimited.
13. Idle behavior and TTL<=0 tested against agreed contract, no background workers.
14. Cache hit/refresh/error no stale fallback tested.
15. Singleflight/cancellation isolation/all waiters cancel remain correct.
16. Evict/EvictAll detached in-flight generations and already-held snapshots remain valid, race covered.
17. Permanent AAA regression tests; validation/cold/warm/cache hit/miss benchmarks separate.
18. Baseline/candidate >=5 samples same Go/CPU/GOMAXPROCS; raw logs and comparison saved.
19. Full root tests + race; adapter/registry tests; all downstream repository modules build/tests with unchanged pins, scope restricted to prompty.
20. Independent reviewer A 100% APPROVE and B APPROVE with zero known unfixed findings on same final tree; changes require both rechecks. Reports record SHA/tree, commands, limitations, findings and verdict.

21. WithCacheLimit exact signature, invalid limit panic and nil-base compatibility documented before code and tested.
22. Unlimited WithCache retains >512 fresh IDs, including captured generations at TTL<=0.
23. Opt-in maxEntries 1/16/128 at TTL>0, ==0 and <0; admissions always <=bound.
24. FIFO/LRU policy chosen and justified before code; hit/refresh/replacement and successful publication ordering tested.
25. No fresh-cache full scan at lookup/fetch and no linear capacity victim search; deterministic work tests.
26. Expiration boundary and idle-to-lookup cleanup; payload unreachable through all auxiliary nodes.
27. Same-ID refresh/Evict churn does not grow stale index nodes; EvictAll clears indexes immediately.
28. Concurrent different-ID fetch completion at capacity maintains atomic bound and correct returned IDs/versions.
29. Error/canceled fetch does not evict unrelated fresh entry; detached older fetch cannot republish after newer generation.
30. Cardinality scope explicitly excludes caller-held outputs/in-flight/byte budgets; bounded TTL<=0 refetch behavior documented.
31. Paired fresh-hit benchmarks at16/128/512/4096, >=5 baseline/candidate samples.
32. Paired idle-backlog cleanup/refresh/capacity eviction/concurrent benchmarks at16/128/512/4096, >=5 samples, all measured maintenance inside timers.
33. Median hit regressions >10% investigated and documented with justified decision, without concealing measured work.
34. User-updated specification mapped fully to fixed checklist, including new API and index lifecycle; no old arbitrary512 limit or fresh full scan remains.
