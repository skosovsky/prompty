# Task40 — independent completeness audit, round 3

**Decision: ACCEPT — 100% (17/17 fully complete).** No confirmed incomplete mandatory requirement or new blocking finding was identified. This is an audit conclusion, not a mathematical guarantee of defect absence.

## Identity, independence and denominator

Audited `/tmp/prompty-task40-snapshot-03`, baseline `c8141c5`. Manifest SHA-256 identity: `c78e268c2e185fc856abc3f6395349d6e0a17ca83a7f6deaaac1202e553ad01e`. Every **704/704** file hash passed both before and after audit: `/tmp/task40-integrity-r3-before.log`, `/tmp/task40-integrity-r3-after.log`.

Full scope: task40 specification and all acceptance text, `.cursor/task/README.md`, current contracts, public source-snapshot documentation, closeout, implementation and fixtures. The denominator remains **17**: eight numbered requirements, eight acceptance bullets, one mandatory closeout. README constraints apply across this matrix; none were waived or replaced by narrower checklist items. Partial would count as incomplete.

No implementation participation, no frozen/working source edits, no current peer report reads. Probes and independent tests used separate copy `/tmp/prompty-task40-audit-r3`, with `/tmp/prompty-go-cache`. Only this report is written to the working repository.

## Full requirement matrix

| Requirement | Code / test evidence | Status |
| --- | --- | --- |
| C1. Unified verified materialization, captured identity and needed source, no independent manifest catalog | `manifest_snapshot.go` private ManifestSnapshot and SourceBundle; one-method ManifestSnapshotRegistry; VerifiedManifestPlan and existing checkpoint/registry abstractions. Sources/config sealed and getter cloned; identity defensive-copy tests. | Complete |
| C2. Verify and materialize exactly one captured byte generation; no verify/read TOCTOU | VerifiedManifestPlan compares one captured snapshot then invokes its Plan. Typed recipe Plan/Execute and raw BindRuntime use this path. Remote raw ReadManifestBytes now returns fetched bytes without invoking parser; capture privately copies input, checks mutation and seals output. Independent r1/r2 probes both pass. Compiled performs no registry lookup after Plan. | Complete |
| C3. Deterministic main/import/layer/partial/config closure, inactive conditional imports and cycles, traversal independence | `manifest/snapshot.go` visits all declared imports, including inactive branches, with stack cycle detection; layers resolve import refs against declared imports in compose.go. Captured partial bytes and declarative config participate in canonical JSON identity. Transitive/inactive import tamper, cycle/empty import, partial-only identity change, stable map ordering/config fixtures pass. | Complete |
| C4. Digest guarantee excludes Go functions/services/time/random/model and leaves runtime dependencies to host | Public source-snapshot document, README link, contracts/closeout explicitly distinguish source integrity from deterministic rendering and durable agent state; no serialized runtime Go code or deterministic-function promise. | Complete |
| C5. Cache binds identity to immutable parsed/compiled material; old snapshots restore only under own identity/policy or mismatch | File cache and TTL cache hold whole ManifestSnapshot generations. parseCapturedManifest deep-clones parser output before cached closure; Plan clones private parsed data; plan clone/late binding preserve descriptor; compiled retains that descriptor. OLD cache/NEW descriptor mismatch, explicit Reload, retained snapshot restore, parser output ownership fixtures pass. Independent retained parser probe returns OLD with unchanged OLD sources/identity. | Complete |
| C6. Contract applies to file/embed/remote/cache/Git, typed/raw/compiled; minimal custom port, incomplete custom cannot claim verified | Built-in capture paths share CaptureSources; embed construction copies FS bytes; remote capture has no preliminary unsafe peek; public PeekComposeFieldsE uses protected parse helper. TTL capture errors without base capture port. Recipe/raw accept minimal capture-only registry; compiled rejects uncaptured Plan. File/embed/cached+uncached remote/Git/custom public fixtures and independent remote mutation probe pass. | Complete |
| C7. Mutable multi-read bundle does not claim atomic revision; explicit immutable revision binding fails unsupported/unavailable; no VCS/storage service | WithRevision/RevisionFetcher optional source port; Git full commit object/tree blob reads independent of checkout, rejects moving refs and unavailable commits; immutable revision fixture changes checkout. Docs explain mutable bundle vs revision, shallow availability and host retention. | Complete |
| C8. Breaking descriptor/checkpoint/wire/codegen semantics; reject old incomplete formats; remove posthoc digest paths | Version-prefixed descriptor validation and executable schema parity; compiled wire v3/StatePreserve rejects v1/v2 and legacy digest source; raw/canonical/from-registry-after-plan constructors removed. Captured generator index binds schema and digest to same captured bytes; logical/path alias collisions reject both index orders; regenerated constructors/goldens/consumers pass. | Complete |
| A1. OLD cache/NEW descriptor yields corresponding NEW or mismatch, never OLD under NEW | File source cache regression asserts mismatch before Reload and NEW after Reload; compiled OLD keeps OLD identity. | Complete |
| A2. Deterministic source mutation between protocol stages; no sleep-based race reproduction | Changing remote source fixture checks one-read capture and no compile reread; capture-only custom fixture forbids unrelated Plan/Verify; codegen captured-index replacement fixture. Independent ownership probes are deterministic and contain no sleeps. | Complete |
| A3. Imported/partial changes alter identity, unchanged-bundle traversal order does not | Conservative transitive/inactive import checkpoint tests, partial-only source identity change, canonical map/config ordering fixtures. | Complete |
| A4. File/embed/remote cached/uncached/Git/custom contracts and source disappearance | Partial/main filesystem removal after capture still executes OLD; embed caller FS mutation does not replace sealed generation; remote cache and uncached fixtures; Git local immutable commit fixture; minimal retained source port recipe restore. Shared materialization uses captured private closure/partials and needs no original source read. | Complete |
| A5. Concurrent Reload/Plan/Execute isolation, race, no caller plan-input mutation | 12×12 concurrent verified file Plan/compiled Execute regression; remote dedupe/cancellation/cache fixtures; template/history/bound maps are cloned; recursive ComposeValues/map and owned Lookup results pass nested ownership fixture. Remote fetched slice remains unchanged under rejected parser mutation. Independent race suites pass. | Complete |
| A6. Recipe checkpoint, raw BindRuntime and internal compiled use one digest meaning | Typed/raw capture-only fixture asserts identical descriptor/materialization and zero unrelated Plan/Verify; compiled compose/checkpoint digest parity; shared versioned source descriptor validators in construction/decoding. | Complete |
| A7. Environment, conditional composition/runtime values and partial resolution fixtures | File/embed/remote strict environment, missing/active/strict runtime conditional fixtures; conservative inactive import closure; file/embed partial patterns; environment/pattern/resolution mode/version/revision included in source config; deep runtime ComposeValues copies preserve branch selection. | Complete |
| A8. Docs distinguish source integrity, deterministic rendering and durable agent state | Public docs full source/capture/restore/revision/runtime/migration sections; README cache/registry links; contracts and closeout. | Complete |
| M1. Mandatory closeout: changed contracts, removed legacy paths, breaking migration, checks and limitations | Current closeout lists new ports/formats, removal of independent verify/Plan and compiled posthoc reads/constructors, regeneration/minimal capture migration, retained host restore policy and source/runtime limits. Final logs show all 12 modules lint/test/race plus generated consumers; no release/publication or host-service expansion. | Complete |

