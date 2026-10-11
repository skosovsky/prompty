# Task46 — completed

Issue: https://github.com/skosovsky/prompty/issues/5 — CLOSED, 2026-10-07T09:31:43Z.
Migration comment: https://github.com/skosovsky/prompty/issues/5#issuecomment-6035108139.

## Result

Pure core `AdmitToolCalls` shares original-schema/scope/whole-batch validation with the one-shot helper, with owned snapshots and fail-closed continuation. The helper remains JSON-only; ordinary handler errors continue admitted calls and cancellation stops them. Universal core acquired no external runtime dependencies.

Optional `examples/host_dispatch` demonstrates actual SDK HTTP → full canonical checkpoint → pure admission → real ID-aware tool execution → explicit model result projection → next checked HTTP request. Native IDs and exact raw JSON remain intact, including integers above 2^53. Caller owns approval cursor, outcomes, audience policy, opaque codecs and stream completion joining; unsupported mappings explicitly fail. Published/source runners and CI pins establish a portable independent integration boundary.

## Acceptance evidence

| Requirement | Authoritative evidence |
| --- | --- |
| AC1–AC8 implementation, contracts, full dispatch, schemas, projection/state/lifecycle | completeness-r3.md, errors-r4.md, optional/core AAA regression tests |
| AC9 current local sources | local-r8.json, actual HEAD/dirty recorded, race count1/lint/demo PASS |
| AC9 clean CI source refs | ci-source-r2.json, exact declared release commits, clean working trees, race count1/lint/demo PASS |
| AC9 published dependencies | published-r1.json and independently completeness-published-r3.json, GOWORK=off/no Replace, race count1/lint/demo PASS |
| AC10 required full gates | make-lint-r3.log, make-test-r2.log, make-release-check-r2.log; repeated by release-break-r2.log |
| AC11 independent reviewers | completeness-r3.md 100%; errors-r4.md PASS/no open confirmed errors; fixes independently rechecked |
| AC12 release | release-break-r2.log, release-transaction.json published, remote-refs.json all 13 expected refs match |
| AC12 migration message and closure | migration-comment.md, issue-comment-verification.json exact posted body, issue-closeout.json CLOSED |

## Release and migration

`make release-break` published v0.16.0 from source c2253dc71ccc531707cf3b2fbbb89bc859d5d751 as isolated release commit 4d028f87265f687580ab6614dfcf36bb6d125b45. New public admission API and unavailable-continuation rejection are material contract changes. Publication reconciled successfully; no uncertain or conflicting refs remain. A stalled SSH read failed before preparation; the successful retry used process-local HTTPS configuration without changing the repository remote.

The issue author was explicitly told to retain the JSON helper only for one-shot scenarios; move ID/approval/business outcomes to checked Execute + pure admission + host dispatch; preserve AssistantMessage and StatePreserve instead of TextContent history; check business errors/audience/unsupported destinations; join cleanup after consumption rather than waiting for Close. The comment includes API/setup snippets and the runnable example.

## Limits

Offline provider SDK HTTP and in-memory approval fixtures establish contract behavior. Live provider support, authenticated production IAM/durable storage, signature validation and remote rollback were not tested or promised. Hosted CI run is not claimed: equivalent clean pinned-source fixtures were run locally. Consumer default branches precede the supported published contracts; no passing remote HEAD claim or hidden fallback is made. Current local and fixed CI source refs are explicitly separate evidence.

All acceptance criteria and required external actions are complete. Full logs and historical failed rounds remain in this report directory; final evidence is listed above.
