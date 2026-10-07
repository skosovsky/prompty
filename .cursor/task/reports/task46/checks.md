# Task46 — verification and release decision

The core exposes only pure `AdmitToolCalls` using existing caller-owned execution/response/validator types. Approval, sequential effects, outcome projection, context codecs and joining resource ownership remain in the optional example. No external consumer dependency was added to root or production adapters.

## Verified gates before publication

- `make lint`: PASS, make-lint-r3.log, every inventoried module and generated consumer.
- `make test`: PASS, make-test-r2.log, every inventoried module and generated consumer, race enabled.
- `make release-check`: PASS, make-release-check-r2.log, 11 transaction/inventory tests.
- Fresh optional `go test -race -count=1`: PASS, targeted-r4.log after UTF-8 corrections.
- Actual current local source trees: tests, lint, demo PASS, local-r8.json; HEAD and dirty state explicitly recorded. No external repository was changed.
- Clean CI source checkouts: tests, lint, demo PASS, ci-source-r2.json. Tool execution commit 6ed0a8a6a4e664c25f32397fa63dd6f0a46cba09, semantic context commit 3ce901169b09b7136b126d9baa98e1875b33fa82, lifetime commit fe61308e030e163195ccdb4d548109a5a4ebdde1. These are commits underlying the declared published external dependency set. Their remote default branches precede the tested contracts; clean-head-r1.json records the explicit compile failure rather than a successful HEAD claim. CI checks out these exact refs.
- Root dependency graph/manifests prove core independence; the optional module has its own dependencies and release inventory entry.
- Error acceptance PASS: errors-r3.md, independent adversarial probes confirm all R1/R2 fixes and no open confirmed defects.
- Completeness before publication: completeness-r2.md, 90%; AC9 still requires new root/adapter modules after publication. ci-source-r2.json supersedes older external dependency refs from that report.

## Release decision and ordering

Use `make release-break`: a new public admission API and fail-closed rejection of unavailable continuation materially tighten the public contract. JSON-only helper signature remains unchanged; no compatibility bridge or application runtime was added.

The published baseline cannot compile the new admission API (published-baseline.json). Publication is therefore necessary to test AC9's new API using published artifacts. Implementation/error acceptance and all make gates precede publication; post-publication semantic tests and final independent acceptance must pass before issue closeout or goal completion. A prepared archive build is never counted as published semantic evidence.

## Limits

Local SDK HTTP fixtures require no credentials. Evidence covers actual consumer public APIs and local host approval semantics, not live model support, production IAM/durable storage, cryptographic signature validity or remote-effect rollback. CI definition is checked through equivalent clean source fixtures locally; no hosted CI run is claimed.

## Publication verification

`make release-break` completed successfully (release-break-r2.log). The first attempt stopped during SSH-only ref reading before preparation; transaction remained the prior published release, no new refs existed. HTTPS was used through process-local Git URL/credential configuration for the retry; remote configuration and tokens were not changed or printed.

Release v0.16.0, source c2253dc71ccc531707cf3b2fbbb89bc859d5d751, published commit 4d028f87265f687580ab6614dfcf36bb6d125b45. All 13 expected remote refs independently matched that commit (remote-refs.json); transaction status is published (release-transaction.json).

Published-mode tests, lint and demo PASS (published-r1.json): GOWORK=off, no Replace entries, root/adapter v0.16.0, external tool executor v0.18.0, context v0.13.1 and lifetime v0.7.0. The new public admission API is tested through actual downloaded module artifacts. Post-publication independent acceptance reports will establish the final verdict before closeout.

## Final acceptance and closeout

Completeness-r3.md: 100% AC1–AC10, independently repeated published race/lint/demo (completeness-published-r3.json). Errors-r4.md: PASS, independent published adversarial/race tests, no open confirmed defects. Both final reports supersede the pre-publication verdicts.

Migration comment: https://github.com/skosovsky/prompty/issues/5#issuecomment-6035108139; posted body equals migration-comment.md (issue-comment-verification.json). Issue independently read as CLOSED after close command, closedAt 2026-10-07T09:31:43Z (issue-closeout.json). AC11–AC12 satisfied. No source/runtime changes occurred after release.
