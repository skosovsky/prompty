# Task45 acceptance record

This is execution evidence and a checklist, not a parallel normative standard. Source requirements: [.cursor/tasks/task45-prompty-review-remediation.md](../.cursor/tasks/task45-prompty-review-remediation.md), including its appended role reports. Each stage must reach independent completeness 100% and correctness PASS before its commit and the next stage. Current changes document target contracts; they do not claim defects are fixed.

## Stage 1 — contracts checklist

| ID | Requirement / acceptance evidence |
| --- | --- |
| C1 | Ownership: JSON defaults isolate nested values; token borrowing and mutable generic DTO limits explicit; executor serial one-attempt policy; stream single-consumer/Close semantics; registry lifecycle. provider-exchange-contract + source-snapshot-contract. |
| C2 | Stream shape table, optional/repeated Start, terminal ownership, immutable finish/exact repetition, late annotations, invalid enum/atomic rejection, partial evidence, empty fragments/final document. provider-exchange-contract. |
| C3 | Exact structural decimals vs byte digests, field dominance before inference, no schema fallback, valid empty input after validation, scoped dependencies, compiler hardening choice. schema-data-contract. |
| C4 | Evict/EvictAll generation boundary, waiter isolation, literal HTTP IDs/read errors, Git fail-closed refresh/cancellation, close lifecycle. source-snapshot-contract. |
| C5 | Raw invalid-input policy and source-backed vs raw DTO guarantee; sync/native envelope parity; role/PDF projection, no live claim. provider-exchange-contract. |
| C6 | Isolated release transaction, explicit refs/atomic publication/reconciliation, GOWORK=off graph, platforms/major boundary, complete codegen inventory/staging/ownership. README Development release contract. |

Validation: document diff and cross-check against source requirements; no implementation test claims for this documentation stage. Independent reviews completed on the final document diff:

- `/root/stage1_completeness`: 100% (C1–C6 all closed), no open gaps. Initial C3 gap corrected with explicit shared compiler mutation guard and permitted exact normalization.
- `/root/stage1_correctness`: PASS, no open findings. Exact Annotations field name and monotonic ContinuationUnavailable semantics clarified and re-reviewed.
- `git diff --check`: PASS. Scope: target specifications and source/fixture shape inspection, not production fixes, tests or live-provider conformance.

Stage 1 accepted; commit: `docs: public contracts`.

## Stage 2 — release/codegen checklist

| ID | Requirement / proving evidence |
| --- | --- |
| RC1 (A1) | Prepare from explicit HEAD commit in isolated checkout; original branch/detached state, index, working files and untracked fixture unchanged. Stage only declared manifests; full diff rejects unexpected paths. Actual release script exercised on disposable repository. |
| RC2 (A1) | Publish exact root/submodule refs only; unrelated local tag stays absent remotely; intended refs all resolve to validated release commit. Atomic push without non-atomic fallback. |
| RC3 (A2) | Rejected/uncertain publication records exact version/source/commit/ref transaction, reconciles remote refs; safe retry uses same version, partial/conflicting/unknown state blocks fresh bump. Preserve original checkout and avoid stale version tags. Exercise rejection and reconciliation fixtures. |
| RC4 | Validate inventory/selected module paths; portable edits; v0 boundary enforced; release graph validation uses GOWORK=off and declared same-release dependencies. Tests cannot require a real publication. |
| RC5 (A3) | Derive complete filename inventory before output writes; normalized distinct basenames and shared filename collision reject with both owners, without partial output. |
| RC6 | Validate complete package Go symbol inventory, including helper/ID/type/normalized property conflicts; valid noncolliding fixture consumer compiles. |
| RC7 | Generate staging content before publishing; preserve arbitrary consumer files; deterministic explicit ownership inventory permits only owned stale cleanup and mode switches. Failure preflight leaves prior outputs intact. |
| RC8 | Config version1/complete YAML document, nonempty valid Go package/output/source settings; duplicate destination packages reject before mutation. Include invalid controls. |
| RC9 | Meaningful AAA regression controls, repeated generation unchanged; targeted release/codegen checks pass; both independent stage reviews 100%/PASS. |

Inspection: current release script detaches user HEAD, stages all files and pushes all tags. Current generator writes shared file before per-source validation and normalizes output basenames without an output inventory. Candidate implementation: isolated Python-stdlib release transaction delegated from the bash entrypoint; prepublication proxy archives and GOWORK=off graph compilation; explicit commit-to-ref atomic push with persisted recovery state. Generator uses all-package staging, filename/AST symbol checks and digest-based output ownership/rollback.

