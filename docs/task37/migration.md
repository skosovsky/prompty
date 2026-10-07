> Historical task37 evidence; superseded by [current contracts](../README.md) and [migration](../migration.md). Original audit content below is retained; old API snippets and test counts are not current acceptance.

# Task37 migration — implementation in progress

## Whole-request cache settings

Use execution.RequestCache for whole-request OpenAI key/retention settings,
not a message CachePolicy and not CacheResource. Select CacheImplicitPrefix,
provider `openai` for Chat or `openai.responses` for Responses, and JSON Controls
with `prompt_cache_key` and/or `prompt_cache_retention`. Retention accepts only
`in-memory` or `24h`; this is request intent, not a durability guarantee.
Declare CapabilityCacheImplicit and each selected `cache_control:<key>` in the
actual endpoint/model profile. Do not infer support from SDK field availability.
Choose CacheRequired to reject unknown/unsupported settings before dispatch;
CacheAdvisory permits omission and requires inspecting cache dispositions.
Neither `bypass` nor `ttl` is mapped by these OpenAI request ports. Do not treat
request omission as deleting a remote resource. Responses Get/Cancel do not
inherit generation cache settings. Normalize/Clone/wire preserve independent
request intent and CacheResource binding; no migration bridge repairs old hints.
`ExampleAdapter_requestCache` is executable against a local SDK fixture. Chat
sync/native/buffered and Responses Create fixtures verify outgoing settings,
required/advisory handling and usage composition; this is not live model support.
Final independent audits remain required.

## Gemini signed incremental continuation

Gemini signed partial calls now preserve required state using the
separate gemini.partial_parts.json codec. Retain the complete Response/ChatMessage
ProviderState and source-part Annotations through approved wire/history, not just
assembled ToolCallPart.Args. Ordinals restore global order across interleaved calls;
signed payloads remain only in opaque state. Missing/duplicate/edited provenance
is rejected before dispatch, with no fallback to canonical call order.
Rebind the same destination and a Vertex SDK client for continuation. The adapter
validates the projection, then restores original fragments/signature positions;
do not synthesize a merged signed SDK call. Incomplete/canceled sequences are not
continuable. The new codec requires original ordinals; old payloads are not repaired.
Expanded conformance/final audits remain required for task acceptance.

## Explicit batch execution

Gemini NewBatches now supplies the native Developer API inline BatchManager.
Declare each batch capability in your endpoint/model profile, give each item an
explicit AttemptID and preserve original executions alongside EncodeBatchHandle.
Results requires matching rebound executions and validates their digests before
Get. Use metadata-key correlation and inspect both download/correlation completeness
and each item outcome; never zip outputs by position or treat cancel acknowledgement
as terminal cancellation. Native output/source/schema/usage reuse existing checked
contracts, including original JSON Number content preservation. Caller owns polling,
retry lineage and persistence. Expanded conformance remains required; this inline
checkpoint does not certify live account/model support.

For Gemini file-backed requests select NewFileBatches explicitly and inject a
BatchFileTransport with matching gemini/endpoint/account binding. No fallback to
inline and no file store are installed. Submit validates all items before upload,
then creates a job using the returned input reference. Preserve known Resources
even if job creation fails; SubmissionAttempted distinguishes the job attempt
from upload. Unknown acceptance never authorizes blind retry. Inspect to obtain
the provider output reference, preserve it in the handle and rebind originals
before Results. Configure input/line bounds with BatchFileOptions and download
bounds with BatchResultsOptions; the operation closes the returned body once.
Read HTTP/read/close failures, download completeness and correlation completeness
separately from each item's outcome. ExampleNewFileBatches uses local fixtures;
host cleanup/auth/retention policy and unverified live support remain explicit.

Batch Results now prepares only the original local validation contract and the
retrieval capability. Original cache/state expiry or unavailable generation options
do not require replay/renewal and cannot hide already-produced results. Keep the
original execution unchanged for digest matching; do not strip state/cache fields
to bypass validation. Source schemas, declared tool names and ForcedTool remain
enforced without reapplying provider lowering. Submission still checks all live
generation constraints. Custom adapter integrations replace ParseBatchItem with
PrepareBatchResultItem/ParseBatchResultItem for retrieval; no legacy alias remains.

OpenAI NewBatches uses file-backed native Batch for Chat Completions. Inject
NewBatchFiles with the same explicit endpoint/account and SDK client or supply a
compatible host BatchFileTransport. All items must use one model; profiles declare
batch capabilities separately. File upload is a real independent provider effect.
On failure inspect Resources and SubmissionAttempted: a known file without a job
is not an unknown submitted batch. ErrBatchFileUnknown requires file reconciliation,
ErrBatchUnknown requires job reconciliation. No automatic retry/delete occurs.
Host handles cleanup/retention and original input persistence.

Preserve resource references and ProviderStatus through batch handle wire. Results
reads both error/output files, not just successful outputs, and validates rebound
input digests before download. Keep per-item SourceJSON/Outcome/ErrorKind/Response;
batch canceled status does not fabricate canceled outcomes for missing IDs. Limits
apply per line and across both files; read failures retain prefix and known remote
status. Semantic capture has a pre-upload plan and post-upload actual SDK request;
capture failure between them leaves a known file, not a created batch. Runnable
local SDK fixture is not live conformance. Other Batch endpoints are not claimed
as implemented by this Chat Completions port; full task scope remains authoritative.

Anthropic NewMessageBatches exposes Submit/Get/Cancel/Results with independent
batch capabilities on each actual endpoint/model profile. BatchItem requires a
unique ID and caller-owned AttemptID. OriginalID defaults to ID; retries supplied
by host use a new AttemptID and retain OriginalID/ParentAttemptID, never restart
automatically. Do not assume result order or ended status means all items succeeded.

Persist EncodeBatchHandle output, rebind transport/account, retain original
executions and validators. Results takes matching BatchResultBinding entries;
changed inputs/schema/model fail before download. Prefer explicit effective models
before Submit; after restore, use each item's bound model rather than a changed
adapter default. Runtime validators and operation CaptureBinding are not persisted.
The handle stores digests/lineage, not original documents or an application store.

Inspect each item's Outcome/ErrorKind/Response/SourceJSON. Batch completeness has
separate DownloadComplete/CorrelationComplete and lists missing/duplicate/unexpected
IDs; all observed duplicate/unexpected records remain visible. ErrBatchResults
retains accepted prefix items and the previously confirmed remote status. Distinguish
ErrBatchRejected from ErrBatchUnknown; uncertain acceptance never authorizes blind
Submit. Cancellation can race completion and does not roll back earlier items.

Capture is optional and operation-level; existing per-item capture intent is
explicitly rejected, not silently ignored. Default recording is metadata-only and
post-dispatch capture errors retain known handles/results. Results bounds each line
and total input with MaxLineBytes/MaxBytes; zero uses documented defaults. Results
uses its own original-line reader over the native SDK dispatch, preserving EOF/read
failure semantics the SDK JSONL wrapper does not expose. Local fixtures/example
are evidence only for this checkpoint. Full expanded conformance/final
independent audits remain required.

## Explicit Responses operation handles

Handle operations no longer apply generation ModelOptionsSchema to fabricated
empty options. Existing Responses Get/Cancel, batch inspect/cancel/results and
Gemini cache handle operations remain callable after acceptance even if generation
requires max_tokens. Do not strip the profile's schema or invent dummy settings:
its full snapshot and operation capabilities/combinations remain checked. Custom
operation adapters can use adapter.PreflightOperation with explicit model,
capabilities and scoped override; this report does not substitute for an actual
operation port or handle/result validation. Generation Create/Submit retain full
Preflight; original schemas must still be rebound when inspecting typed results.
Cache content creation uses PreflightInputOperation: generation settings/output
intent are rejected, while tools/state/MIME and schema mapping remain checked.
Do not pass dummy max_tokens to cache Create to satisfy a generation-only schema.

