# Task43 checks r1

Frozen acceptance snapshot: snapshot-r1.json, 800 files, SHA-256 `5627f70f37aca76f3b1e46791c26cd0a94e99a49b6f5d5aa6f8d9bbd47cc74c6`, base `132afc2`.

- `make test`: exit 0, all 12 modules and generated package, race enabled. `checks/final-test-r1.log`.
- `make lint`: exit 0, all 12 modules. `checks/final-lint-r1.log`.
- Final Anthropic module `go test -race ./...` and lint: exit 0, after last private block hardening. `checks/final-anthropic-test-r1.log`, `checks/final-anthropic-lint-r1.log`.
- OpenAI full module race/lint: exit 0, `checks/responses-conformance-r1.log`, `checks/responses-conformance-lint-r1.log`.
- Core hosted-only failed continuation race/lint: exit 0, `checks/core-scoped-stream-r1.log`, `checks/core-scoped-stream-lint-r2.log`.
- Hosted batch/example race: exit 0, `checks/hosted-batch-example-r3.log`.
- `git diff --check`: exit 0.

Local SDK fixtures do not assert live account/model conformance. Independent completeness/error acceptance is pending; no commit yet.