Candidate checks:

- `GOCACHE=/tmp/prompty-task45-go-cache go test -race ./cmd/prompty-gen/... -count=1`: PASS (generator + gen), including standalone consumer with pinned dependency sums and temporary writable module cache.
- `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_release.py`: PASS, 8 tests, all disposable local repositories/bare remotes. Covers success, rejection/resume, partial conflict, lost ack, atomic support inventory/major rejection, relative local remotes including colon-containing paths, and followed annotated tags. Fixture and synthetic release commits have explicit unsigned signing policy.
- Generator module lint: PASS, 0 issues.
- Initial independent acceptance found catalog field/method collisions, symlink destination aliases, relative remote resolution and ambient signing. Corrections and regressions are implemented; final repeated acceptance completed. Full-project local publication smoke exposed missing standalone transitive requirements; module manifests/checksums now include that exact graph while keeping development internal versions/replacements. Full-project disposable smoke: PASS, actual current code/module graph, 12 modules built with GOWORK=off and exact refs published only to the local bare remote. Fixture `/var/folders/46/5ywmz5gj26n7mnky51gd60g00000gn/T/prompty-task45-release-project-6jm1efwi`; release commit `544a9ee654d7e6f7053c27cc2d3815ca1fc501ba`. Independent correctness review confirmed exactly 12 refs and only 11 inventoried changed go.mod files in the release commit. The last relative colon-path finding is fixed with Git-compatible slash/colon classification and a dedicated regression; final repeat reviews completed.

- `/root/stage2_completeness`: 100%, RC1–RC9 closed; independently repeated Python 8-test suite and generator race tests, inspected full-project local refs/diff.
- `/root/stage2_correctness`: PASS, no open confirmed findings after independent repros of catalog fields, symlink aliases and both relative-remote shapes; full-project refs/diff independently inspected.
- Final `git diff --check`: PASS. Full workspace lint/test and platform matrix remain final-goal gates; stage2 evidence is local macOS Go1.27.1 and offline fixtures.

Stage 2 accepted; commit: `fix: release safety`. No real refs/releases or live providers touched.

## Stage 3 — core/schema checklist

| ID | Requirement / proving evidence |
| --- | --- |
| CS1 (CORE-F01) | Constructors, reusable partial-variable options and CloneTemplate own independent nested JSON maps/slices. Caller/source/returned plan template/sibling mutations cannot affect other executions. Exact json.Number values retained; owned clone race regression. Parsed templates stay shared read-only. |
| CS2 (CORE-F02) | Required dependency extraction understands range/with dot scope, variable aliases/root, else, nesting and named partial invocation. Ordinary local member names do not become required root variables. Genuine missing root fields/schema constraints still fail; optional-message inclusion follows real dependencies. |
| CS3 (CORE-F03) | ValidateSchema errors survive empty/history-only recipe inputs and block registry access for late/no-late Plan/Execute variants. Valid empty structs render. Invalid history/embedding/binding errors survive; remove error-swallowing fallback rather than adding aliases. |
| CS4 (S1) | Reflection resolves encoding/json field dominance before selected field inference. Shadowed/ignored/ambiguous unsupported fields do not trigger errors; selected unsupported controls still fail. Result schema matches actual JSON projection. |
| CS5 (S2) | Strict mapper uses shared exact decimal structural equality in root/nested object/array/enum constraints; decimal/exponent normalization permitted without float64 and with distinct byte digests. Adjacent large integers/different decimals/invalid documents reject. |
| CS6 (S3) | CompileProviderSchema guards successful source and translated compilation through compileSchemaSnapshot. Structural contract mutation rejects before mapper/dispatch; harmless exact normalization allowed. Honest compiler valid control passes; no claim to detect a lying validator without input changes. |
| CS7 | Public API/BYOT AAA regressions have negative and valid-path controls, retained sentinel identity and precision. Targeted core/schema race + lint checks PASS; two independent reviews 100%/PASS before commit. Diagnostic old-behavior tests must not be counted as fixed regressions. |

Candidate implementation: deep JSON defaults at option/constructor/CloneTemplate ownership boundaries; dependency extraction tracks dot provenance and lexical aliases across control scopes and named templates; optional dependencies computed after all definitions; recipe admission preserves validation/binding errors and explicitly binds zero-field structs; reflection infers only dominant fields; strict mappings use shared exactjson equality; provider source and translated compilers use the shared snapshot guard.