For local functions, pass canonical Tools and optionally ForcedTool; enable
tools/forced_tool_selection in the effective profile. Named choice is native,
not a system instruction. Original argument schemas are checked after receiving
completed function calls, with strict=false explicitly sent. Inspect the returned
error/outcome before execution; the Responses adapter does not run handlers.
Get/Cancel must rebind original Tools in RemoteResponseOperationOptions against
the handle's ToolSchemaDigests/ForcedTool. Changed/missing schema bindings fail
before I/O. Schema bindings now include normalized dialect, role and name with
exact source bytes. Source-only historical binding hashes fail rebinding, with
no fallback; rebind the original definition, not just its JSON.
Persist original Content and ProviderState together: call_id differs
from the native item ID retained in opaque state. Replay completed function items
using assistant Content plus required ProviderState; supply RoleTool ToolResultPart
with its exact ToolCallID. Alternatively send only results with Previous: the
scoped handle's ToolCalls records completed validated call IDs and names, without
arguments or handlers. Name may be omitted, but if supplied must match. Duplicate,
foreign, premature and conflicting references fail before I/O. Text results keep
their native array boundaries, including image URL/bytes and document URL/inline
PDF under InputMIME gates. No local fetch/upload is introduced. Audio/video/cache/
state decorations remain unsupported. IsError=true is rejected: no native error
flag can preserve it. For a model-visible error payload, choose your own explicit
text/JSON representation with IsError=false; no automatic wrapper. Expanded
conformance remains required. Instructions and current tools are explicit on every Create. See the
offline ExampleNewResponses_toolResults; there is no automatic agent loop.

For hosted stateless replay, use AssistantMessage without dropping annotations.
Keep public message/search items and the scoped output-order manifest together
with Content and required ProviderState. The manifest preserves native source
order and references private items only by canonical index. Original citations,
search results/coverage and message extensions remain in native items, not plain
text summaries. Edited canonical text/citations, wrong scope, duplicate IDs,
unfinished items or missing manifest fail before I/O. No legacy reconstruction;
no automatic inherited tools. ExampleNewResponses_hostedHistory is runnable.

Reasoning summaries now appear as ReasoningPart entries, not output text. Retain
their exact Content and ProviderState together for stateless assistant history;
encrypted payload is opaque and must not be replaced with summary text. Use
StatePreserve for continuation wire; StateOmit removes private state and marks
continuation unavailable. Edited content, missing/expired/foreign state, unfinished
items and unsupported metadata fail before dispatch. Create with an explicit
CapabilityProviderState requirement requests encrypted content; ErrStateUnavailable
can accompany a known answer if the provider omits it. The original output-schema
validator validates output text, not reasoning summaries. Portable handles now
require explicit provider_state_required, including false; regenerate old handles
from authoritative caller/provider state, never invent a guarantee during decode.
Get preserves that requirement and requests encrypted content after checking
provider_state capability. Cancel cannot request the native Get include; known
cancel results retain missing-state errors and the requirement for explicit later
inspection. A new Create with Previous does not inherit the requirement. Retrieval
schema validation binds the original digest without requiring current generation
schema support. Hosted history/local tool calls use the implemented mappings
described above; expanded conformance and final independent acceptance remain
required.

Responses now maps each user/assistant TextPart to an ordered native input_text
item instead of merging all text into one string. System/developer messages remain
explicit text instructions. User MediaPart supports profile-declared image MIME
types from URL/raw bytes and native document URLs or inline PDF data. Keep exactly
one source and matching MediaType/MIMEType; declare InputMIME in the actual
openai.responses profile. The adapter does not fetch/upload files or infer access.
URL policy/credentials and provider fetching remain host concerns. Inline PDF uses
a deterministic filename, not a storage path. Unsupported/ambiguous media fails
before I/O. Consumers inspecting SDK request capture must handle ordered input
content arrays. ExampleNewResponses_mediaInput is a local fixture, not live media
conformance. Required local tools/state/expanded conformance remain outstanding.

Use OpenAI NewResponses with openai.responses profiles and a host account reference;
Chat Completions profiles do not provide Responses capabilities. Call Create with
RemoteResponseRequest, explicit Store, optional Background and Previous handle.
Resend current instructions explicitly; no local settings/tool inheritance is
performed. Persist EncodeRemoteResponseHandle output, then restore through the
strict DecodeRemoteResponseHandle path and rebind transport/account separately.
Get/Cancel are explicit calls, not a worker. Distinguish remote status from model
outcome: failed/incomplete/refusal can retain partial content and known usage.
Cancel may return completed after a race; unknown acceptance never authorizes a
second Create. Availability errors do not erase the prior observed handle.

For structured Get/Cancel pass RemoteResponseOperationOptions.OutputSchema with
the exact original source schema; missing or different digest fails before I/O.
Validator/Binding are runtime ports, excluded from JSON/YAML. Create uses execution
capture intent; Get/Cancel use operation options. Accepted results survive required
post-dispatch CaptureError, while unacknowledged terminal capture remains incomplete.
Default capture omits payload, and source JSON is not byte/physical-attempt coverage.
Current text/native-schema/background/continuation APIs are implemented with local
SDK fixtures/examples. Set HostedTools to explicit HostedWebSearch/HostedFileSearch
requests and declare their independent capabilities. File search references an
existing host vector store; no upload/corpus operation is implied. Get restores
selected kinds from the handle and requests sources/results, but the next Create
sends only freshly selected tools (an explicit empty list when absent).
Read HostedSearchItems for ordered typed calls/results and original JSON from
MessageAnnotations. Preserve those annotations through wire/history. Results
missing/null/empty/unfinished are unknown/unavailable/available/partial, respectively;
coverage is visibility, not completeness of the searched web/corpus. Hosted calls
are not ToolCallParts and must not be registered as local handlers. Numeric filter
values and raw result attributes preserve source precision. Unknown/reserved or
duplicate setting keys fail before I/O. Richer input/state and expanded conformance
remain mandatory, not deferred. Store=false is not proof of no provider retention.

## Explicit remote cache reuse

Checked generation revalidates the prepared provider-state lease after capture
and any native StreamStart pause, even without a sink. Updating caller-owned
expiry after preparation does not extend the immutable request lease. Handle
ErrStateExpired as local rejection with zero generation dispatch, not unknown
remote acceptance. Batch submission performs the same check before upload/submit
and again before job creation after upload: retain returned Resources for an
already-uploaded file even when SubmissionAttempted is false and no job exists.
No state refresh, upload rollback, recache or retry is automatic. Results-only
reconciliation still validates original result contracts without renewing or
rechecking the input generation lease.

Cache content Create also rechecks signed input provider state after host capture
and immediately before dispatch. An envelope that expires during a callback now
returns ErrStateExpired, without HTTP or a cache handle. Handle this as a known
local failure, not unknown remote acceptance: the library does not renew state,
silently omit signed content, create a replacement cache or retry the operation.

Use Gemini NewCacheResources with a host account reference and injected transport/
profile for explicit Create/Get/UpdateExpiry/Delete. CacheResourceCreateRequest
reuses immutable canonical execution input and explicit CacheResourceExpiry.
Do not pass generation settings/output schemas/cache markers as cache config.
Persist only EncodeCacheResourceHandle/DecodeCacheResourceHandle output; rebind
credentials/transport/account and authorize access outside the library.

For generation set execution.CacheResource, WithCacheAccountBinding and required
CapabilityCacheReuse knowledge. Wrong endpoint/model/account, expired/deleted/
unknown state fails before I/O.
Expiry is rechecked after capture callbacks and a native StreamStart consumer
pause, immediately before dispatch. Handle expiry errors at that boundary mean
no provider call occurred; refresh/reconcile explicitly outside the library.
Absolute create/update expiry is also rechecked after capture. This is not a
remote lease: expiry during a provider call remains a remote outcome.
Base reuse does not override cached system/tools; provide the question without
conflicting configuration. No automatic recreate,
upload, renewal or cleanup worker runs. ErrCacheResourceUnknown is not permission
to retry a mutation; inspect an available handle or reconcile externally. A
resource TokenCount is presence-bearing size metadata, not invocation billing.
Generation usage remains a separate common Usage contract. Lifecycle capture
reuses execution.Capture/Requirements.Capture for Create; Get/UpdateExpiry/Delete
accept at most one optional CacheResourceOperationOptions containing Capture
intent and Binding. Runtime Binding is omitted from JSON/YAML. Rebind ports after
restore; host owns redaction/persistence. Capture is semantic input/SDK parameters/
bounded original JSON/result/one terminal, not HTTP bytes or physical attempts.
Known resource results survive post-dispatch CaptureError; never interpret it as
rollback or permission to retry. Expanded fault/cancellation conformance and
paired final audits still require completion.

