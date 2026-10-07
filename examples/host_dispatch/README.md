# Caller-owned full dispatch

This optional module runs without credentials or paid requests. It imports the
real tool executor, semantic context and resource lifetime consumers. Core has
no dependency on them. See [the contract](../../docs/host-tool-dispatch.md).

From the repository root, run the offline demo using the declared workspace:

```sh
go run ./examples/host_dispatch/cmd/demo
```

It makes a checked SDK request to a local HTTP fixture, stores/restores the full
response through a registered opaque context codec, admits the whole tool batch,
passes native IDs/raw JSON to the real caller session and sends the correlated
results in the next checked request. The exact integer is retained as raw JSON.

`Admit` owns the execution snapshot; `Batch.Run` is called by one goroutine.
`Batch.Pending` and `Outcomes` retain host evidence. Only `Batch.Execution` after
all calls complete is eligible for continuation. Explicitly resolve a control
with your authorization gate before invoking `ResumeApproval`. Infrastructure or
unknown outcomes require external reconciliation; this recipe rejects retry.
The sample pending approval uses an in-memory host gate, not authenticated IAM.

Business failures are checked even with nil Go error. Model projection supports
JSON/plain text for model audience, with constant safe business-error text.
Private/user audience, binary and unimplemented shapes fail with `ErrProjection`.
Effects/progress/controls remain host evidence. OpenAI Chat rejects error results
because it cannot preserve the error flag; the test verifies zero HTTP dispatch.

`SaveResponse`/`RestoreResponse` implement an opaque storage profile preserving
all canonical wire fields. It is not a semantic budgeting adapter; the host
retains preceding history separately. `ToContext`/`FromContext` are a narrower
semantic projection with explicit unsupported cases. Use `ContextCodec()` when
persisting semantic messages: its registered host sidecar retains original raw
argument strings, checks them against the semantic JSON before restoration, and
rejects stale or unknown metadata rather than replacing changed arguments. URL images with MIME or
bytes cannot map into the consumer's URL-only image shape; inline images retain
MIME/bytes. Rich state and annotations use opaque storage, not text flattening.

For streaming, create `OwnStream`, run `Consume` on your chosen caller goroutine,
and close the stream from callbacks. `Lifetime.Close` cancels and joins only from
outside consumption. Even a closed-before-start handle is consumed once so its
completion signal closes. It starts no goroutine itself. Retain resource permits
until the joining cleanup finishes.

## Dependency matrix

Use the same semantic tests with real dependencies in two modes:

```sh
python3 scripts/host_dispatch_checks.py local \
  --source toolsy=../toolsy --source contexty=../contexty --source routery=../routery \
  --run --report /tmp/host-local.json
python3 scripts/host_dispatch_checks.py published --run --report /tmp/host-published.json
```

The local mode creates a temporary workspace with explicit source paths. The
published mode copies the consumer into a temporary directory, disables the
workspace and all replacements, resolves a published root/adapter pair (latest
by default, or `--ref`), and retains the pinned external consumer set in go.mod.
CI source checkouts are pinned to the commits behind the declared external
consumer releases. Their default branches currently precede those contracts;
checking default branches instead fails explicitly. Source mode also accepts
current local working trees and records their HEAD and uncommitted changes;
passing those trees does not prove that remote default branches pass.

Both modes run `-race -count=1`, lint and optionally the demo, and record actual
module refs and commands. Neither mode edits your go.mod/workspace files.

A published root without `AdmitToolCalls` fails compilation explicitly. Before
its first publication, the release transaction can validate the prepared module
archives offline; that gate does not constitute published-mode evidence. Run the
published mode after publication before closing the issue.

Offline fixtures establish contracts, not live model support, signatures,
production storage durability, authenticated approvals or remote rollback.
