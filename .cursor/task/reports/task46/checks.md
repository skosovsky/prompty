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