## Explicit synthetic output selection

Synthetic sync/buffered capture uses the shared five-stage semantic contract,
including SDK build identity. Default projection omits payload; opt-in projection
can retain the private output-tool request and original provider tool-use JSON.
Canonical output is validated text, never a handler-eligible synthetic call.
Sync pre-dispatch sink failures now attempt a pre-dispatch terminal and close the
capture session, preserving the original error with zero provider calls. Buffered
closeout stays on its stream handle. Post-dispatch recording failure does not
mean execution failed or authorize retry; retain the known result/usage.

Native output remains the zero-value choice (or explicit OutputNative). Select
Anthropic NewSyntheticOutput plus Requirements.OutputMode = OutputSyntheticTool
and ResponseFormat.Name for received-result validation through one output tool.
Declare independent CapabilitySyntheticOutput, CapabilityTools and
CapabilityForcedToolSelection in the effective endpoint/model profile. Merely
declaring native_json_schema does not select this mode. Ordinary adapters reject
synthetic selection, even when an override claims support. Mode survives wire
and clone; runtime validator/transport/capture ports still need rebinding.

Use the checked client for original-schema validation. Raw ParseResponse only
extracts content and structural errors; it is not schema acceptance. The output
is JSON TextPart, never ToolCallPart; do not register an output handler. No real
tools/caller ForcedTool are permitted in the base profile. Manual enabled
thinking is rejected, not disabled. Handle SyntheticOutputError reasons,
schema errors and refusal/incomplete OutcomeError separately; retain known
partial response/usage on failure, do not blindly retry unknown acceptance.
Sync and explicit buffered mode are available; this separate adapter has no
native-stream port yet. Executable ExampleNewSyntheticOutput uses local SDK
fixtures; it does not certify a live endpoint/model combination.

This documents implemented APIs, not an approved release. Final completeness/error audits and remaining expanded-scope conformance are outstanding. Owner scope decisions are resolved: all five directions are mandatory. No remote/background/batch/cache-resource lifecycle is advertised before implementation. See the [release and closeout checklist](release-closeout.md).

OpenAI output-schema strict checks now also inspect tuple-form `items`, `additionalItems` and schema-form `dependencies`. A nested open object that formerly escaped traversal can now fail compilation with its source pointer. If object-closing is intentional, explicitly permit `openai.close-object` lowering; original result validation remains unchanged. Property-dependency name arrays and annotation values are not schema nodes. This does not establish live provider support for those keywords.

The same checks now descend into schema-valued `additionalProperties`, `unevaluatedProperties` and `unevaluatedItems`. Local references/definitions and exact numeric enums survive whole-schema nullable wrapping, but original missing/null semantics remain authoritative. Supplying reference resources enables local validator compilation only; it does not fetch schemas or upload dependencies through a provider SDK.

Inspect `*prompty.SchemaCompilationError` for bundled validator compilation failures. `errors.Is(err, prompty.ErrSchemaUnsupported)` still identifies pre-dispatch rejection; `Locations` supplies authoritative document/pointer pairs. Empty `Resource` means the supplied schema; nonempty identifies an explicit resource, not the provider endpoint. Empty `Pointer` denotes a known document root, whereas empty `Locations` means unknown position. Missing-resource/fragment/anchor refs now report actual engine-verified reachable origin paths, including escaped and transitive resource positions; the rejected compile never becomes a usable validator. Annotation/example/default refs and unreferenced definitions are not fabricated failing origins. Malformed JSON can still lack a source position. Engine/decoder causes remain accessible through the error chain; do not log arbitrary causes or caller-owned resource/pointer values without host policy. No fake root pointer or implicit network fetch is substituted.

## Schema dialect and validation

Handle `ErrSchemaJSONDecode` separately from `ErrSchemaMismatch`: invalid/truncated/multiple JSON documents are decode failures, while a valid document may violate the original constraints. The one-shot structured helper returns decode failures directly rather than wrapping them as repairable `ValidationError`; its executor does not append domain feedback for them. Incremental structured parsing still exposes `ErrStructuredStream` and original typed decoder causes. Known checked responses returned with post-dispatch errors remain available; never infer not-executed or retry automatically. See [schema conformance boundaries](schema-conformance.md).

Set `SchemaDefinition.Dialect` and `ToolDefinition.Dialect` explicitly when supplying a schema declaring `SchemaDialect7`. Empty continues to select `SchemaDialect2020`, and a conflicting `$schema` fails compilation before translation/dispatch. Execution wire persists each definition's `dialect`; rebound runtime validators must enforce that selection. YAML uses `response_format.dialect` beside `response_format.schema`, and `tools[].dialect` beside `parameters`. Output definitions reject non-string dialect rather than silently using the default. Source-schema validation remains separate from domain `Validate()`.

## Stream contract

Replace the old iterator/IsFinished API with an explicit handle and mode:

```go
stream := client.ExecuteStream(ctx, execution, prompty.StreamNative)
defer stream.Close()
for event, err := range stream.Events() {
    if err != nil {
        // Status still contains the partial result. Do not blindly retry.
        break
    }
    if event.Kind == prompty.StreamTerminal {
        // event.Result is final; a finish marker alone is not terminal.
    }
}
status := stream.Status() // also available after early consumer stop
```

Use `StreamBuffered` only when a synchronous call is explicitly acceptable. Handle creation is lazy; consumption is single-use. Buffered execution cannot satisfy a required native-stream capability. Use `AssembleToolCallParts` instead of the removed adjacency-based helper. Tool identity is not the function name.

Use `event.Identity.MessageID` to correlate events of one handle, and `event.Identity.PartIDs[i]` for its `Content[i]`. The same logical text part can span multiple deltas. MessageID is locally generated, not a provider request ID or physical attempt ID. In status and the terminal event, PartIDs aligns with `Result.Content`; text fragment references can repeat, while collapsed tool calls retain their original logical reference. Do not substitute fragment offsets or function names for identity. These references are runtime/capture correlation, not a new persisted response-wire identity.

Custom stream sources now must supply explicit references for every content entry:

```go
frame := &prompty.ResponseChunk{
    Identity: prompty.StreamIdentity{PartIDs: []string{"answer"}},
    Kind: prompty.StreamDelta,
    Content: []prompty.ContentPart{prompty.TextPart{Text: delta}},
}
```

Leave source MessageID empty; the handle assigns it. Empty control frames have no part references. Reuse the reference only for the same logical part/type/tool-call ID, keep references unique within each content frame, and never bind one tool ID to multiple logical references. Missing, blank, duplicate, foreign or conflicting references fail with `ErrStreamProtocol`; no compatibility guessing or adjacency fallback remains. Invalid frames do not update accepted content, usage or references.

`WithStreamCaptureObserver` now receives `(context.Context, prompty.StreamStatus, error)`, not a Response-only argument. Read `status.Result` and `status.Identity`. `WithStreamFrameObserver` receives a cloned accepted frame; failure preserves it and fails the stream without replay. Native and buffered canonical capture payloads include `stream_identity`, with one canonical terminal owned by the handle. Ordinary sync captures do not invent stream references.

Middleware must not feed one handle's already-normalized events into a new handle. Attach lazy observation using `stream.ObserveLifecycle`: return a context with added values and the same Done channel, plus a closeout function taking `StreamStatus`. Attachment after consumption returns `ErrStreamConsumed`; nil/detached bindings fail with `ErrStreamProtocol`. Tracing now keeps the original handle/finalizers/capture and retains a known sync response alongside its error. No tracing-specific type or dependency is introduced into core.

