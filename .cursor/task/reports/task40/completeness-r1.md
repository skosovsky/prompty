# Task40 — independent completeness audit, round 1

Decision: **REJECT — 88.24% (15/17 complete)**. Partial requirements are not counted. This is the result of an independent audit of the frozen implementation, not a mathematical guarantee of defect absence.

## Identity and scope

Audited only `/tmp/prompty-task40-snapshot-01`, baseline `c8141c5`. SHA-256 of its manifest: `3726add65ce44f68b23b4e86d60309285bae3d7c48aa78272c6ebd2688549cfe`; all **697/697** file hashes verified (`/tmp/task40-integrity-r1.log`). Implementation was not edited. Independent probe was added only to `/tmp/prompty-task40-audit-r1`, a separate copy. The other current-round auditor's report was not read.

Scope is the entire task40 specification, `.cursor/task/README.md`, implementation contracts, public source-snapshot documentation and closeout. Denominator: eight numbered requirements, eight acceptance bullets, one mandatory closeout = **17**. README constraints apply across these requirements and are not used to shrink scope. The checked-off status of implementation/acceptance is not evidence by itself.

## Requirement matrix

| Requirement | Code and test evidence | Status |
| --- | --- | --- |
| C1. Unified verified materialization, captured identity/sources, no parallel catalog | `manifest_snapshot.go`: `SourceBundle`, private snapshot fields, small `ManifestSnapshotRegistry`, `VerifiedManifestPlan`; `prompty.go` checkpoint port; source getter defensive-copy fixture | Complete |
| C2. Verification and materialization use the same captured bytes, without verify/read TOCTOU | `VerifiedManifestPlan` compares one returned snapshot then invokes its Plan; recipe/runtime use this path; deterministic changing-source and unrelated-Plan fixtures pass. However built-in captured material retains parser output aliases and can change after capture while source bytes/identity remain OLD (F1). | **Partial** |
| C3. Deterministic main/import/layer/partial/config closure; inactive branches, cycles, traversal independence | `manifest/snapshot.go` conservative recursive capture of all declared imports; layer import refs resolved from declared imports by `manifest/compose.go`; named partials/config enter canonical JSON; sorted JSON map-key encoding; `manifest/checkpoint_test.go` transitive/inactive imports and corruption; `compose_digest_test.go` cycles/empty IDs; source partial-only change and canonical map ordering tests | Complete |
| C4. Source digest excludes Go functions/services/time/random/model, host runtime boundary explicit | `docs/source-snapshot-contract.md`, README link, contracts/closeout; no function/service serialization or deterministic-render promise | Complete |
| C5. Cached identity and immutable parsed/compiled material belong to one generation; explicit retained restore/mismatch | File and TTL caches retain snapshot objects; plans preserve source descriptor, compiled consumes it, host retained snapshot port documented/tested. But captured parsed closure aliases externally accessible parser output, so cached material is not immutable (F1). | **Partial** |
| C6. All built-in registries, typed/raw/compiled; minimal custom port and incomplete custom rejection | File/embed/remote capture paths share `manifest.CaptureSources`; TTL capture refuses base without capture capability; Git revision port; recipe and runtime signatures require only capture port; internal compiled rejects uncaptured plans; `TestSourceSnapshot_CheckedRecipesDoNotCallUnrelatedPlan`, minimal custom restore fixture, compiled tests, Git fixture. Coverage/application exists across all required ports; ownership defect is recorded under C2/C5. | Complete |
| C7. Mutable bundles do not claim atomic revision; explicit immutable binding fails unsupported/unavailable; no VCS/storage service | `remoteregistry.Registry.CaptureManifest` explicit `RevisionFetcher` binding; `git/revision.go` full commit, immutable tree blobs, reject moving refs/missing commit; revision fixture changes checkout; docs distinguish mutable multi-read bundle from atomic revision | Complete |
| C8. Breaking formats/wire/codegen, old incomplete formats reject, posthoc digest paths removed | `manifest_descriptor.go` versioned digest validation; executable descriptor JSON Schema parity test; compiled wire version 3, legacy rejection; removed raw/canonical/registry-after-plan constructors; generator captured index supplies both specs and descriptors; captured-index source-replacement test and generated consumers | Complete |
| A1. OLD cache/NEW descriptor returns NEW or mismatch, never OLD under NEW | `TestSourceSnapshot_FileCacheCannotAcquireNewDescriptor`: old cache mismatch, explicit Reload NEW, compiled OLD retains OLD identity | Complete |
| A2. Deterministic source replacement between protocol stages, no sleep race reproduction | Changing remote fixture returns OLD first read then NEW; compiled performs no second read; unrelated Plan/Verify fixture cannot participate in checked materialization; captured codegen source-replacement fixture | Complete |
| A3. Imported/partial changes change identity; traversal ordering does not | Conservative transitive/inactive import checkpoint fixtures; partial-only OLD→NEW identity fixture; canonical ordering/defensive-copy test | Complete |
| A4. File/embed/remote cached/uncached/Git/custom contract; source disappearance after capture | Source snapshot tests cover file disappearance and explicitly retained custom source; embed caller FS changes do not affect capture; remote cached/uncached one-read fixture; local Git immutable binding fixture; shared capture materialization does not read original reader | Complete |
| A5. Concurrent Reload/Plan/Execute isolation, race; no caller-input mutation | `TestSourceSnapshot_ConcurrentReloadPlanIsolation` with 12×12 concurrent verified Plan+compiled Execute; remote cache concurrent dedupe/cancellation tests; source input/config defensive clones; `NewRenderPlanFromPlanInput` clones bound vars/history, template getter/plan clone isolation; independent root/module race runs pass. Parser output ownership is separately C5 and is not a caller plan input. | Complete |
| A6. Recipe checkpoint, raw BindRuntime, compiled have one digest meaning | Capture-only recipe/runtime fixture verifies identical descriptor and text, no unrelated verifier/Plan; compiled compose digest matches checkpoint; all constructors/wire validate source-bundle identity | Complete |
| A7. Environment, conditional composition/runtime values, partial resolution fixtures | Existing file/embed/remote environment and conditional strict/missing/runtime fixtures now execute through snapshot Plan; conservative inactive closure tests; file/embed partial fixtures; resolution mode/pattern/environment/version/revision included in source config | Complete |
| A8. Docs distinguish integrity, deterministic rendering and durable agent state | `docs/source-snapshot-contract.md` full source/restore/runtime boundaries, README registry/cache links, contracts and closeout | Complete |
| M1. Mandatory closeout, breaking migration, all-module checks and remaining limits | closeout lists new ports/formats, removed paths, regeneration/host restore migration and limits; checks-r1 records all-module lint/test/race/generated consumers; supplied logs inspected; no release/tag/publication/agent-loop/storage added | Complete |

