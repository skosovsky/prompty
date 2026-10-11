# v0.16 recovery

The recovery merges the infrastructure line at `4fe0d0272412bfc11975387b1b6a392f7823ebf9`
with the release source and closeout line at `57a515dd302db3a8b1d5bf5fc5a566e5c4c2f3d8`.
The latter contains the v0.16.0 source `c2253dc71ccc531707cf3b2fbbb89bc859d5d751`.
The published v0.16.0 commit remains `4d028f87265f687580ab6614dfcf36bb6d125b45`;
the v0.16.1 commit remains `17c61fddd4337be7fcc57597a5f1dbd258231712`.

## Contract

The public API and runtime behavior follow v0.16.0, including capture, stream
lifecycle, schema fidelity, caller-owned tool admission and host dispatch.
Compatibility with the regressed v0.16.1 API is intentionally not retained.
Consumer migration is a separate task. `retract v0.16.1` is declared in the twelve
modules that published that version. `examples/host_dispatch` restores the
thirteenth module; it has no v0.16.1 tag to retract.

Go 1.27.2, golangci-lint 2.14.0, explicit zero initialization and compatible helper
extractions from main are retained. Restored protocol state machines use local,
explained lint exceptions; the shared linter configuration is unchanged.
Makefile, `.github/workflows/ci.yml` and `scripts/release.sh` are byte-identical to
4fe0d02. The superseded Python release backend is removed. No release rollback
guard or additional candidate-verification phase is introduced.

## Verification before publication

- `make lint`: all thirteen modules pass.
- `make test`: fresh race tests pass in all thirteen modules. The last Ollama
  helper extraction also passes a separate fresh race run.
- `make test-integration` and `make test-e2e`: all thirteen modules pass.
- `make fuzz`: discovery succeeds across all modules; no fuzz targets exist.
- Generator regeneration produces no change to tracked generated fixtures.
  Generated consumer race tests and lint pass explicitly.
- Workspace inventory: three disposable-repository tests pass; thirteen modules,
  Go 1.27.2 and twelve retractions are checked.
- `golang.org/x/exp/apidiff` compares root, four adapters, generator,
  telemetry and git registry against both published tags. No incompatible API
  changes from v0.16.0 are reported. The accepted v0.16.1 break comprises 72
  reported changes across those module comparisons.

The existing release script repeats its source gates in an isolated checkout.
Publication must be followed by exact remote-ref verification and a downloaded
host-dispatch consumer run with GOWORK=off and no replacement modules. These
post-publication checks are recorded separately and are not claimed above.

## Preserved histories

A complete verified pre-recovery bundle is stored outside the repository at
`/private/tmp/prompty-recovery-20261011/pre-recovery.bundle`.
Keep `codex/recovery-v0.16` and `codex/release-v0.16-source` for comparison, as
requested after the original cleanup plan. Existing release tags are immutable.
