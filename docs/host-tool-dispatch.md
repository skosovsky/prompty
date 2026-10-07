# Host tool dispatch

`ExecuteWithTypedTools` is a JSON-only one-shot convenience helper. Its handler
port receives name and raw arguments, not native call ID. Ordinary handler errors
produce error results and continue the admitted batch. Cancellation stops later
calls. An approval/pause error from a caller is an ordinary handler error here:
error identity survives, but this helper does not enforce a lifecycle policy.

Use a full `Response` for ID-aware execution, business outcomes, approval and
continuation. [The optional executable recipe](../examples/host_dispatch/README.md)
owns those decisions in the application, with actual tool, context and lifetime
consumers. Core acquires no dependencies on these consumers.

## Pure admission contract

`AdmitToolCalls(ctx, execution, response, validator)` returns owned, complete
`[]ToolCallPart` in source order, or no admitted calls and an error. It never calls
a provider or tool handler. Inputs are snapshotted; callers must not mutate them
concurrently. `ToolValidator` must be pure and concurrency-safe when shared.

Before returning calls it checks context, the retained execution scope, original
schema compilation, response metadata/outcome, continuation availability,
correlation (including confirmed name-only correlation), complete object arguments,
and all execution schemas plus caller validation. Even the last invalid call
rejects the whole batch. Compiler isolation follows the existing schema contract.
Provider-lowered schemas must never substitute for original execution schemas.
An empty completed response admits an empty batch. Refusal, incomplete and paused
provider outcomes are rejected; provider pause differs from host tool approval.

Response scope/metadata/outcome failures retain a cloned response in `ToolDispatchError`.
Argument failures use `ToolCallError` with original causes, a cloned response and
validation feedback. Feedback is diagnostic only; it is not approval to send error
strings to a model. Context causes remain discoverable via `errors.Is`. Admission
is not authentication and cannot roll back effects or prevent host-side races.
Hosts revalidate admission when resuming against changed policy or execution.

## Dispatcher policy used by the recipe

The application preserves native ID/name/raw arguments in each caller request.
It admits the full batch before execution, saves the full assistant message once,
and processes results sequentially. Its pending cursor advances only after a
terminal outcome. On a control or infrastructure error it retains pending ID,
partial outcomes and original error identity, and stops later effects. Approval
is an explicit host decision and `ResumeApproval` retains the same ID; completed calls are
not replayed. This in-memory recipe is not production authentication or durability.

Business errors with nil Go error become safe error results. Internal/user audience
payloads never become model text; this example rejects them explicitly and retains
the host outcome. Controls/effects/progress stay host-owned. JSON/text model payloads
are supported; binary delivery and unknown MIME/audience/shape fail explicitly.
Hosts may implement broader profiles, rather than silently flattening payloads.

## Supported mapping

The recipe's context projection follows the narrow semantic subset described by
[the context consumer](https://github.com/skosovsky/contexty/blob/3ce901169b09b7136b126d9baa98e1875b33fa82/docs/supported-mapping.md).
The recipe intentionally supports a smaller semantic subset than that host
profile: scoped annotations/provenance and rich media use the opaque checkpoint
or fail explicitly; they are not claimed as semantic mappings here.
It stores canonical response wire as a registered opaque host payload, with the
same codec identity and destination profile on restore. This checkpoint carrier
is distinct from a semantic context projection and is never flattened to text.

| Canonical field | Caller representation / reverse mapping | Unsupported / rejection | Evidence |
| --- | --- | --- | --- |
| Call ID/name/raw arguments | Caller ToolInput.CallID/ArgsJSON → ToolResultPart.ToolCallID/name; no float64 | duplicate IDs, unconfirmed/ambiguous ID-less calls | Admission and full dispatch tests |
| Original schema and ToolScope | retained execution snapshot and pure admission | unknown tool, forced mismatch, invalid last call, lowered schema substitution | Batch tests |
| Tool business error | ExecutionError → safe IsError text result | infrastructure failure stops; native destination without error mapping rejects | Outcome tests |
| Audience / controls / effects / progress | retained host outcome; no default model projection | private/user audience, binary, unknown MIME/delivery, invalid UTF-8/JSON | Projection tests |
| Text / URL image without MIME or inline image | typed semantic parts; exact MIME/bytes | image detail, URL MIME/bytes, non-image semantic media | Context subset tests |
| Tool call/result semantic context | raw string in registered host sidecar, checked against semantic JSON; text result with ID/name/IsError | argument chunks, explicit argument MIME, non-text/nested results | Context subset tests |
| Provider scope / state / expiry / Required | StatePreserve wire in registered OpaqueState payload | wrong destination, expiry, unknown codec, changed bindings | Opaque checkpoint tests |
| Part/message annotations and associations | complete canonical wire; original order and indices | invalid associations; unknown extension codec, stale raw-argument sidecar | Opaque checkpoint and semantic codec tests |
| Reasoning and richer media | complete canonical wire in opaque carrier | reasoning in semantic projection | Context subset and checkpoint tests |
| Continuation unavailable | preserved marker | unavailable/logging projection cannot be replayed | Checkpoint/admission tests |

Semantic projection rejects state and annotations unless retained in the opaque
checkpoint profile. It rejects native cache controls and provenance rather than
inventing equivalents. The provider adapter still validates destination codecs
and supported content before the next request. Opaque storage is not proof that a
provider accepts every stored shape or that a signature is valid.

## Stream lifecycle ownership

One caller owns `Events()`. `Stream.Close()` is callback-safe, concurrent and
nonblocking: it requests cancellation, not cleanup completion. The caller closes
a completion channel only after consumption and its unwind finish. A separate
resource owner can join that channel in lifetime cleanup, retaining its permit
until cleanup completes. A callback calls `stream.Close()` directly; it must not
call the joining lifetime cleanup or wait for its own consumption to exit.

Close before consumption avoids provider dispatch. Cancellation is cooperative;
callback/transport work must honor it. No polling of Status, hidden producer,
retry, rollback or remote-rejection guarantee is introduced.

## Migration

For JSON-only one-shot callers, keep `ExecuteWithTypedTools`. For native ID or
approval, use checked `Invoker.Execute`, pure `AdmitToolCalls`, then a caller-owned
dispatcher as demonstrated by the recipe. Inspect business outcomes even when the
Go error is nil. Replace history built from `TextContent` with `AssistantMessage()`
and explicit `StatePreserve` persistence. Use consumption completion to join stream
cleanup. The recipe provides an offline contract test, not live provider, IAM or
durable-run certification.