Validation:

- Root `go test -race ./...`: PASS, including dependent root-module registries/parser/exchange packages. Final root run 5.484s; logs `/tmp/prompty-task45-core-race.log`.
- Root `golangci-lint run --allow-serial-runners ./...`: PASS, 0 issues. Log `/tmp/prompty-task45-core-lint.log`.
- Permanent AAA regressions in `task45_core_regression_test.go` cover public RenderPlan.Template/Execute isolation and exact numbers; typed scope render, optional messages, missing input/schema constraints; late/no-late admission sentinels and invalid binding; dominant-field JSON projection; exact decimals and byte digests; original/translated honest compiler mutation controls.
- Initial completeness gaps added as public/negative controls. Independent correctness reproductions exposed escaping assignments, literal index provenance, later partial definitions and action declaration shadowing; each fixed and retained in permanent regressions. Final reviews are recorded after repeat acceptance. Tests use local BYOT fixtures, no live providers.

- `/root/stage3_completeness`: 100%, CS1–CS7 closed; independently repeated race scope regressions and inspected current validation logs.
- `/root/stage3_correctness`: PASS, no open confirmed findings; independently repeated original adversarial overlays under race and reviewed all six core/schema defects/hardening requirements.
- Final `git diff --check`: PASS. Scope: root-module public/BYOT fixtures and local macOS checks; final workspace/module/codegen gates remain required later.

Stage 3 accepted; commit: `fix: schema fidelity`.

Complete appendix design-item disposition remains stage6, except S3's already explicit stage1 hardening decision.

## Stage 4 — streams/adapters checklist (accepted)

| ID | Requirement / proving evidence |
| --- | --- |
| SA1 (S-F01) | Non-nil empty first/middle/last text fragments are no-ops during native/buffered structured parsing; genuinely empty whole document fails at finalization. Nil/non-text parts still reject. Actual Anthropic local SDK/SSE empty block-start/citation frames preserve annotations, provisional items, one terminal/capture and transport cleanup. |
| SA2 (S-F02) | Accepted finish outcome/reason are immutable. Late usage may omit or exactly repeat them and may carry ordered MessageAnnotations. Contradictory/unknown outcomes fail atomically; accepted finish and prior usage/prefix survive. Incomplete/refusal/paused/tool-call outcomes never become structured success via metadata. One terminal/capture failure, continuation revoked. |
| SA3 | Central accepted source event shape follows stage1 stream contract: optional/repeated metadata Start, lifecycle-owned Result/Terminal, per-kind content/finalization/state/annotation constraints, enum validation on every event, monotonic ContinuationUnavailable and no partial installation of invalid frames. |
| SA4 (OA01) | OpenAI Chat distinct system/developer roles preserve exact roles/order in actual local SDK HTTP body; input remains unchanged. Checked supported-profile control passes without hidden role mapping. |
| SA5 (OA02) | Inline Chat PDF has filename plus exact reversible application/pdf data URI consistent with Responses. Actual SDK local request fixture; unsupported/malformed media reject before dispatch; no implicit fetch/upload. |
| SA6 (AN01) | Source-backed Anthropic sync validates consumed text, usage, tool IDs/names before SDK coercion, reusing narrow native validation where equivalent. Checked local HTTP plus reused buffered/batch parsers reject malformed source with honest partial/no successful continuation. Unknown optional source fields and valid raw-free typed fixtures remain supported. |
| SA7 (GO-F01) | Gemini/Ollama sync, buffered/native and raw decoder roles/content agree: explicit foreign role rejects, assistant/model/contractual omitted-role controls succeed; Ollama output images reject rather than disappear. Foreign-role tool payload cannot reach local handlers. |
| SA8 (GO-F02) | Public raw Ollama stream decoder rejects untyped nil, typed nil and wrong type with ErrInvalidResponse without panic. Valid/empty typed chunk controls keep documented behavior. |
| SA9 | AAA public wire/partial/terminal parity regressions and touched root/provider module race+lint PASS; independent completeness 100% and correctness PASS before stage commit. Offline local SDK fixtures only, no live conformance claim. |

