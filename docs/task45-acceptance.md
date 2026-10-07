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

## Remaining stages (strict order)

4. S-F01–02, OA01–02, AN01, GO-F01–02; wire/parity/partial/terminal regressions.
5. R01–04, O-F01; channel-based races and evidence-based cancellation.
6. All appendix design items with leave/change/remove, rationale and spec/test reference; API/docs/examples/migration/CI and performance evidence where needed.

Final gates: make lint; make test across all modules and explicit generated consumer; deterministic regeneration; GOWORK=off publishability; final two independent reviews. Diagnostic tests that assert old defects must become expected-contract regressions. All 19 main defects plus GO-F02 must have valid and negative controls. No real release/live provider calls.
