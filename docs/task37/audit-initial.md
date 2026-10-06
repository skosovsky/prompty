# Initial independent audits

These reports describe the implementation slice before consumer-capture fixes. They are not final approval for subsequently changed code. Both agents read the original task and inspected the implementation independently; neither changed the workspace or made external writes. No immutable tree fingerprint was recorded for this initial pass, so its verdict must not be reused as final-snapshot evidence.

## Completeness

Agent `/root/completeness_audit` reported 133 fully completed / 151 required whole rows = 88.08%, excluding partial rows from the numerator. Seventeen additional pending-trigger rows and conditional fragments within required rows leave the overall applicable denominator unknown. This is conservative original-task accounting, not a claim of 88.08% final goal completion.

Seven stale partial assessments were independently reconciled: S01-04, S04-02, S04-04, S06-01, S12-03, S12-07, S12-08. S04-04 does not require a new proof-bearing arbitrary-Invoker interface: library-owned clients share Prepare, wrappers retain requirements, and truthful host-port implementation is a contract obligation. Adding transport attestation would expand the original scope.

Remaining source whole rows: S05-13, S09-06, S09-11, S10-01, S10-07, S12-01; S13-RELEASE, S13-CLOSE, S13-LINKS and S13-01–09. S09-11 is a genuine missing-reference origin resource/pointer gap. Conditional demand/dispositions remain unapproved. Author instructions/release-closeout content are substantially prepared; external delivery is not performed or currently authorized. The goal requires preparation, not unauthorized publication, comment or issue closure. Final conditional links and exact final audit/check evidence still need completion.

The auditor independently ran root/workspace and all four nested provider module tests successfully. It inspected the current full-module lint/race logs and Makefile. Local fixtures are not live account/model certification.

## Correctness

Agent `/root/correctness_audit` confirmed three findings and reported that original AUD-01–05 scenarios are fixed in the inspected implementation. Passing baseline tests did not expose these new intersections.

| ID | Severity | Trigger / consequence | Initial source |
| --- | --- | --- | --- |
| IA-01 | P1 | Consumer false after accepted content skips terminal capture; a prior frame observer failure also disappears from canceled status. Required closeout remains open even though source cleanup succeeds. | `stream.go`, checked observer in `adapter/client.go` |
| IA-02 | P2 | Every signed-history block overrides a message default with CacheOmit, but state validation rejects the unused default before cache preparation can mark it not_applicable. | `adapter/anthropic/state.go`, `adapter/prepared.go` |
| IA-03 | P1 | Gemini codec/Translate preserves integer 9007199254740993, but actual SDK request encoding sends 9007199254740992 with the same thoughtSignature and no checked error. DTO round-trip evidence is insufficient. | `adapter/gemini/state.go`, `adapter/gemini/gemini.go` |

The parent independently reproduced IA-01. Audit reproductions used local fixtures only in `/private/tmp/prompty-correctness-audit.WeWIxI/`; permanent tests must not depend on these temporary files. The auditor ran root/adapter/mediafetch and four provider tests successfully; it did not perform new exhaustive race/fuzz or live-provider/optional-remote audits. Severity, triggers and observed effects are confirmed; the report is not a proof that no other error exists.

## Required follow-through

Address all confirmed findings without weakening universal/BYOT boundaries or original acceptance criteria. Reconcile matrix statuses from actual evidence, preserve unresolved conditional scope and reference-origin diagnostics, rerun full gates, then rerun both same audit agents on the actual final state. No initial verdict above grants release/issue-close authority.
