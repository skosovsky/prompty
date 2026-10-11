# Logical operation tracing

`WithTracing` wraps a checked Invoker. Configure `WithTracerProvider(provider)` or
`WithTracer(tracer)` with caller-owned OTel instances; no process-wide singleton
is required. `WithTracerName` chooses the instrumentation name. With no injected
tracer/provider, the existing OTel global provider is used. OTel stays in this
extension module; core DTOs have no telemetry dependency.

Tracing retains the shared ToolScope validation as defense in depth before
delegation, in sync and stream modes. It validates the same portable scope contract
as checked execution, without authenticating scope or defining an alternate policy.

## Contract and conventions

The GenAI conventions are **Development**, pinned to official snapshot
[`4f85037ef86e92c510d2ef881a58f1076f6fc0e4`](https://github.com/open-telemetry/semantic-conventions-genai/tree/4f85037ef86e92c510d2ef881a58f1076f6fc0e4).
See [client inference](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md)
and [operation spans](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md).
The extension implements the documented subset, not every convention or future
revision. Inference uses standard operation/provider/request model/response model,
finish reasons and input/output/cache-read/cache-write/reasoning token attributes.
Observed `Response.Model` is distinct from the selected request/profile model.
Only counters in `Usage.Known` are emitted: known zero survives, unknown estimates
are omitted. `prompty.usage.total_tokens` is an extension; cache/reasoning subsets
are never added again. No pricing catalog or monetary cost is calculated.

A span represents one caller-visible logical operation, not an HTTP attempt.
Nested SDK transport spans remain separate. The extension observes no retry count
and exports no token metrics of its own. Applying the same middleware twice is
caller instrumentation twice; wrap each port once to avoid duplicate observations.

Sync responses are recorded even when returned with capture/schema/other errors.
Machine categories distinguish preflight, schema/capture, transport where known,
provider control outcomes and interruption. Unclassified errors stay operation
failures: no fabricated transport/dispatch evidence. Refusal/incomplete/paused
metadata does not become a network error automatically. Default span status and
attributes use categories, never arbitrary `error.Error()` text.

Synchronous invocation and resource wrappers end their logical span on caller-port
panic and re-panic with the original value. They record only the safe category
`caller_panic`, with no panic payload. This cleanup is not error-return recovery;
injected telemetry callbacks/exporters must themselves return normally.

## Lazy streaming

The observer attaches to the original stream handle through `ObserveLifecycle`.
Span creation occurs at consumption, and closeout ends it once. Unused handles and
Close before consumption create no fictitious provider duration. Consumer stop,
Close during reading and parent cancellation retain available output/usage/model
and have distinct `StreamStatus.StopReason` metadata. Close after terminal keeps
final status immutable. No producer goroutine or second Events loop is added.

A RemoteResponseStreamer must transfer a fresh lazy handle whose lifecycle can
accept observation. If registration fails on an already consumed/malformed handle,
the wrapper closes that transferred original to cancel owned work, returns a fresh
failure stream and preserves RemoteResult for reconciliation. It never starts a
second provider iterator or rewrites the original terminal status. Sharing that
same handle with another consumer violates the port contract; the caller owns
reconciliation of any operation already started by such a malformed port.

## Explicit resource operations

`WrapRemoteResponses` observes Create/Get/Cancel; `WrapResponseStreaming` observes
native foreground CreateStream. `WrapBatch` observes Submit/Get/Cancel/Results;
`WrapCache` observes Create/Get/UpdateExpiry/Delete. These wrappers implement the
original ports and return original results/errors. They do not expose an optional
port that the caller did not provide or add polls, retries and automatic resume.

Generation usage is recorded on explicit Responses Create/stream operations.
Get/Cancel retain available response model/outcome/finish alongside resource
metadata, without generation token attributes. Batch/cache record job/resource
metadata, not generation token attributes. Fetching the same result again therefore does
not count its model usage again. Batch job state differs from per-item outcome;
cache `TokenCount` is resource size, not invocation billing. Item results and raw
resource observations are available to explicit host projection; no item execution
or synthetic generation spans are invented from a downloaded file.

## Privacy and projection

Default tracing omits prompts, prompt IDs, tools/arguments/results, reasoning,
opaque state, credentials, request URLs, account refs, resource IDs and raw error
text. It does not claim to be a general secret scanner. Host owns exporter,
projection policy and retention; model/provider/finish metadata is still metadata
that should match the host's egress policy.

`WithProjection(func(ctx context.Context, observation Observation)
[]attribute.KeyValue)` explicitly selects/redacts additions. Observation includes
owned execution/response/resource metadata and the error for caller classification.
The error preserves its identity; host projections must not mutate error objects.
Returned attributes are exported as supplied. The extension does not auto-record
payloads or exception events; a projection can intentionally export sensitive data
and must be reviewed by the host. Projection cannot mutate original results via
its owned snapshot. See in-memory exporter tests and runnable Examples.

## Migration

Legacy `prompty.prompt_id`, `prompty.model`, `prompty.tokens_total` and raw exception
message/status export have been replaced by the pinned attributes and safe
categories. Update dashboards explicitly; no compatibility aliases are emitted.
`RemoteResponseStream` now exposes lifecycle observation, and portable responses
carry optional observed model. For the other breaking changes see the root
[migration guide](../../docs/migration.md) and [capability matrix](../../docs/provider-capability-matrix.md).