Custom tool-stream sources must bind each provider ID to a single tool index. OpenAI rejects an ID reused by another index instead of silently collapsing distinct calls. Inspect `adapter.ErrInvalidResponse` for native identity conflicts and `prompty.ErrIncompleteToolCallArgs` for assembler name/snapshot conflicts. Send `Args` for a complete snapshot or `ArgsChunk` for a delta, never both: even equal values now fail translation. Error diagnostics no longer expose the conflicting tool names.

Ollama now has a native Chat transport: use explicit StreamNative with a declared endpoint/model profile, not its old chunk-parser-only path. ParseStreamChunk now emits complete Args snapshots rather than ArgsChunk. Repeated snapshots are idempotent only with the same provider ID and content; ID-less calls stay separate, without fabricated IDs/SDK-index correlation. Tool results retain tool_name, and helper name correlation requires explicit checked capability and unique names. Sync/native mode selection no longer mutates the caller request. IA10 parent correction is described below: use the explicit number-preserving HTTP operation port or reject unverified SDK floats. Final independent acceptance remains pending.

Native OpenAI frames must carry an explicit choice index zero and explicit nonnegative indices for all tool deltas. Omitted/null indices no longer inherit zero from SDK defaults. Fix custom endpoints/fixtures that omit correlation fields; do not fabricate indices from function names or adjacency. Continuations can still omit already-bound ID/name. Independent handles from one client keep separate correlation state even if the provider reuses an ID across requests.

Keep that argument representation consistent across every frame of the same call too. A complete `Args` snapshot followed by an `ArgsChunk` delta (or the reverse) now fails with `ErrIncompleteToolCallArgs`, even when the assembled string would be valid JSON. Identity-only frames do not select a mode; repeated equal snapshots remain idempotent. Sources must normalize their own provider protocol instead of relying on the assembler to guess a representation transition.

After breaking iteration, read `stream.Status()` for canceled partial content; do not expect another terminal yield. Cancel/Close release the SDK response body too. A source read failure after accepted deltas remains failed with the original error and partial content, not canceled due to cleanup. Neither native nor explicitly buffered execution replays a completion automatically; consuming a handle twice returns `ErrStreamConsumed`.

Structured helpers attach incremental parser and whole-document validation to that same lazy handle before consumption, composing with the prepared finalizer. Invalid JSON/separators or truncated documents now produce a failed lifecycle with `ErrStructuredStream`, not a consumer-canceled status; the received partial result remains available and source cleanup cannot deadlock terminal observation. Items already yielded remain provisional. Ordinary consumer break still cancels without another yield. A frame whose non-completed outcome is already known does not emit new JSON items; refusal/incomplete/tool-call outcomes are classified separately at finalization, retaining trailing usage. OpenAI propagates known refusal outcome from its delta, not only its finish marker. No legacy canceled-on-parse behavior is retained.

For streamed caller-owned `Validate()` failures, use `errors.As` to inspect `*ValidationError` and `errors.Is` for the original domain cause; these failures do not carry `ErrStructuredStream`. Decoder failures preserve their typed causes (for example `*json.SyntaxError`) underneath `ErrStructuredStream`. Neither error category triggers replay or repair automatically. Stream domain failures have no complete `RawAssistantMessage`; the handle retains the received partial result.

## Profile binding

`ForcedTool` now requires `prompty.CapabilityForcedToolSelection` in the effective checked profile separately from `CapabilityTools`. Custom adapters must also implement `adapter.ForcedToolAdapter` and truthfully preserve native named selection in Translate; text instructions are insufficient and overrides cannot add the mapping port. Gemini uses native ANY/allowedFunctionNames without modifying system messages. Ollama rejects ForcedTool rather than injecting a system prompt; use ordinary tools without that guarantee or choose a supported mapping. This is separate from the explicitly selected Anthropic NewSyntheticOutput adapter; ordinary ForcedTool does not enable synthetic output. Execution-scoped helper validation still checks returned calls before any handler.

Provider adapters expose endpoint/default model and accept caller-owned `WithProfiles(endpoint, profiles...)`. The supplied endpoint must describe the actual injected SDK client; this option does not reconfigure transport. Model overrides are checked against their effective destination. No provider-wide capabilities are inferred from the provider name. Each claimed profile needs provenance and conformance evidence; unknown required guarantees fail before dispatch.

`WithClient` alone no longer assigns the provider's default endpoint to an opaque SDK client. Pass `WithProfiles(actualEndpoint)` even for an ordinary checked request without capability knowledge, or include verified profiles when guarantees are required. Missing endpoint binding returns `adapter.ErrNoRuntimeDescriptor` before Translate/dispatch in sync/native/buffered paths; an execution override cannot supply the transport binding. Either option order is valid. Configure the SDK's URL separately from the same caller-owned value. Descriptors without a client retain only a nominal inspection default, not executable destination evidence.

An execution override supplies knowledge only. It cannot implement native streaming or provider-state translation on an adapter that has no corresponding port. Raw SDK Execute is a transport primitive, not a checked canonical execution.

Provide at most one descriptor profile per effective provider/endpoint/model. Duplicate matching entries now fail with `adapter.ErrInvalidProfile`, independently of slice order, rather than selecting the first. A matching explicit override replaces descriptor knowledge and is recorded in diagnostics. Selected malformed profiles fail with `prompty.ErrCapabilityProfile`; malformed overrides additionally carry `adapter.ErrInvalidProfileOverride`. Fix blank destination/provenance, invalid support states, blank capability/MIME entries, empty or duplicate-member combinations and non-schema model-options documents before dispatch. Extension capability names remain valid and unknown destinations remain unknown. Runtime and persisted override shapes use the same invariants; no legacy permissive profile recovery is retained.

## Response state, history and persistence

Capture coverage and failure handling are inventoried in [capture conformance](capture-conformance.md). Durable intent needs both advertised sink capability and confirmed acknowledgements. A failed ack after dispatch does not erase known execution; a transport failure can leave outcome unknown. Preserve both SDK and capture errors, and do not infer not-executed or retry automatically from capture failure.

Apply the [host redaction policy](host-redaction.md) before emitting logging/capture projections. `LoggingExecution`/`LoggingResponse` omit state, not secrets in arbitrary content or provider settings; use an application allowlist. Keep separately approved persistence isolated. A redacted capture must remain incomplete and cannot claim byte-exact replay.

Use a stable credential-free `ProfileIdentity.Endpoint`: URL userinfo, query/empty query marker, fragment and malformed URI are now rejected in profiles, checked descriptors, state scopes, schema compilation and response diagnostics/wire. Keep auth and request-specific URL parameters in the injected SDK/transport, not in the portable identity. Rebuild old persisted scopes explicitly; the library neither strips authentication components nor guesses an equivalent destination. Opaque references, URI paths and arbitrary canonical payload still require host redaction policy.

See [usage mapping inventory](usage-mappings.md) for each module's presence and total composition. OpenAI counter conversion now also returns `ErrUsage` before narrowing an invalid int64. Valid sibling counters and the known response survive; native streaming retains the accepted frame and closes transport on failure. Do not infer a cache miss or zero reasoning usage from an absent breakdown.

Valid JSON is not sufficient for structured success: incomplete/refusal produces `OutcomeError`, not a repairable `ValidationError`. Inspect outcome and finish reason before applying JSON-repair feedback or retry policy. The shared structured helper checks this for caller-owned invokers too, independently of provider constrained-output support. Native checked schema execution keeps trailing usage and the known response in failed terminal/status.

Derived usage overflow now returns `prompty.ErrUsage` with a non-nil known response. Preserve that response when handling an error; its provider outcome/content and individually representable counters remain available, while an unrepresentable prompt/total is not marked known. Do not retry automatically. Checked adapters also preserve partial results returned by a parser alongside an error. Buffered streams retain this response in failed status; sync and buffered capture emit one failed terminal carrying known outcome/usage.

OpenAI refusal now has `OutcomeRefusal`, independent of `finish_reason=stop`. Refusal text is a distinct text part with `openai.refusal` annotation, including an explicitly present empty refusal. Citation annotations retain full provider JSON as `openai.annotation` on the content text part. Do not treat every text part as successful output: checked schema execution rejects refusal with `ErrResponseOutcome`, retaining the known result and usage without retry. Empty truncated/filtered responses retain incomplete outcome and available usage instead of becoming `ErrEmptyResponse`. Multiple/nonzero choices and annotations without content are rejected rather than silently flattened or rebound.