Accepted implementation: canonical source event-shape validation and immutable finish guard before accumulation; empty fragments are parser no-ops; Chat mapping preserves native developer role and all input message boundaries with PDF filename/data URI; shared Ollama/Gemini role/content checks and raw typed-nil rejection; source-backed Anthropic consumed text/tool identities/usage checked before SDK coercion, retaining independently valid usage counters and accepted canonical prefix.

Current evidence:

- Root `go test -race -timeout=90s ./...`: PASS, including remoteregistry (1.691s). Log `/tmp/prompty-task45-stage4-root-race.log`. A prior run exposed a cancellation-test scheduling race: its 10ms deadline could expire before the fetch worker started, leaving the test waiting forever. The test now coordinates worker start, waiter admission, cancellation and fetch release with channels; no registry production changes are included in this stage.
- Latest core + Anthropic `go test -race -timeout=90s . ./adapter/anthropic/...`: PASS; log `/tmp/prompty-task45-stage4-core-an-race.log`. Permanent tests cover authoritative final outcomes, deferred provisional content, immutable finish/reason, atomic invalid-frame rejection, genuine empty documents, and actual Anthropic SDK SSE/buffered citation and empty-start frames with one capture/terminal/transport close. Consumed-source fixtures cover sync, buffered, native and batch paths with valid controls.
- Gemini + Ollama module race: PASS; log `/tmp/prompty-task45-stage4-go-ol-race.log`. Checked local HTTP tests cover sync/buffered/native valid and foreign roles, omitted-role controls, unsupported Ollama images, raw nil shapes and real tool handlers. Foreign-role calls do not invoke handlers; admitted controls do.
- OpenAI module race: PASS; log `/tmp/prompty-task45-stage4-oa-race.log`. Checked local SDK HTTP verifies exact system/developer order, unchanged input and reversible PDF filename/data URI; unsupported or malformed media never dispatch.
- Root/OpenAI/Ollama/Gemini lint: PASS, 0 issues. Anthropic module lint: PASS, 0 issues; `/tmp/prompty-task45-stage4-anthropic-module-lint.log`. Other logs `/tmp/prompty-task45-stage4-<module>-lint.log`.
- `/root/stage4_completeness`: 100%, SA1–SA9 closed; independent full race run core/AN/Gemini/Ollama/OpenAI PASS and five lint logs inspected.
- `/root/stage4_correctness`: PASS, no confirmed open findings; both prior adversarial probes pass. Independent race runs core/adapter/four providers and diff check PASS.
- Final `git diff --check`: PASS. All fixtures use local transports; no live provider conformance is claimed.

Stage 4 accepted; commit: `fix: stream fidelity`.

Complete remaining appendix design decisions stay stage6; this stage enforces the already specified source/frame contracts and mandatory defects.

## Stage 5 — sources/observation checklist (accepted)

| ID | Requirement / proving evidence |
| --- | --- |
| SO1 (R01) | HTTP preserves decoded logical ID bytes via Path/RawPath. Percent/slash/dot/query/fragment/non-ASCII controls, configured base path/query, distinct sources, direct Fetch and Registry.Plan imports. Traversal remains rejected. |
| SO2 (R02) | Bounded HTTP reads accept exact 1 MiB only with EOF; extra byte/zero progress/read error/cancellation remain failures; body closes once. |
| SO3 (R03) | Evict/EvictAll detach old inflight generation. Existing waiters may finish; new callers dedupe only fresh work; old completion cannot overwrite/delete new state. Channel-based overlapping captures under race. |
| SO4 (R04) | Git admission/post-lock/post-refresh/pre-result cancellation and deadline never yield bytes/metadata; mutable refresh errors fail closed. Local initialized/uninitialized/persistent/file paths, waiting mutex, active Pull and bound revisions checked. No retries/live remotes. |
| SO5 | File/embed metadata and file/Git traversal preserve cancellation instead of notfound/success. Valid controls and error identities retained. |
| SO6 (O-F01) | Shared sync/buffered/native capture classifier preserves known canceled/schema/parse/capture causes, arbitrary errors become operation, transport needs typed evidence. Actual mode terminal capture; errors.Is retained, AfterDispatch independent, no invented outcome/retry/raw error export. |
| SO7 | Normative source/capture contracts and AAA fixtures agree; root/Git module race+lint PASS; two independent reviewers 100%/PASS before commit. Appendix policy decisions remain stage6 unless explicitly required above. |

Candidate evidence:

