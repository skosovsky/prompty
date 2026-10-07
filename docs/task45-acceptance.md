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

## Remaining stages (strict order)

2. A1–A3 release/codegen, local bare-remotes only.
3. CORE-F01–03, S1–S2 and S3; public regression, BYOT/exact numbers.
4. S-F01–02, OA01–02, AN01, GO-F01–02; wire/parity/partial/terminal regressions.
5. R01–04, O-F01; channel-based races and evidence-based cancellation.
6. All appendix design items with leave/change/remove, rationale and spec/test reference; API/docs/examples/migration/CI and performance evidence where needed.

Final gates: make lint; make test across all modules and explicit generated consumer; deterministic regeneration; GOWORK=off publishability; final two independent reviews. Diagnostic tests that assert old defects must become expected-contract regressions. All 19 main defects plus GO-F02 must have valid and negative controls. No real release/live provider calls.