## F1 — captured parsed closure remains externally mutable

Location: `manifest/snapshot.go:85–90`, retained closure at `:35–44`, materialization at `buildParsedTemplate` (`:133` onward).

`CaptureSources` supplies `&raw` to injected `Unmarshaler` and stores exactly that pointer in the captured parsed map. The parser can retain the output pointer (or returned slice/map buffers). After successful capture, changing that object changes subsequent materialization. Cloning at Plan time does not seal the original parsed generation: it copies its already changed value. The bundle mutation guard cannot detect this because the materializer reads the captured parsed closure rather than the supplied byte copy.

Independent deterministic probe: `/tmp/prompty-task40-audit-r1/manifest/task40_audit_probe_test.go`, log `/tmp/task40-completeness-probe-r1.log`.

Steps:

1. Reader returns a valid JSON manifest whose sole text is `OLD`.
2. Parser delegates to the standard JSON parser, successfully parses the same bytes, and retains the `*RawManifest` output pointer.
3. Capture succeeds; retain snapshot descriptor.
4. After capture returns, set the retained parsed text to `NEW`.
5. `snapshot.Plan` and Execute both succeed; exported captured bytes still contain `OLD`, descriptor unchanged, execution text is `NEW`.

Command: `GOCACHE=/tmp/prompty-go-cache go test -race ./manifest -run TestAuditParsedMaterialIsolated -v` in the separate copy. Exit **1**:

```text
descriptor retained=true; source OLD=true; output=NEW
parsed material aliases parser-owned output: NEW
```

The test does not install an arbitrary custom SnapshotMaterializer or reread source storage. It exercises the built-in capture/materialization implementation with the public parser port. The specification excludes attestation of parser *code* and trusts parser interpretation; that does not make a captured parsed object immutable once ownership crosses the callback boundary. The parser initially returns a correct interpretation of OLD; only the retained output alias changes afterwards. Sealing the parsed generation is the library's C5 responsibility. Documentation of trusted host code is not automatic enforcement of this invariant.

Expected acceptance evidence: parser-retained raw/slice/map mutation after successful capture cannot affect captured material; main and imported parsed nodes are isolated, and ordinary builtin parsers continue passing the existing same-generation fixtures. No universal DTO/storage service or attestation of arbitrary parser semantics is required.

## Checks and limits

- Independent integrity: all 697 hashes pass.
- Independent separate-copy `go test -race ./...`: exit 0; `/tmp/task40-completeness-root-r1.log`.
- Independent separate-copy `go test -race ./cmd/prompty-gen/... ./remoteregistry/git/...`: exit 0; `/tmp/task40-completeness-modules-r1.log`.
- Supplied all-module lint/test logs inspected: all 12 modules, zero lint issues, test/race and explicit `gen_out` consumers pass. Logs `/tmp/task40-lint-final-r1c.log`, `/tmp/task40-test-final-r1b.log`; final boundary race rerun `/tmp/task40-final-boundary-r1.log` passes after formatting-only changes.
- Independent adversarial ownership probe fails as detailed above; green existing suites do not cover that boundary.

BYOT and scope boundaries otherwise preserved: capture is a single-method port, regular Registry remains usable without verified claims, provider types do not enter core, immutable Git binding is an optional source capability, host retains storage/history/authentication/retention and agent-loop responsibilities. No requested scope was waived. Acceptance requires a new frozen snapshot and paired audit after the incomplete invariants are addressed.
