# Task40 — independent completeness audit, round 2

**Decision: REJECT — 94.12% (16/17 complete).** Partial requirements are not counted. This report records audit evidence, not a mathematical guarantee of defect absence.

## Snapshot and scope

Only `/tmp/prompty-task40-snapshot-02` was audited, baseline `c8141c5`. SHA-256 identity of the manifest is `58250bf7ef16b94095539574706d1a87f322c629a533fbe52d035e3386711c53`. All **701/701** hashes pass; integrity log `/tmp/task40-integrity-r2.log`.

The full task40 specification, README rules, current contracts, source documentation, closeout and code were reconsidered. Scope and denominator remain **17**: eight numbered requirements, eight acceptance bullets, mandatory closeout. No mandatory scope was waived. Frozen and working implementation were not modified; probes are exclusively in separate copy `/tmp/prompty-task40-audit-r2`. No current-round peer report was read.

## Full requirement matrix

| Requirement | Code / test evidence | Status |
| --- | --- | --- |
| C1. Unified verified materialization, captured identity/sources, no independent catalog | `manifest_snapshot.go`: private ManifestSnapshot, SourceBundle, one-method ManifestSnapshotRegistry, VerifiedManifestPlan; checkpoint port embeds capture; defensive byte/config copies and canonical identity fixtures | Complete |
| C2. Verification and materialization use one captured byte generation; no verify/read TOCTOU | VerifiedManifestPlan compares one captured descriptor then plans that snapshot; typed/raw binders use it; changing-source/unrelated-Plan tests; r1 parsed alias independently rerun and now sealed; no posthoc registry lookup in compiled | Complete |
| C3. Deterministic main/import/layer/partial/config closure; conditional imports/cycles; traversal stability | `manifest/snapshot.go` recursively captures all declared imports, including inactive branches; `manifest/compose.go` resolves declared layer import_refs; canonical JSON sorts map keys; partial/config bytes participate; transitive/inactive import checkpoint tests, cyclic/empty import tests, partial-only change and reordered SourceBundle fixture | Complete |
| C4. Digest boundary excludes Go functions/services/time/random/model; host runtime dependencies | Public source-snapshot document, README and contracts distinguish integrity from runtime behavior and durable run state; no runtime code/service serialization or deterministic-render promise | Complete |
| C5. Cached identity with immutable parsed/compiled material; restore under own identity or mismatch | File and TTL entries retain snapshot generation; `parseCapturedManifest` deep-clones output; Plan clones private raw closure; plan/compiled preserve descriptor; old-cache/new-descriptor mismatch and retained-source port fixtures. Independent original retained-parser probe returns OLD/OLD and unchanged descriptor. | Complete |
| C6. Contract across file/embed/remote/cache/Git, typed recipes/raw BindRuntime/compiled; small custom port; incomplete custom fails verified claims | All required capture/planning ports exist and are covered; incomplete cached custom registry capture errors; minimal capture-only recipe/raw fixtures and uncaptured compiled rejection pass. **Remote capture still passes unprotected fetcher bytes to the parser in a preliminary compose peek, bypassing the current readonly-source/mutation rejection contract (F1).** | **Partial** |
| C7. Mutable bundles do not claim atomic revision; immutable binding fails unsupported/unavailable; no storage/VCS service | WithRevision/RevisionFetcher explicit optional binding; Git full commit tree blob reading rejects moving refs and unavailable commits; moving checkout fixture; docs explain shallow availability and bundle-vs-revision boundary | Complete |
| C8. Breaking descriptors/wire/codegen; old incomplete formats reject; remove posthoc digest paths | Version-prefixed descriptor validation and executable JSON Schema parity; compiled format v3/StatePreserve rejects legacy formats; constructors consume captured Plan; generator captured index binds schema/digest from same bytes and now rejects logical/path alias collisions both orders; generated consumers/goldens pass | Complete |
| A1. OLD-cache/NEW-descriptor gives corresponding NEW or mismatch | File cache mismatch/Reload regression; compiled OLD keeps OLD identity | Complete |
| A2. Deterministic source change between protocol stages; no sleep-based race | Changing remote read fixture, custom separate verifier/Plan fixture, codegen captured-index source replacement fixture; no second lookup in compiled | Complete |
| A3. Imported/partial change affects identity; stable unchanged-bundle traversal | Transitive/inactive import tamper checkpoint fixtures; partial-only change; canonical map ordering/SourceBundle tests | Complete |
| A4. File/embed/remote cached/uncached/Git/custom contract and disappearance | File partial/source deletion snapshot fixture; embed FS construction isolation; remote cached/uncached reads and TTL generation fixtures; Git immutable revision fixture; retained minimal custom port restore. Shared materializer needs no source read after capture. | Complete |
| A5. Concurrent Reload/Plan/Execute isolation/race and caller plan-input ownership | 12×12 concurrent verified file Plan/compiled Execute; remote concurrent dedupe/cancellation/cache tests; template/history/bound-map clones; ComposeValues now recursively clones maps and Lookup output, nested ownership fixture passes. Remote source-reader ownership gap is C6/F1, not a caller RegistryPlanInput. | Complete |
| A6. Typed checkpoint, raw BindRuntime and compiled share digest semantics | capture-only typed/raw fixture, compose compiled/checkpoint identity parity, all source descriptor constructors validate same versioned digest | Complete |
| A7. Environment/conditional composition/runtime values/partial resolution fixtures | File/embed/remote strict environment and conditional missing/active/runtime fixtures use capture Plan; conservative inactive import closure; file/embed partial patterns; environment/pattern/mode/version/revision config identity; deep ComposeValues copy fixes preserve runtime branches | Complete |
| A8. Documentation separates integrity, deterministic render and durable agent state | README links public source contract; full guarantee/restore/migration sections; arbitrary parser/materializer code remains trusted host boundary, without deterministic rendering or durable run claims | Complete |
| M1. Mandatory breaking closeout/migration/checks/limitations | Current closeout lists changed ports, removed legacy constructors/read paths, regeneration and explicit retained restore migration, source trust/storage/runtime limits. Final logs show lint all 12 modules, test/race all modules plus generated consumers. No release/tag/publication/agent-loop/storage expansion. | Complete |