Do not build history from `response.Content` alone. Use the response projection so signatures and annotations stay associated with their parts:

```go
message, err := response.AssistantMessage()
if err != nil { return err }
execution = execution.AddMessage(message)
```

For persistence, explicitly select state preservation:

```go
wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{
    State: prompty.StatePreserve,
})
executionWire, err := prompty.MarshalExecution(execution, prompty.WirePolicy{
    State: prompty.StatePreserve,
})
```

The old `MarshalExecution(execution)` signature is removed. `DecodeResponseJSON` and `DecodeExecutionJSON` validate the complete serialized schemas plus semantic associations. Typed `UnmarshalResponse`/`UnmarshalExecution` do the same validation. Missing/foreign format markers are explicit migration errors; old saved payloads are not repaired or decoded through a legacy bridge.

`LoggingResponse` and `LoggingExecution` omit opaque state. `WirePolicy{State: StateOmit}` does the same explicitly. Omission of required state marks continuation unavailable, so this projection cannot silently become a continuation checkpoint. Canonical JSON/YAML serialization excludes opaque state; use the explicit wire projection for persistence. Canonical text, media URLs and annotations still require the host's redaction policy.

Restore does not rebind runtime ports. Rebind validator/transport/profile knowledge as required before calling the provider. Signed content cannot be edited while reusing its state. A foreign/expired scope fails before transport. State-bound and annotated system messages are not normalized together.

Implemented opaque-state paths include Gemini signed parts and Anthropic signed/redacted thinking blocks. Raw translators that do not implement state preservation reject envelopes instead of silently dropping them. No additional native stream support is implied.

## Results and schema validation

Inspect `Outcome` rather than assuming that a JSON-looking response succeeded. Refusal, incomplete output and tool calls are distinct from completed structured output. `StreamStructuredOutput` accepts an explicit stream mode and emits provisional objects: success requires exhaustion, terminal completion and whole-document original-schema validation. An explicit stream schema describes the complete object/array; inference accepts one T object or an array of T objects.

OpenAI maps only the recognized `stop` finish reason to completed. Unknown or absent reasons remain incomplete, with the original finish reason/content preserved, rather than promoting unfamiliar provider behavior to success. In a native stream, EOF without a finish marker still fails the transport protocol separately.

The `SchemaValidator` port is separate from the domain DTO's `Validate()`. The standard implementation uses only explicitly supplied reference resources; it does not fetch remote schemas. The tool helper validates the whole batch against execution parameter schemas and scope before any handler. Global registry membership is not execution permission.

Use `Usage.Known` or `Usage.Availability()` before interpreting scalar token counts. Missing counters are not known zero. Cache/reasoning counters are breakdowns and are not added again to total. Gemini retains the provider total, whose composition can include thoughts and tool-use prompt tokens. Ollama total is derived from prompt plus evaluation counters only when both are known; SDK zero fields without presence remain unknown.

Call `Usage.Validate()` for caller-constructed metadata: counters must be nonnegative, Known must contain unique recognized counters, and Mode is empty/cumulative/delta only. A partial scalar estimate can remain unknown; do not promote it to Known just because it is nonzero. `ValidateResponseMetadata` additionally checks outcome and state/annotation associations. Checked sync returns a known response plus `ErrUsage` on malformed usage, without another provider call. Stream accumulation rejects invalid or overflowing updates with both `ErrStreamProtocol` and `ErrUsage`, retaining the earlier accepted partial result unchanged. Wire marshal/restore reject invalid usage as well. Custom invokers feeding structured/tool helpers must meet this contract: metadata errors are not schema-repair requests and never reach a tool handler.

Anthropic prompt totals include uncached input plus cache-read and cache-creation input. All three counters must be known before `UsagePrompt` is known; `UsageTotal` additionally requires known output. Cache counters are already included and must not be added again. Explicit zeros are known; omitted/null counters remain unknown. An unknown stop reason is incomplete, not success; `tool_use` and `refusal` now have distinct outcomes.

Gemini SDK scalar zero alone does not prove usage presence. A retained valid HTTP body can establish known zero only through a matching numeric counter field. Null, quoted numbers, other shapes, mismatched SDK/body values or malformed body cannot establish known zero. Do not use a present-but-null HTTP key as evidence of usage availability.

## Gemini part fidelity

ID-less local tool execution now requires `prompty.CapabilityToolNameCorrelation` in execution requirements and supported knowledge in the effective profile. The checked adapter must implement `adapter.ToolNameCorrelationAdapter`; Gemini provides its existing name-preserving function-response mapping. The helper checks destination/provenance/snapshot and supported tools/correlation diagnostics before handlers. Every ID-less call must be complete and its name unique across the entire batch, including siblings with IDs. Unconfirmed/ambiguous correlation fails with `ErrToolScope`; no provider IDs are fabricated. Custom invokers must supply truthful checked diagnostics. Other adapters do not gain this port merely through a profile override.

Mixed SDK content variants now fail with `adapter.ErrInvalidResponse` instead of choosing a switch branch and dropping other content. The current mapper rejects code execution/function-response payloads, media resolution/video metadata/display names and incremental function-call fields with `adapter.ErrUnsupportedContentType`. Do not use a capability override to claim these unmapped fields are preserved. Supported ordered text/thought, complete calls, inline/file media and signature-only parts retain their associations through response/execution wire and next-request translation. Signed `gemini.part.json` envelopes must remain required; changing them to optional fails before continuation dispatch. No legacy repair/fallback is provided.

Saved Gemini payloads must also retain exact JSON keys/values through SDK reconstruction. Duplicate keys (including inside argument objects/arrays), case aliases of SDK fields, unknown fields, discarded null/zero values and changed signature encodings now fail with `ErrProviderState` before dispatch. Preserve the library-produced envelope rather than hand-normalizing its JSON. Key order/whitespace are not signed values; arbitrary keys inside function args remain caller-owned and JSON numbers are decoded without float conversion. This codec check does not promise that an upstream SDK which already rounded a provider value can recover the original HTTP bytes.

Handle `ErrProviderState` when the current Gemini SDK cannot encode exact signed numeric values. Checked preparation and raw Translate reject this before dispatch rather than send rounded args with the same signature. For example 9007199254740993, 0.10000000000000001 and 1e1000 are not representable by this SDK request path; 9007199254740992, 0.1 and 1.00 retain their numeric values. Core wire/codec precision is unchanged. Do not round or edit a stored signed payload to bypass the check, and do not retry that same continuation blindly; use an independently valid provider workflow or an adapter encoding path with demonstrated fidelity.

Gemini `WithClient` now binds an isolated snapshot of the SDK's public configuration and a copied HTTP client for response observation. The original client/transport are not mutated; auth, redirect and timeout policy are retained. Rebind the adapter if you later reconfigure the SDK/client. The adapter observes one original response with invocation-local bounded memory (default 4 MiB); use `WithResponsePayloadLimit(bytes)` for a positive caller-owned bound. This does not provide capture transport coverage, persistence or replay.

Signed parts are decoded from original JSON with Number before canonical/envelope construction, not from the SDK's rounded float args. Original 9007199254740993 now remains exact in both wires; its next SDK request is rejected by the separate outbound gate rather than rounded and replayed. Oversized/incomplete original observation, ambiguous/lossy signed JSON or an incompatible source/SDK association cannot become a signed continuation. SDK signed decode overflow returns a safe typed ErrProviderState rather than echoing the provider body. Body-read errors and host redirect denial retain their causes and do not trigger replay.

Raw caller-supplied signed float DTOs without an original response Body now fail with ErrProviderState. Supply authoritative original payload or exact caller-owned Number/integer semantic values; do not cast an already-rounded SDK float to Number as a recovery step. The previous ingress defect IA-04 has parent implementation/regression evidence but still requires independent re-audit; `audit-checkpoint2.md` describes the historical failure.

## Anthropic thinking continuation

