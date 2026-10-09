# Repository verification

Make invokes standard Go commands and golangci-lint directly. Modules are discovered
from go.mod files, excluding hidden directories and vendor. All commands use
GOWORK=off. There is no aggregate check target or tool version validation target.

| Command | Scope |
|---|---|
| `make modules` | List all discovered development modules. |
| `make test` | Fresh ordinary race tests across all modules. |
| `make test-integration` | Files with integration build tag; execute TestIntegration… functions only. |
| `make test-e2e` | Files with e2e build tag; execute TestE2E… functions only. |
| `make test-live` | Files with live build tag; execute TestLive… functions only, including paid calls. |
| `make lint` | Formatting diff and lint without rewriting files. |
| `make fix` | Go fix, formatting and lint fixes; modifies files. |
| `make fuzz` | Every discovered fuzz function separately, 30 seconds per function. |
| `make bench` / `make cover` | Benchmarks / per-module coverage. |

Build tags alone do not exclude ordinary test files. Profile targets combine the tag
with a matching test-name prefix so a module without such tests executes none.
Use the same convention for new tests; each profile runs directly through Go too:

```sh
GOWORK=off go test -race -tags=integration -run '^TestIntegration' ./...
GOWORK=off go test -race -tags=e2e -run '^TestE2E' ./...
```

Recipes use tools from PATH and explicitly propagate command failures.
Tool versions are pinned in CI, not enforced by Make. CI and source release gates
run lint, fresh unit tests, integration and e2e sequentially.

## Project content

`examples/` contains runnable consumer examples. All 12 discovered modules are
checked and published, including examples, provider adapters, `cmd/prompty-gen`,
`ext/otelprompty` and `remoteregistry/git`. No separate publication inventory exists.

Integration checks cover template helpers, YAML parsing, HTTP/Git registry composition, provider runtime descriptors
and token estimators. Infrastructure tests run the release protocol against temporary
local bare repositories with lightweight fixture gates. E2E checks cover composed
manifest execution/checkpoints, provider cache policies and code generation.
The codegen e2e profile explicitly executes `cmd/prompty-gen/testdata/gen_out`,
which Go's recursive package discovery otherwise excludes.

All profiles require Go 1.27.2, Git, Make and Bash; race tests require CGO and a C compiler.
Missing prerequisites fail selected checks. Python, PDF runtimes, Docker and provider
credentials are not prerequisites for unit/integration/e2e tests. Docker is used only
for optional local Linux verification on macOS. Paid runnable examples require their
provider credentials and are executed manually. No live-provider tests currently exist.

Performance measurements, fuzz campaigns and paid provider calls remain separate.