## F1 — remote preliminary parser call bypasses source protection

Location: `remoteregistry/registry.go:69–73`, `manifest/compose.go:105–113`; protected capture parsing is in `manifest/snapshot.go:parseCapturedManifest`.

`Registry.CaptureManifest` uses the registry as its ManifestBytesReader. Before `CaptureSources` can copy the fetched bytes or apply `parseCapturedManifest`, `ReadManifestBytes` calls `PeekComposeFieldsE(data, r.parser)` on the original fetched slice. Peek forwards the slice directly to `Unmarshal`, and even a peek error is ignored by ReadManifestBytes. The new private-copy/mutation-rejection guard therefore does not protect this first parser call. A parser mutation here changes source bytes before the protected second call and can mutate the fetcher's own retained data.

Independent deterministic probe: `/tmp/prompty-task40-audit-r2/manifest/task40_remote_audit_probe_test.go`; `/tmp/task40-completeness-probes-r2.log`.

Arrange: Fetcher owns and returns a valid OLD JSON slice. Parser delegates correctly to the standard JSON parser and changes `OLD` to `NEW` only on its first call. Act: call public remote Registry.CaptureManifest, then Plan/Execute. Assert: capture should reject the synchronous parser input mutation, and fetched caller-owned bytes should remain unchanged. Actual:

```text
capture error=<nil>; parser calls=2; original source still OLD=false
captured NEW=true; execution=NEW
remote capture did not reject parser input mutation
```

Command in the separate copy: `GOCACHE=/tmp/prompty-go-cache go test -race ./manifest -run 'TestAudit' -v`; exit **1**. The original r1 retained-output test passes in the same run.

This is not an attempt to attest arbitrary parser code or sandbox source storage. The current public contract explicitly provides readonly parser input, private copies and synchronous mutation rejection. The shared capture helper enforces those promises, but a built-in remote pre-read path invokes the callback outside that guard. F1 is assigned to C6 rather than inflating the percentage penalty across every source-integrity row. The resulting NEW bundle and NEW execution are internally consistent; the violation is the capture contract's pre-guard mutation and source ownership boundary.

Expected reacceptance evidence: remote capture must not permit its preliminary metadata inspection to mutate source storage or bypass capture parser mutation handling. Exercise cached and uncached remote capture (including revision-bound reader, which uses the same code), in addition to retaining all existing closure/cache/recipe/compiled fixtures. No unrelated filesystem security framework or generic DTO/storage facility is needed.

## Independent and supplied checks

- Integrity 701/701 pass.
- Separate-copy root, codegen, Git and generated consumers: `GOCACHE=/tmp/prompty-go-cache go test -race ./... ./cmd/prompty-gen/... ./remoteregistry/git/... ./cmd/prompty-gen/testdata/gen_out`, exit **0**. Log `/tmp/task40-completeness-independent-r2.log`.
- Original independent retained-output r1 probe rerun on r2: pass, source OLD/output OLD/descriptor unchanged.
- New remote input-mutation probe: fails with no capture error and mutated source, as above.
- Author final full logs `/tmp/task40-lint-r2c.log`, `/tmp/task40-test-r2.log` inspected: all 12 modules, zero lint issues, race and explicit generated consumers pass; checks-r2 records final manifest ownership race after last assertion edit. These green checks do not cover F1.

Other round2 changes reviewed independently: `parseCapturedManifest` private input and deep output clone, embed preliminary input copy, readonly/output ownership godoc, recursive ComposeValues/map/Lookup clones, codegen alias collision checks in both traversal orders. BYOT, clear break, small ports and host responsibility boundaries remain intact. There is no current evidence of another incomplete mandatory requirement, but 100% acceptance is unavailable while C6 is partial.