See [state/result conformance](state-conformance.md) for the implemented codecs, metadata/usage distinctions, pre-dispatch rejection fixtures and boundaries of local SDK versus live endpoint evidence.

Valid effective `CacheOmit` is allowed without editing signed payload. Leave Mode/Provider/Controls unset. An explicit `ReasoningPart.CachePolicy` replaces the message default; nil inherits it. If a message default specifies a marker, each signed block must explicitly omit that marker. A fully overridden default is `not_applicable` and needs no capability/TTL knowledge. Real effective controls on signed blocks still fail before dispatch; malformed policy shape is rejected even when unused. Do not infer cache bypass or permission to reuse state across destinations.

Thinking payload keys are exact and case-sensitive. Signed thinking requires only `type`, `thinking`, `signature`; redacted thinking requires only `type`, `data`. Null required values, duplicate keys, case aliases, unknown keys and fields from the other variant (including null) are rejected with `ErrProviderState` before continuation dispatch. Do not normalize or repair corrupted saved state into a replayable request. Whitespace/key order may change in SDK reconstruction; signed string values must not change.

Use the checked client for effective-model state binding, or call `ParseResponseForProfile` with the exact destination when using low-level SDK primitives. The default `ParseResponse` binds the configured default destination, not an arbitrary model override. Ordered exposed thinking text uses `ReasoningPart`; signature and redacted-thinking data live only in required `anthropic.thinking.json` envelopes. Unknown payload fields are rejected instead of silently discarded. Preserve response/execution wire and `AssistantMessage` associations. Do not edit signed text, remove its state, change destination, or add cache controls to thinking. Unsigned reasoning cannot be replayed as provider-signed history. A logging projection omits opaque bytes and explicitly cannot be reused for continuation.

## Media

Replace global HTTP client mutation with `mediafetch.DefaultFetcher{Client: client, MaxBodySize: limit}`. Caller redirect restrictions are preserved; every followed redirect must remain HTTPS. Address/DNS/proxy/egress decisions remain host-owned.

Without caller CheckRedirect, the standard ten-request limit remains; with one, that caller policy sets the hop limit, while HTTPS still applies to every hop. Separate fetchers retain independent policies concurrently and cancellation between redirects prevents the next request. Body reads accept an exact positive limit and reject an extra byte. MaxInt64 no longer overflows into an empty successful download; you do not need a special workaround or a library-imposed global size ceiling.

## Gemini native transport

Native tool fragments without a provider ID can now assemble through the original handle's checked source-declared PartIDs. Keep those logical references stable and distinct for distinct calls, including equal names; do not substitute provider IDs or infer identity from adjacency. Standalone `AssembleToolCallParts` still rejects ID-less deltas without this correlation proof. Handler admission remains separately checked: this assembly change does not authorize ambiguous name-only execution.

Replace the deleted `ParseStreamChunk` API with checked `ExecuteStream(ctx, execution, prompty.StreamNative)` and caller-owned supported native endpoint/model knowledge. The SDK chunk parser previously flattened ordered/signed parts and incorrectly treated complete arguments as incremental JSON; no compatibility bridge remains. `StreamBuffered` is an explicit different mode.

Complete native calls use `ToolCallPart.Args` snapshots. Signed parts retain the full checked destination, including model family. `WithResponsePayloadLimit` bounds each source SSE line including framing; source event JSON capture is semantic, not full HTTP bytes. Preserve the original handle, call `Close` on early stop and inspect terminal `Status` after iteration unwinds. Finish can precede late usage or metadata; read failure remains failure even after finish, retaining accepted output. No automatic second completion is issued. Signed incremental calls use the ordered partial-parts codec and continuation rules at the start of this guide. Incomplete/canceled/edited state remains noncontinuable. See the executable offline `adapter/gemini/stream_example_test.go`.

Gemini candidate citations/grounding have moved from `Annotations` on part zero
to `MessageAnnotations` on response, stream chunks and assistant history.
Use `candidate_citation_metadata` / `candidate_grounding_metadata` kinds and keep
stream updates in order. Exact source JSON retains explicit zero/empty values and
numeric precision; REST `citationSources` is not renamed in retained metadata.
Grounding chunk indices address provider chunks across those updates, not
canonical content parts. Segment indices remain provider-relative; no adapter
guarantee binds a late segment to a guessed text fragment. Do not merge metadata
updates as independent snapshots or replay them as text. Response/execution wire,
cloning, partial failure and canonical capture preserve them. Host redaction
must cover message metadata as well as content, part annotations and state.

Custom native stream sources can use `ResponseChunk.StateUpdates` when opaque
tool state evolves alongside argument fragments. Each update uses the current
frame's tool-part index and its established logical PartID; it explicitly
supersedes the prior binding on that call. Keep format/scope/codec/required/expiry
semantics unchanged, do not also supply ordinary state on the same part, and
retain every still-required signature inside the provider-owned payload. The
core never decodes or repairs that payload. Ordinary `ProviderState` remains
append-only: changing its bound content without an update still fails. Completed
`Result.ProviderState` contains latest bindings remapped to materialized parts,
not a list of every state update. Capture frame records identify update indices
in `state_update_parts`; status/history wire retains the resulting binding.
Gemini signed incremental mapping uses its separate ordered partial-parts codec
with source ordinals and original signature positions; actual Vertex SDK fixtures
cover stream/wire/continuation. The common update contract alone is not provider
integration evidence, and final independent acceptance remains required.

For unsigned incremental calls, put `gemini.CapabilityIncrementalToolArguments` in `execution.Requirements.Capabilities` and declare supported actual endpoint/model knowledge, along with tools/native. This explicit opt-in sends `streamFunctionCallArguments=true` via the Vertex SDK mapper. Optional boolean `stream_function_call_arguments` cannot silently enable the guarantee without its requirement or contradict it with false. MLDev's installed mapper cannot send the field and rejects before I/O; sync/buffered cannot select it.

Inspect `gemini.partial_function_call` annotations for exact path/value progress correlated to a provisional ToolCallPart with empty Args. Do not treat these values as raw JSON `ArgsChunk` text or execute a handler on a prefix. Closure emits a complete snapshot on the same logical reference. ID-less continuation needs a unique open call; provider-ID calls can interleave. See executable `adapter/gemini/partial_example_test.go`. Native parser progress does not bypass original-schema or separate handler-scope/correlation checks.

## Anthropic native output

Keep the response JSON Schema in `execution.ResponseFormat.Schema`. The adapter now sends it as `OutputConfig.Format.Schema`; it no longer synthesizes an `output_format` tool or forces tool choice. Real `execution.Tools` coexist with native output when the effective caller-owned profile supports both and does not forbid their combination. An explicit `execution.ForcedTool` still selects a real tool, independently of output format.

Remove assumptions that structured output arrives as a synthetic tool result. Native JSON output is ordinary text; a user tool called `output_format` is now returned as `ToolCallPart`, not converted to text. Checked compilation and original validation are bound; the local SDK fixture proves mapping and profile integration, not live endpoint/model availability.

## Synchronous capture

Set `execution.Capture` to caller-owned `CaptureBinding` (sink, optional projector, correlation and payload limit). Set `execution.Requirements.Capture` for required stages, minimum accuracy, durable acknowledgement or physical-attempt coverage. Requirements persist through execution wire; runtime ports do not, so rebind them after restore. Default projection captures metadata only and omits payload. Explicit host projection owns prompt/output/opaque-state redaction; SDK credentials and auth headers are not supplied to the generic sink. Record metadata always omits endpoint URLs.

Checked sync exposes canonical execution, translated request, provider response JSON, canonical parts and terminal. These are semantic projections, not HTTP bytes. Canonical execution keeps original schemas before lowering; provider RawJSON retains exposed SDK-unknown response fields. Transport bytes and physical attempts are not advertised and required coverage fails before dispatch. OpenAI disables hidden SDK retries. Embedded SDK build metadata is recorded when available; absence is explicitly `@unknown`, never a fabricated build identity.

Inspect `response.Diagnostics.Capture`. On post-dispatch capture failure, sync returns the known response together with typed `CaptureError{AfterDispatch:true}`; do not retry based on a sink error. Advisory capture failures do not change provider outcome but mark capture incomplete.