## Adversarial regression evidence

Both original independent probes were copied unchanged into the separate r3 copy and run again. Log `/tmp/task40-completeness-probes-r3.log`:

```text
TestAuditParsedMaterialIsolated:
descriptor retained=true; source OLD=true; output=OLD
PASS

TestAuditRemoteParserInputMutationRejected:
capture error=... parser mutated source bytes
parser calls=1; original source still OLD=true
PASS
```

Thus the r1 retained parsed alias and r2 preliminary remote peek bypass are closed by enforcement, not by documenting limitations. Built-in capture parses a private byte copy and seals the output; remote raw read no longer invokes the parser; shared public peek uses the same guarded parse helper. Public CaptureSources input mutation, output sealing, remote source mutation and peek source mutation regressions also pass.

Other r1 fixes were rechecked in current code and tests: recursive ComposeValues copies/owned Lookup results, codegen captured index and logical-ID/path alias collision rejection in both traversal orders. Independent module race execution includes those regressions and generated consumers. No obsolete digest/read path was found reintroduced by these fixes.

## Verification and practical boundaries

- Before/after integrity: 704/704 pass, frozen unchanged.
- Independent separate-copy `GOCACHE=/tmp/prompty-go-cache go test -race ./... ./cmd/prompty-gen/... ./remoteregistry/git/... ./cmd/prompty-gen/testdata/gen_out`: **exit 0**; `/tmp/task40-completeness-independent-r3.log`.
- Independent probes/public ownership fixtures: **exit 0**; `go test -race ./manifest -run 'TestAudit|TestRemoteCapture|TestPeekCompose|TestCaptureSources' -v`; `/tmp/task40-completeness-probes-r3.log`.
- Author final `/tmp/task40-lint-r3.log` and `/tmp/task40-test-r3.log` inspected: all 12 module markers, zero lint issues, test/race and explicit generated consumers passed. Targeted log `/tmp/task40-targeted-r3.log` and checks-r3 record targeted/full checks and clean diff. No live remote conformance claim is inferred from local Git/HTTP fixtures.

BYOT and boundaries preserved: source interpretation is a trusted callback/port, with ownership enforcement at callback boundaries; arbitrary parser/materializer semantics are not attested or sandboxed. Plain Registry remains usable without verified claims; custom capture has a single-method port; provider SDK types stay outside core. Host owns source trust/authentication, filesystem/Git access, immutable revision availability, persistence/history/materializer rebinding/retention and agent loop. Mutable multi-read bundles guarantee captured inputs, not an atomic storage revision. Runtime functions/services may produce nondeterministic text. These are the task's explicit boundaries, not omitted mandatory work.

Completeness acceptance: **17/17 = 100%** for the full audited scope. Commit/overall task acceptance remains the parent coordinator's responsibility together with the independent errors audit.