- Root + Git `go test -race -timeout=90s ./... ./remoteregistry/git/...`: PASS; `/tmp/prompty-task45-stage5-final4-race.log`. Covers all root-module packages and separate Git module. Offline fixtures only.
- Root and Git module lint: PASS, 0 issues; `/tmp/prompty-task45-stage5-root-lint.log`, `/tmp/prompty-task45-stage5-git-lint.log`.
- Permanent AAA tests prove actual HTTP URI/path/query projection and imported logical IDs; exact-limit EOF/errors/nonprogress/byte-with-error and one body Close; both Evict variants and completion orders using channels; Git admission/deadline/post-lock/active local HTTP Pull/immutable revision, fail-closed missing origin; file/embed successful and missing metadata closeout, per-entry file/Git walks; actual sync/buffered/native terminal capture and typed transport versus local URL parse failures.
- Initial independent reviews exposed missing deadline/traversal controls, metadata missing-path cancellation and inconsistent trailing escaped base. Corrected with permanent regressions and explicit gates. Final repeated independent acceptance: `/root/stage5_completeness` 100% (SO1–SO7), `/root/stage5_correctness` PASS with no confirmed open findings. Each independently ran full targeted scope under race and inspected lint; both adversarial correctness overlays now PASS.

Stage 5 accepted; commit: `fix: source consistency`.

## Stage 6 — API/docs/simplification checklist

| ID | Requirement / proving evidence |
| --- | --- |
| AD1 | All eight appendix decision registers have exact item inventory, leave/change/remove, rationale and normative/test reference. Changed items implemented; performance hypotheses supported by benchmarks, never assumed. |
| AD2 | Remove ToolHandler, private toolInvoker.name, ExecuteWithToolValidation/ErrToolInvokerRequired; update consumers and migration. Retain whole-batch side-effect preflight and BYOT ToolInvoker. Share validated recipe paths where justified; retain fidelity guards. |
| AD3 | README direct runnable BYOT quickstart and complete optional manifest/codegen workflow; basic_chat correct cwd; allocation claims limited to measured paths. doc.go/current-contract index and superseded historical docs. |
| AD4 | Ownership/concurrency table and executor decision: immutable snapshot vs mutable DTO, serial helper, single-consumer lazy stream/Close cancellation, registry/cache lifecycle. Full model/profile/raw/checked/native/buffered/evidence/media/schema distinctions. |
| AD5 | Public limits/privacy/boundaries accurate: cancellation not rollback, unknown outcome/reconciliation/retries, usage unknown, estimator approximation, retention versus transport memory, callbacks/backpressure, auth/egress/source trust. Naming/setup/comment corrections from all appendices. |
| AD6 | Declared module inventory equals go.work/Makefile/CI; explicit committed generated consumer, deterministic regeneration diff; standalone publishable graph, supported release platforms/major migration, module license packaging evidence. |
| AD7 | Stage6 current scope race/lint/docs/examples/benchmarks PASS; independent completeness100% and correctnessPASS then short commit. Final workspace DoD and final dual acceptance follow separately. |

Stage6 accepted after repeated independent reviews:

- `/root/stage6_completeness`:100%,141/141 appendix items; exact inventory and all AD1–AD7 requirements verified independently.
- `/root/stage6_correctness`:PASS; no confirmed open errors. Initial singleton-object schema and compose example findings fixed; repeated public probes and race suites pass. Unknown event fixtures and handwritten generated-consumer tests rechecked.
- Current `make lint`:PASS for12 workspace modules plus explicit generated consumer,13×0 issues (`/tmp/prompty-task45-stage6-repeat-lint.log`). Current `make test`:PASS across the same scope under race (`/tmp/prompty-task45-stage6-repeat-test.log`).
- Current public CLI regeneration:11 files unchanged by SHA256, including ownership inventory (`/tmp/prompty-task45-stage6-final-regeneration.log`); reviewers also regenerated independent copies.
- Benchmarks and scoped design rationale: [decision register](task45-design-decisions.md) and [performance report](task45-performance.md). All141 appendix items have explicit leave/change/remove with evidence.
- Actual12-module candidate standalone release graph PASS with GOWORK=off, no local replacements or required go.mod edits. Only disposable bare remote published (`/tmp/prompty-task45-stage6-current-release.log`). Eleven release/workspace tests include exact root/nested license bytes.

Stage6 commit: `fix: api consistency`. Committed-HEAD regeneration, full final workspace gates and final independent double acceptance remain post-commit work. No real release/live provider calls.