Caller-created or persisted `CaptureReport` must satisfy `report.Validate()`. Coverage stages are unique/recognized, observations are unique and covered, Closed matches an acknowledged terminal, and Complete requires Closed with no failures. Incomplete/pending/failed snapshots remain valid; an unavailable stage can appear as a coverage failure without becoming observed coverage. MarshalResponse, typed restore and runtime response metadata validation reject contradictions with `ErrCaptureReport`; JSON decoding may reject malformed structure first. A stream capture observer returning an invalid report fails the stream without installing false evidence or losing an existing execution error/partial result. Do not manufacture terminal/complete flags when migrating an old snapshot. Invalid reports are not authorization to replay a request.

Read observed metadata from `record.Projection.Metadata`. OpenAI SDK response/chunk hooks supply `ProviderResponseID` and presence-aware Usage, not HTTP RequestID or HTTPStatus. These HTTP fields remain absent on the current SDK JSON paths. Canonical records supply outcome/usage, and checked compilation supplies role/name/source/translated digest associations without endpoint/provenance/raw schemas. Metadata is isolated from source state before both callbacks. A custom projector owns metadata redaction as well as payload redaction; returning a newly constructed projection with zero Metadata omits those fields. Mark any omissions/redaction as incomplete. Default metadata-only projection excludes response/request IDs, retains isolated known usage/outcome/digests and remains incomplete because payload was omitted. Invalid projected usage, outcome or HTTP status is rejected before sink delivery.

Native OpenAI adds `CaptureProviderChunk` before normalization. Required raw chunks fail before dispatch on adapters without that hook. Canonical frame/terminal capture preserves wire associations and partial usage; frames carry explicit `stream_kind`. Normal terminal observation runs after assembly and original validation, including failed snapshots. Consumer break instead skips assembly/whole-document finalization, unwinds the canceled producer and performs one capture closeout without another yield. Inspect final `stream.Status().Result.Diagnostics.Capture`: an acknowledged canceled terminal may be Closed/Complete for the requested capture stages, but execution remains canceled and partial, not successful. Redaction or failed closeout remains incomplete, and existing frame errors are joined with cancellation/closeout errors. Running Close requests cancellation and terminal report/status becomes immutable only after unwind; caller cancellation/Close also cancels the observer, which may prevent its acknowledgement. A required sink failure retains a successfully parsed chunk before failure and never replays the request. HTTP/SSE bytes and physical attempts still have no claimed coverage.

## Remaining migration work

The owner selected all five extensions. The adapters and concrete migration APIs
above are implemented, while expanded fault conformance and final independent
acceptance remain open. No new consumer-demand answers are required, and current
local SDK evidence is not a live endpoint/model/account guarantee.

## Profile-scoped reasoning options

Reasoning knobs remain in `ModelOptions.ProviderSettings`; no universal budget/mode DTO is introduced. Add `ModelOptionsSchema` (JSON Schema draft 2020-12) to each caller-owned capability profile that accepts these knobs. The schema validates the complete `ModelOptions` object, including `provider_settings`, numeric budgets and combinations with other fields. Without it, checked reasoning requests return `adapter.ErrUnknownProfile`; an incompatible value/combination returns `ErrUnsupportedCapability` with a `model_options` diagnostic. Invalid/unresolved schemas fail before translation/dispatch. Use the existing runtime `execution.Validator` with explicit resources if references are needed. Profile schemas survive overrides, clone and execution wire without aliasing.

```go
profile.ModelOptionsSchema = prompty.JSONDocument(`{
  "type":"object",
  "properties":{"provider_settings":{"type":"object",
    "properties":{"reasoning_effort":{"enum":["low","medium"]}},
    "additionalProperties":false}},
  "not":{"required":["temperature"]}
}`)
execution.ModelOptions = &prompty.ModelOptions{
  MaxTokens: new(int64(4096)),
  ProviderSettings: prompty.JSONDocument(`{"reasoning_effort":"low"}`),
}
```

This is an illustrative caller contract, not a model support catalog. Set allowed values/constraints from knowledge of your actual destination. OpenAI no longer guesses reasoning behavior from `o1`/`o3` prefixes or silently removes temperature when effort is set. `MaxTokens` maps consistently to `max_completion_tokens`; it bounds visible and non-visible completion output. Update clients that depended on the old `max_tokens` field or temperature omission; a checked profile must reject unsupported combinations rather than relying on silent mapping changes.

For combinations with execution features, use the existing profile rules, for example `profile.ForbiddenCombinations = [][]string{{prompty.CapabilityTools, prompty.CapabilityReasoning}}`. Requested reasoning controls participate in these checks alongside tools, native stream and output requirements; options-schema validation alone does not allow a forbidden feature combination.

Anthropic maps `thinking_mode` (`enabled`, `adaptive`, `disabled`) independently of effort. Enabled requires an integer `thinking_budget` at least 1024 and below effective `max_tokens`; adaptive/disabled reject a supplied budget. `thinking_visibility` (`summarized`, `omitted`) requires enabled/adaptive mode and uses the SDK's explicit extra-field extension for `display`; the checked local SDK fixture verifies actual request bytes. `reasoning_effort` maps to `output_config.effort` (`low`, `medium`, `high`, `max`). Exact mode/effort/visibility availability and combinations still come from your model profile. Visibility does not grant access to hidden reasoning or reconstruct missing state.

Gemini retains `thinking` as the exposed-summary visibility flag. `thinking_budget` accepts -1 (dynamic), 0 (disabled when supported by profile), or a nonnegative token budget. Alternatively use `thinking_level` (`minimal`, `low`, `medium`, `high`), not budget and level together. A profile may reject any SDK-representable value or combination unsupported by its model. Other modules do not acquire reasoning support through a profile alone; unmapped settings fail explicitly.

`ProviderSettingsMap` now returns `json.Number` for numeric values, preserving exact integers instead of first rounding them through float64. Custom adapters must handle this representation. Internal numeric conversion rejects fractional/out-of-range integer settings and the rounded float upper bound rather than wrapping it to a negative integer.

## Cache policy clear break

`CachePolicy.Type` is removed. Saved execution wire with type-only hints is rejected; update manifests and stored executions explicitly. A block marker now uses:

```go
policy := &prompty.CachePolicy{
    Mechanism: prompty.CacheBlockMarker,
    Mode: prompty.CacheRequired,
    Provider: "anthropic",
    Controls: prompty.JSONDocument(`{"ttl":"1h"}`),
}
execution.Messages[0].CachePolicy = policy
```

Declare `prompty.CapabilityCacheMarker` and `cache_control:ttl` as supported in the caller-owned profile for the effective endpoint/model. The TTL capability means the caller has verified both mapped values (`5m`/`1h`) for that destination, not merely the existence of an SDK field. Without that knowledge a required request fails before dispatch. The module enforces at most four effective markers and longer TTLs before shorter/default TTLs; it does not silently drop controls. Unsupported bypass/retention options fail rather than being treated as deletion or cost guarantees.

Use `prompty.CacheAdvisory` (or empty Mode) if unsupported/unknown controls may be omitted. Inspect `response.Diagnostics.Cache`: each decision has a stable source path and accepted/unsupported/not_applicable/unknown disposition, without control values or prompt content. Accepted means selected request intent, not a hit, persistence or permission to reuse someone else's data. Message policy is an inherited default; part policy overrides it. Normalize no longer merges messages with different message-level cache policies. Clone and wire preserve controls without mutable aliases.

Use `&prompty.CachePolicy{Mechanism: prompty.CacheOmit}` to explicitly prevent a part from inheriting a block marker. Leave Mode/Provider/Controls unset. Nil continues to mean inherit. Checked omission of unsupported/conflicting advisory controls or an accepted implicit-prefix hint also uses this portable policy so a parent TTL is not silently reactivated. Diagnostics still describe the original source intent; the caller's execution is unchanged. CacheOmit does not disable provider implicit caching, delete a cache, enforce retention or authorize reuse.

A message-level policy is only a default. When every top-level part overrides it (or the message has no content), checked preparation now reports that default as `CacheNotApplicable` and does not demand unused mapper/TTL support or include it in combinations. If any block inherits the default, required checks remain. A tool-result block has its own marker: overriding all of its children does not suppress the enclosing marker. Malformed policy shape still fails before preparation. `MessageInheritsCachePolicy` exposes this read-only canonical inheritance query, not provider support knowledge.

For implicit prefix caching, use `prompty.CacheImplicitPrefix` and explicitly declare `prompty.CapabilityCacheImplicit`; no block field or TTL is fabricated. OpenAI whole-request key/retention belongs in RequestCache as described above, not block policies. Required intent needs checked execution; raw unmapped translators reject it. Read `Usage.Known` for cache read/write counters—unknown is not zero or evidence of a miss. Explicit resource lifecycle is a separate implemented Gemini adapter, not a consequence of these controls or an unresolved demand trigger.

Include cache mechanisms and `cache_control:<key>` in destination-profile `ForbiddenCombinations` when they cannot coexist with tools/output/stream/state/reasoning or another cache control. Checked preparation now enforces those combinations across message and nested part policies. Conflicting advisory controls are removed with unsupported diagnostics, then required controls are reevaluated; a remaining required conflict returns both `ErrCacheRequirement` and `adapter.ErrUnsupportedCombination` before translation/dispatch. Independent controls remain. Mapper policy validation receives isolated data and cannot rewrite the selected request intent. This is not resource deletion, routing or a cache-hit guarantee.

Tool-stream sources must use `ArgsChunk` for incremental arguments and `Args` only for a complete JSON object snapshot, never both on one part. Annotation indices follow collapsed calls during terminal materialization; opaque continuation state is remapped only when its own associated part remains byte/structurally unchanged. An unrelated citation/signature no longer prevents complete calls from assembling.

The portable policy slot is `execution.Requirements.Schema`: mode `strict`/`lowering` and `AllowedTransforms`. Clone/history/Normalize and execution wire preserve it; runtime validators must still be rebound after restoration. Wire and shared adapter validation reject malformed policies. `CompileProviderSchema` is bound to checked execution and native finalization. Lowering availability remains mapper-specific; a persisted policy does not implement an absent transformation.

Checked `adapter.NewClient` now compiles output/tool schemas before Translate and requires the adapter's actual `ProviderSchemaMapper` implementation; capability overrides cannot create that port. Anthropic native object schemas, Gemini's strict subset, OpenAI output lowering/tool passthrough and Ollama strict tool representability checks are bound. Custom adapters must implement the mapper and report every permitted semantic transformation; SDK serialization must preserve numeric constraints without rounding or string conversion. Provider/model conformance remains explicit and scoped, not universal.

For sync calls, inspect `response.Diagnostics.Schemas` for role/name-associated source/translated digests and lowering diagnostics. The response is validated against original schemas. A post-dispatch schema error may return a non-nil response alongside the error: retain its outcome/usage/diagnostics, and do not interpret the failure as proof that execution did not occur or automatically retry. Refusal/incomplete uses `OutcomeError`, not a repairable JSON mismatch.

## OpenAI explicit schema lowering

Silent optional-property normalization is removed. Strict output schemas must already forbid additional properties and require every declared object property, recursively. To intentionally change that contract through checked sync execution, select `Requirements.Schema.Mode = prompty.SchemaLowering` and explicitly allow `openai.LoweringCloseObject` and/or `openai.LoweringRequiredNullable`. Each actual change appears in `Diagnostics.Schemas[].Transformations` with a source pointer and original/translated digests.

Required-nullable wraps the complete source subschema, preserving enum/const/refs and other constraints for its non-null branch. The original validator still rejects null if the source contract forbids it. Raw `Translate` only accepts an already strict-compatible output schema; use the checked client to obtain lowering reports and original-result validation, including terminal native-stream validation.

## Checked stream finalization

Checked native streams now validate their assembled output/tool args against prepared original schemas after normal source EOF and trailing usage. Deltas remain provisional. A provider finish event is not final success: inspect the single terminal event and `stream.Status()`. Schema failure retains accumulated content, usage, outcome and schema diagnostics in failed status and never retries completion.

`WithStreamFinalizer` is an optional runtime validation port for custom streams. It receives a cloned assembled response and cancellation context, runs without the status mutex, and must honor cancellation. Early consumer stop or protocol/transport failure skips finalization. Buffered post-dispatch validation errors now preserve a known response in status instead of appearing as an empty failed execution.

## Gemini strict schema mapping

The typed response-schema mapper no longer drops unsupported keywords or values. A checked request fails before dispatch with `*prompty.SchemaFidelityError`, which unwraps to `ErrSchemaUnsupported` and identifies an RFC 6901 source `Pointer`. Numeric enum, numeric constraints, references/compositions, boolean schemas and type unions are not supported by this mapper. This is an implementation limitation, not a statement about every endpoint's capabilities.

Do not strip constraints from the original schema to bypass this error: that changes the contract. This Gemini typed mapper does not implement lowering transformations and reports unsupported input explicitly. Other implemented provider mappers have separately documented strict/lowering profiles; their support is not inherited by Gemini. No automatic weaker-mode fallback is provided.

## Ollama tool-schema fidelity

IA10 correction: `ollama.WithClient` now accepts the provider-scoped ChatClient operation port. Use `ollama.NewHTTPChatClient(endpointURL, hostHTTPClient, positiveFrameByteLimit)` for exact numeric arguments. Rebind auth/egress/redirect/timeout policy explicitly through that host HTTP client; SDK automatic auth is not inherited. Already-rounded SDK floats fail with adapter.ErrMalformedArgs before handlers instead of being repaired. Outbound tool history uses Number-preserving SDK construction. `DecodeToolArgs[map[string]any]` now returns json.Number for numeric interface values; update assertions/conversions accordingly. Explicit int/float fields remain caller-selected DTO semantics. Fixtures cover sync/native/buffered, nested values, wire restore and actual subsequent HTTP encoding; final independent acceptance remains pending.

The adapter now rejects tool parameter fields that its SDK drops or changes. Both checked compilation and raw tool translation compare source fields against SDK serialization. Unsupported constraints (including nested required/minimum or additionalProperties) and rounded numeric enums produce `SchemaFidelityError.Pointer` before dispatch. Do not remove constraints to make translation pass; no lossy lowering is implemented for this mapper. Ordinary representable object tool schemas remain supported under the effective caller-owned profile. Output schemas remain unsupported by this adapter.

## Custom invoker wrappers and checked helpers

Running `Stream.Close` is now a nonblocking cancellation request; wait for the active event iterator to unwind before treating Status as immutable terminal evidence. Close before consumption still cancels immediately without dispatch. A plain consumer break now runs one capture closeout after producer cleanup, sends no additional event and skips whole-document finalization. Inspect Status.Err for both cancellation and any retained frame/capture errors. Close and caller cancellation still cancel callback work; there is no detached recording context or background retry.

Pass `adapter.NewClient` or `adapter.NewRuntimeClient` into structured/tool helpers for library-owned provider preflight. A custom wrapper can expose only `prompty.Invoker` and delegate unchanged executions and stream mode to that client. Do not clear requirements or delegate to raw SDK Execute while claiming checked guarantees. Arbitrary custom implementations must enforce the Invoker fail-closed contract themselves; helpers cannot inspect their transport or establish zero dispatch after a response. Local original-schema/outcome/tool-batch checks still apply independently. See [helper composition evidence](helper-preflight-conformance.md) for the thirty zero-dispatch fixtures and the resolved host-owned Invoker trust boundary. No new proof-bearing interface is required.

## Gemini response-error privacy

After the original HTTP 2xx body has been completely received, malformed/unconvertible SDK responses fail with static `ErrProviderState` instead of a body-bearing SDK error. Do not unwrap or log the old SDK decoding cause; it is deliberately no longer exposed. This applies without successfully detecting a signature and also isolates SDK conversion panics on malformed response shapes after successful EOF. There is no response replay. Incomplete read/transport/host-policy errors retain their causes; cancellation/deadline remains identifiable separately from sanitized decoding failure. Caller transport panics before completed successful response observation are not hidden.
