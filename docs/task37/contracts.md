# Provider execution contract

## Whole-request cache intent

PromptExecution.RequestCache is a portable CachePolicy for the complete request,
not a message/part marker or CacheResource. Its mechanism is implicit_prefix;
controls are provider-scoped JSON, cloned and persisted in execution wire. Nil
means no explicit request policy. Required unknown/unsupported controls fail
before dispatch; advisory controls are omitted with visible disposition. Accepted
means selected API settings, never observed hit, retention durability or savings.
Control-key capabilities and forbidden combinations participate in the same
cache preflight as block policies; diagnostics omit values. The optional actual
request mapper port cannot be supplied by profile knowledge alone. Raw translation
is not checked execution. Remote resource CRUD and cleanup remain host-invoked.

OpenAI first mappings are prompt_cache_key and prompt_cache_retention on both
Chat Completions and Responses. Only installed SDK values are mapped, with model/
endpoint knowledge required separately. Other provider knobs are not fabricated;
Gemini automatic implicit cache and explicit resource reuse remain distinct.

## Responses local functions

Handle operations check their own capability/profile/destination, not a fresh
generation request. A generation ModelOptionsSchema requiring fields such as
max_tokens must not block response Get/Cancel, batch inspect/cancel/results or
cache get/update/delete after acceptance. Operation preflight preserves the whole
effective profile in the snapshot and enforces requested capability combinations,
but does not fabricate generation settings or validate empty options against a
generation-only schema. Generation Create/Submit retain full generation preflight.
Cache-content Create is also non-generation: its input operation still checks
message/tool/state/MIME capabilities and original schemas, and rejects any
generation settings rather than fabricating them. Original result schemas and bindings still validate separately. Invalid
profile knowledge, unknown operations and incompatible handles remain fail-closed.

Local ToolDefinition maps to native functions separately from hosted search.
Original schemas are preserved, strict=false is explicit, and received completed
arguments are locally validated before success. ForcedTool uses native named
choice and its independent capability, never an instruction. Unresolved required
tools, empty/duplicate names and invalid schemas fail before I/O. No handler runs.
Handles retain tool-name/source-schema digests and forced selection. Get/Cancel
rebind original tools through operation options and check exact names/digests,
without requiring generation tools support. Missing/changed schemas fail locally.
Native call_id is ToolCallPart.ID, distinct from the provider item ID. Original
function-call JSON is required scoped openai.responses.function_call.json state.
Duplicate call IDs, undeclared names, wrong forced choice and invalid completed
arguments retain known output with errors. Unfinished items are incomplete.
Function-call history must retain exact completed native items in required scoped
state. Restore verifies canonical ID/name/arguments, no args delta/cache, original
shape/completion, scope/expiry and uniqueness before SDK ToParam. Text, reasoning
and function items retain their order; no item identity is reconstructed.

ToolResultPart maps to native function_call_output by explicit call_id. For
stateless history the prior checked function item must precede the result; for
Previous-only continuation the handle's known completed ToolCalls binds ID/name.
Unknown, duplicate, premature or conflicting references fail before I/O. Current
tools/instructions remain explicit; results never execute handlers. This mapping
supports ordered text/image/document arrays with IsError=false. Images use approved
HTTP(S) URLs or byte-backed data URLs; documents use URLs or inline PDF data.
InputMIME profile gates also apply inside results, with exclusive sources and no
fetch/upload. Audio/video, cache/state decorations and IsError=true are explicitly
unsupported. The native function-output item has no independent error flag; the
adapter never silently drops IsError or invents an error wrapper. Hosts wanting
an error payload provide their own explicit text/JSON semantics with IsError=false.
Protocol basis: [OpenAI function calling](https://developers.openai.com/api/docs/guides/function-calling).

Remote output/tool schema bindings hash normalized dialect, fixed role, schema
name and exact source digest. A source-only digest does not prove the original
interpretation: changing dialect with identical JSON must fail before retrieval.
Diagnostic SourceDigest retains its exact-byte meaning; binding hashes are a
separate handle contract. No legacy source-only hash fallback is allowed.

## Responses reasoning state

Each native reasoning item projects to one ordered ReasoningPart containing only
its exposed summary texts joined by two newlines (empty summary remains empty).
Encrypted/private content is never text or a public annotation. A nonempty
encrypted_content gives a required, exact original-JSON envelope using
openai.responses.reasoning.json, bound to the effective destination and part.
Absent/null/empty encrypted content marks continuation unavailable; output text
can still succeed. Malformed summaries/status fail with retained earlier output.
Unfinished reasoning makes the canonical outcome incomplete.

Stateless assistant continuation validates scope, expiry, required envelope,
original summary projection and completed item before sending original native
JSON through the SDK reasoning union. No reconstruction from summary text is
allowed. Text runs and reasoning retain order; unsupported metadata fails closed.
An explicit provider_state requirement on Create requests encrypted content;
missing state returns ErrStateUnavailable alongside the known response. The
portable handle persists ProviderStateRequired as an explicit boolean, including
false. Missing/null/nonboolean wire fields are rejected: clear break, no old-wire
default. Create records its actual current state requirement; a new Create with
Previous does not inherit that requirement implicitly.

Get with ProviderStateRequired checks the independent provider_state capability
and requests reasoning.encrypted_content, composing it with hosted-search includes.
The requirement remains in known handles after queued/terminal/unavailable/failed
observations; omission is visible as ErrStateUnavailable with known output. Cancel
does not invent an include parameter absent from the native operation or require
retrieval capability to cancel. If its response lacks required state, the known
cancel outcome and requirement survive with the state error; caller may explicitly
Get afterward. No automatic fetch/retry or durability promise is introduced.
Hosted stateless history and local function declarations/results are implemented
with original scoped native items, order/correlation and actual SDK fixtures.
Expanded operation conformance is recorded in operation-conformance.md; final
paired audits remain mandatory and local fixtures do not establish live support.

Get/Cancel schema rebinding validates the original schema/digest locally without
requiring the current native_json_schema generation capability. No schema
lowering or generation replay is authorized to inspect accepted output. Unknown
or unsupported operation capabilities still fail before I/O.

Protocol basis: [OpenAI reasoning guide](https://developers.openai.com/api/docs/guides/reasoning)
describes replaying encrypted items for stateless continuation. The installed SDK
supports exact reasoning-item ToParam and explicit encrypted-content inclusion;
no hidden reasoning is interpreted.
Get includes follow the [native retrieve contract](https://developers.openai.com/api/reference/resources/responses/methods/retrieve)
and the installed SDK query encoder; Cancel has no include argument.

## Batch operations

Gemini Developer API batches use the native operation resource `batches/<id>`.
Inspection and cancellation validate scoped handles and independent operation
capabilities before I/O. A cancel acknowledgement is not terminal cancellation:
retain already observed terminal status, otherwise record canceling and require
explicit subsequent inspection. Failed inspection preserves the prior handle;
unknown mutation never triggers a second submission/cancellation.
Read original bounded response JSON before SDK normalization. Persist raw native
status, reject duplicate fields/foreign identity/model, and retain a uniquely
observed accepted identity even if other metadata is malformed. Inline result
metadata keys provide correlation; ordered position is not a substitute. Developer
inline transport and Vertex/file-backed transports require separate conformance;
support for one must not imply support for the others.
Inline Submit reuses checked generation preparation for every item, requires one
model, unique caller keys and explicit attempt lineage. All checks precede remote
creation. Original executions/digests are retained for Results rebinding. Native
SDK request conversion must preserve exact content numbers, not normalize tool
arguments through float64. Recheck cache expiry after capture and immediately
before creation. Unknown accepted submission retains safe observed identity and
does not retry. Inline submission has an explicit bounded payload; no hidden file
upload fallback or server-side job retention/durability guarantee is implied.

Results preparation is a separate local operation, not request translation or
replay. Validate the rebound execution digest and destination, check the independent
results capability and compile original output/tool schemas with the host validator.
Do not require generation-only cache/state leases, input MIME/reasoning settings or
submit capabilities to remain supported/live. No outgoing request is prepared,
no input state is replayed or renewed, and no provider schema lowering is reapplied.
Diagnostics describe results capability/original validation, not current native
generation availability. Submit retains all generation-time validation gates.

Gemini file batches explicitly inject the common scoped file transport. Submit
prepares every item and encodes native GenerateContent JSONL with caller `key`,
then uploads once and creates the native job with `fileName`. No implicit inline/
file fallback. Known upload references survive later failure separately from job
acceptance. Handle input/output references use safe `files/<id>` names; Get observes
`response.responsesFile` (REST) or `metadata.output.responsesFile` (SDK source)
without inventing a result file. Simultaneous sources or file/inline union members
are rejected as ambiguous. These paths follow the
[provider Batch guide](https://ai.google.dev/gemini-api/docs/batch-api) and the
checked SDK source fixtures. Results downloads
only the observed output reference, with caller byte/line limits and one body close;
malformed/read/close/capture failures retain accepted prefix and known job status.
Correlate each file record by `key`, not position. Original item schema/profile
validation is shared with inline results. File-port capture accuracy must be
explicit before required source capture; host owns transport/auth/egress/cleanup.

File-backed batch transports inject BatchFileTransport with explicit provider/
endpoint/account binding. Upload/download are bounded transport operations, not
an application file store; cleanup is host-owned. Handle resources retain input/
output/error file references, without URLs or credentials. Known uploaded input
survives later capture/submit failure in BatchResult.Resources; distinguish file
upload unknown from batch submission unknown and record SubmissionAttempted.
Required provider-response capture checks the injected file port's explicit
BatchFileCaptureDescriber accuracy before I/O. An opaque custom port does not
inherit the native SDK port's semantic source coverage. No byte/physical-attempt
coverage is inferred.
All item/profile/schema/capture checks precede upload. Capture distinguishes the
planned upload from the actual post-upload batch request. A failed upload or
post-upload capture never silently proceeds to create a batch or deletes a file.
Results reads both output and error files when present, retaining provider order
within each and reconciling IDs across all files. Missing/failed file retrieval
keeps observed prefix items and remote status. File absence does not imply that
unreported items succeeded or were canceled.

BatchManager is an optional explicit Submit/Get/Cancel/Results port. Batch handles
persist destination/account/provider ID, observed remote status and ordered item
bindings: unique caller ID, caller-owned original/attempt lineage, model profile
and canonical execution digest. The host rebinds original executions/validators
for Results; mismatched contracts fail before download. Independent model profiles
may differ only where the native protocol permits; OpenAI requires one model per
batch. All items must share destination. No credentials, scheduler,
dataset runner, retry or application storage is persisted or started.

Batch completion means remote processing ended, not item success. Results retain
provider order and original line JSON, correlate by caller ID and distinguish
success/error/refusal/incomplete/expired/canceled where the protocol exposes them.
Missing, duplicate and unexpected IDs are explicit; duplicate/unexpected records
are not silently discarded. Download completeness and correlation completeness
are separate from remote status. Partial retrieval retains accepted prefix items
without changing known remote completion into unknown execution. Unknown mutation
does not authorize automatic resubmission. Caller retries use a new attempt ID
and preserve original/parent lineage. Capture uses common host-policy stages;
per-item input capture must not be silently ignored or claimed separately.

## Remote response operations

RemoteResponseManager is an optional caller-driven Create/Get/Cancel port, not
an Invoker compatibility shim. Create reuses canonical execution plus explicit
storage intent, background selection and an optional previous scoped handle.
Continuation/background/retrieve/cancel and each hosted tool have independent
profile capabilities. The first adapter is openai.responses, distinct from
Chat Completions knowledge and state. Caller resends instructions/tools; no
settings are inherited locally. SDK retries are disabled on every operation.

RemoteResponseHandle stores format, destination/account binding, provider ID,
observed status, background/storage selection and optional original output-schema
digest. No credentials/SDK client/worker are persisted. Wire validation checks
shape, not remote availability or authorization. Get/Cancel rebind runtime ports.
Structured background retrieval requires the same original schema and validator
binding before I/O; digest mismatch/missing schema cannot produce checked success.
Unknown create/cancel preserves available identity, never fabricates acceptance
or retries. Failed retrieval does not erase a previously observed remote status.
Cancellation is not rollback. Store intent does not attest retention/durability;
the provider may retain background data under account/API-specific policy.

Operation capture uses common semantic stages, host policy and one closeout.
Known accepted handles/results survive post-dispatch capture errors. No claim of
HTTP bytes or physical attempt instrumentation follows from SDK RawJSON capture.
Hosted-tool payloads remain provider execution data, not local handler calls.
Unsupported input/configuration must fail before dispatch rather than disappear.

Responses user/assistant messages preserve ordered input_text parts rather than
concatenating them. User media supports native input_image (PNG/JPEG/WebP/GIF)
from host HTTP(S) URL or inline bytes as a MIME-bound data URL; document HTTP(S)
URLs map to input_file and inline PDF bytes to file_data with a deterministic
filename. Exactly one URL/data source is required; media kind/MIME must agree.
No local fetch, upload, file store or capability inference is introduced. Actual
profile InputMIME checks precede mapping. System/developer instructions remain
text-only and explicit; assistant media, cache markers, unsupported kinds and
unrepresentable state/metadata fail before I/O. Host owns URL egress/access policy;
native media does not claim output-media or realtime parity. Protocol references:
[vision](https://developers.openai.com/api/docs/guides/images-vision) and
[file inputs](https://developers.openai.com/api/docs/guides/file-inputs).

Responses hosted search accepts explicit web_search/file_search settings only,
with separate capabilities and combination checks. Settings are immutable checked
JSON, not arbitrary request overrides; reserved/unknown/duplicate keys fail before
I/O. Caller supplies existing vector-store references; no corpus CRUD or local
handler executes. Request tools are resent explicitly, including an empty list
when none selected. Each selected search requests its exposed sources/results;
scoped handles remember the selected kinds for checked retrieval, not inheritance
into a new Create. Restore/clone isolate those lists.

HostedSearchItems projects typed ordered provider calls from message annotations.
The annotation retains output index and complete original item JSON so response
wire/history/clone do not depend on SDK lossy maps. Typed sources/results keep
Number-preserving attributes/score and expose original data. Coverage refers only
to observed result collections: missing unknown, null unavailable, non-completed
partial, completed array available (including empty). It never means exhaustive
web/corpus retrieval or tool success. Failed/incomplete call status is retained.
Citations keep canonical text-part bindings; source provider indices are not
reinterpreted as content indices. Hosted items never become executable local calls.

Hosted stateless history carries a scoped ordered output manifest in message
annotations. Public message/search source items retain original JSON and citations;
reasoning/function entries refer only to canonical part indices, never duplicate
private payloads. Replay requires exact canonical projection, annotations, complete
source coverage and valid required state, with unique native IDs across history.
Completed messages and completed/failed search items can replay; unfinished items
fail before I/O. Destination mismatch, removed/duplicate manifest, edited canonical
content/citations, missing state and unknown metadata are explicit errors. Current
tools remain explicit, including empty; replay never executes a hosted/local tool.
Source-less historical metadata is not guessed into a new order or bridged.

## Managed cache resources

Lifecycle capture reuses CaptureSession, stages, host projector/sink, bounded
backpressure, durable acknowledgements and safe pre/post-dispatch errors.
Create takes capture intent/binding from its canonical execution. Inspect/update/
delete accept at most one CacheResourceOperationOptions snapshot. Capture remains
optional; unsupported bytes/transport/physical-attempt claims fail before I/O.
Stages distinguish canonical operation input, SDK parameters, bounded original
provider JSON, canonical resource result and one terminal. Source JSON is not
HTTP byte coverage. Required post-dispatch capture failure preserves known
resource result; it is not permission to repeat the mutation. Terminal payload
contains safe error categories, never SDK/projector/sink exception text. No
capture service or application storage is introduced into the library.

Generation selects an explicit `PromptExecution.CacheResource` handle. Checked
preflight requires the cache-reuse capability and actual validation/mapping port.
The Gemini adapter also requires a host account binding; wire restore does not
recreate that runtime binding. Ordinary adapters reject the handle, including
low-level Translate. Expired/unknown/deleted and foreign endpoint/model/account
bindings fail before dispatch. Base generation reuse does not override cached
system/tools/tool config; conflicting supplied system messages/local tools fail
explicitly, not silently disappear. No handle repair/upload/create occurs.

Expiry is checked again at logical dispatch, after capture callbacks and any
native StreamStart consumer pause. Preflight is not a lease: time-sensitive
handles and absolute create/update expiry cannot become valid merely because
they passed an earlier check. Rejection before dispatch records a pre-dispatch
terminal without provider I/O. Expiry during a remote call remains a provider
outcome; this check does not promise a remote lease or rollback.

An optional CacheResourceManager port performs caller-invoked create/get/update
expiry/delete; no scheduler, store, IAM or hidden recache is part of it. Its
portable handle contains a format marker, existing ProfileIdentity, host account
binding, provider ID, immutable content/config SHA-256, observed expiry and
resource state. Runtime credentials/transport are not serialized. Handle wire
validation is separate from current expiry/destination reuse checks. Expired
handles can still be inspected/deleted; generation must reject them. Observed
delete is not proof that arbitrary retained copies cannot exist.

Create inputs reuse canonical PromptExecution for immutable contents/system/tools,
not a second prompt contract. Generation-only settings/output schemas and cache
markers are not silently repurposed as immutable cache configuration. TTL or
absolute expiry is explicit and mutually exclusive. The manager checks separate
operation capabilities against the configured endpoint/model. Unknown remote
outcomes preserve available identity and require caller reconciliation; no
blind create/delete/update retry. Resource token count is presence-bearing
metadata, not generation usage or a billing/latency/cache-hit guarantee.

Gemini SDK operations use an isolated injected client, with bounded original
response observation for resource metadata presence. Provider names/models must
match the operation; names cannot inject paths/query/credentials. The handle
digest describes the immutable translated SDK configuration, excluding credentials,
display labels and expiry. Expanded generation/lifecycle conformance and final
independent audits remain required before full acceptance.

## Explicit synthetic structured output

`ExecutionRequirements.OutputMode` selects native output (zero value or
`native`) versus `synthetic_tool`. The latter requires ResponseFormat and an
actual SyntheticOutputAdapter port as well as independent synthetic-output,
tools and forced-selection profile knowledge. A profile override never creates
a mapper. Native requirements do not silently downgrade after errors. This
field survives execution wire/clone; unknown modes and missing output schemas
fail before dispatch.

Anthropic's separate `NewSyntheticOutput` adapter binds the output tool name to
the request, using ResponseFormat.Name, not a global magic name or shared mutable
adapter state. Its base profile forbids real tools and caller ForcedTool. Manual
enabled thinking is incompatible with forced choice; other reasoning support
remains explicit endpoint/model combination knowledge, never silently disabled.
It sends one tool and named forced choice, not native OutputConfig.Format. It
parses exactly one complete matching tool-use object into JSON text, never a
handler-eligible ToolCallPart. Checked execution validates the original schema
before success. Raw parsing is not original-schema validation. Typed structural
errors and refusal/incomplete outcomes retain observed usage and partial output.
Sync/buffered transport is separate from native streaming; native support must
be implemented and verified, not inferred from the base adapter's methods.

## Ollama exact tool arguments

ChatClient is a provider-scoped injected operation port with SDK request/response types, outside core. SDK clients remain usable only where observed args can prove precision; unverified floating-point argument values fail closed. NewHTTPChatClient receives an explicit endpoint, host HTTP client and positive per-frame limit; preserves host auth/egress/redirect/timeout policy, streams one bounded NDJSON frame at a time, and decodes tool objects with json.Number before SDK argument conversion. No SDK-private introspection, fallback/retry, queues, credential acquisition or application storage. Errors omit provider body; break/cancel closes body. Outbound canonical tool args also use Number-preserving construction, never SDK UnmarshalJSON float conversion. This is the implementation target, not completion evidence.

## Gemini ordered part fidelity

Native tool state can evolve only through explicit `ResponseChunk.StateUpdates`.
Each update is a `PartState` indexed into that frame's complete/provisional
`ToolCallPart`, with the same validated logical PartID as previous fragments.
It replaces at most one earlier state on that logical call and binds the new
envelope to the exact current content part. A first update may establish state;
updates cannot delete state, change scope/codec/required semantics or repair
multiple ambiguous old bindings. Ordinary ProviderState remains append-only.
An update cannot also declare ordinary state on that part. All validations,
identity/usage checks and the replacement plan finish before any accepted state
is changed. The common assembler sees only the latest accepted binding; changing
state-bound content without an explicit update continues to fail. Failed/canceled
partial status preserves the latest accepted opaque state; host projection can
omit it and marks required continuation unavailable. The core does not interpret
provider payloads or invent signatures. Source-frame capture projects updates as
current part states while retaining explicit state-update event semantics.
This supports provider-signed incremental calls, not a general state migration
service or an authorization mechanism. Gemini codec/mapping and source fidelity
must additionally prove any final binding and faithful continuation request.

Signed Gemini incremental function calls use a separate `gemini.partial_parts.json`
codec containing ordered original SDK-known parts for one source-proven logical
call. Every fragment is retained, including unsigned fragments before/after a
signature; signatures stay on their original parts. Canonical arguments are a
projection, never the signed outgoing representation. Once any signature appears,
each accepted frame emits StateUpdates binding the whole sequence to the exact
current provisional/complete call. Sequence size is bounded; unknown or malformed
parts fail closed. Restore replays the partial-argument contract and compares the
canonical call before returning original parts in original order. Incomplete
sequences, incompatible backend, edited content or SDK numeric loss reject before
I/O. This is a Vertex request codec, not a Gemini API compatibility fallback.
Global ordering is source-proven, not inferred from completion order. Partial
state records increasing original part ordinals. Every native part has a scoped
part annotation containing its ordinal and, only when unsigned, original source.
Signed source is retained only in required opaque envelopes. Restoring a message
with signed partial state requires this provenance for every part, verifies
one-to-one source coverage and canonical projections, then sorts original parts
by ordinal. Missing/duplicate/edited provenance fails before I/O; no legacy repair.

Gemini native transport uses the isolated injected SDK client, invocation-local bounded source JSON, and the shared single-use stream lifecycle. It never falls back to sync or repeats completion after deltas. Complete function-call arguments are snapshots, not concatenated JSON chunks. Signed parts retain exact source state under the full checked destination. Candidate finish does not terminate transport: later usage-only events are accepted until EOF. Content or another finish after finish is a protocol error; missing finish EOF fails with accepted partial output retained. Malformed source and SDK body-bearing conversion errors expose static classifications, not provider payloads. Caller request/config objects are not mutated. Unsigned partialArgs now uses the provider-protocol assembler described below; signed incremental binding and complete expanded native conformance remain required, not waived by explicit rejection.

One SDK response part has at most one supported content variant: text/thought, complete function call, inline media or file media. A signature modifies that variant; a signature-only part is retained as an empty reasoning part with required opaque state. Multiple content variants are an invalid response, never switch-priority normalization. Code execution/function-response payloads, media resolution/video metadata/display names and incremental function-call metadata are unsupported by the current canonical mapper and fail explicitly rather than disappear. Persisted signed envelopes must remain required and undergo the same shape checks before continuation dispatch. This is a mapper limitation, not a claim about provider capabilities.

The persisted Gemini codec rejects duplicate JSON keys at every depth and requires SDK decode/re-encode to retain the same JSON keys and values. Case aliases, unknown fields, discarded null/zero fields and altered signature encodings are corrupt state, not candidates for permissive repair. Key order/whitespace may differ; arbitrary function-argument keys and exact JSON numbers remain caller-owned and must not be rewritten. This rule applies to the library-produced persisted codec, not a universal provider HTTP parser or a new SDK schema engine.

Task37 is a clear break. This document fixes decisions before implementation; unfinished APIs are explicitly design, not delivered features.

## Ownership

ID-less complete tool calls require explicit `tool_name_correlation` in execution requirements and supported effective-profile knowledge, plus an actual adapter name-correlation port. The one-shot helper accepts them only with checked destination/provenance/snapshot and supported tools/correlation decisions, and only when the call name is unique across the entire batch. Mixed ID/ID-less calls of the same name are ambiguous. No synthetic provider IDs or registry-based permission are introduced. Required capability cannot be manufactured by an override without the mapper port. Custom invoker diagnostics are a trusted host contract, not cryptographic proof or account authorization.

Core owns canonical execution, results, usage, portable requirements, provider state envelopes, wire formats, stream lifecycle and validator ports. Adapter modules own SDK data, profile mapping and transport hooks. Applications own account authorization, routing, scheduling, persistence, retention and egress policy.

Endpoint identity is not an authentication-bearing transport URL. Populated identity rejects malformed URI syntax, URL userinfo, query (including an empty query marker) and fragments. Use a stable secret-free endpoint reference; inject authentication and request-specific URL parameters through the SDK/host. Reject before copying an invalid descriptor endpoint into diagnostics or dispatching. Profiles, provider-state scopes and persisted response diagnostics enforce the same rule, returning static field errors without echoing values. Opaque endpoint references and URI paths remain host-owned; this is not a universal detector for secrets inside arbitrary strings, prompts or payloads.

Provider-derived usage sums validate nonnegative counters and canonical platform range before addition or narrowing conversion. An unrepresentable sum returns ErrUsage, retaining the known response and individually representable counters without advertising the derived counter as known. Checked execution preserves a parser's partial response alongside its error, stamps diagnostics, captures failed terminal metadata and never replays the request.

OpenAI SDK int64 counters are also checked individually before canonical int conversion. Invalid counters remain unknown without discarding other observed counters. Sync returns its known response plus ErrUsage; native streams accept the representable portion of the current frame, then fail with ErrUsage and release transport without replay. Provider capture metadata contains only representable counters; raw SDK JSON still carries the original fields, and canonical terminal records the parsing failure. Capture metadata extraction is not a substitute for checked parsing.

Realtime duplex, voice sessions, VAD, interruption/reconnect and audio synchronization are outside this execution contract. Request/response media mappings belong to adapters and must be described separately from input/output MIME profile knowledge; media input mapping never establishes output or realtime parity.

## Preflight and prepared request

Core `Invoker` is a host-owned canonical port, not transport attestation. Its implementation must establish required execution guarantees before dispatch or fail closed. Library-owned checked clients use shared Prepare; core structured/tool helpers preserve requirements and independently enforce their local schema/outcome/batch contracts. A custom wrapper should delegate unchanged executions to a checked client, not raw SDK Execute. The helper conformance inventory records thirty zero-Translate/zero-dispatch composition fixtures. Truthfulness of arbitrary host implementations is a port obligation; a separate proof-bearing interface is not required by the task and would not itself attest transport behavior.

Expand adapter.RuntimeDescriptor with immutable endpoint/model profiles and provenance. Requirements are checked against the effective model after defaults/overrides. Unsupported and unknown required capabilities fail before dispatch. An explicit provenance-bearing override may resolve unknown knowledge but cannot implement an absent method. Prepared requests retain the checked profile and schema/cache diagnostics. Raw SDK Execute remains a transport primitive rather than a checked canonical execution.

An effective destination has at most one descriptor profile. Duplicate matching profiles are ambiguous even if their current capability maps happen to agree; descriptor ordering cannot select guarantees. A valid explicit override is selected directly instead of the descriptor knowledge and must match the effective destination. Selected profiles require nonblank provider/endpoint/model and provenance, nonblank capability keys with recognized support states, nonempty duplicate-free forbidden combinations, and nonblank duplicate-free MIME entries. Supplied model-options schema documents are objects or booleans; semantic compilation remains the validator's job. Extension capability names remain caller-owned; no closed provider/model catalog is introduced. The same profile shape validator protects execution-wire overrides. Malformed profile errors expose only static field paths/reasons, never caller values. Unknown destinations with no profile remain conservative unknown rather than malformed knowledge.

An injected SDK client is opaque transport configuration. WithClient therefore does not establish any default endpoint identity. The caller must provide WithProfiles(actualEndpoint, profiles...) even for a plain checked request with no required capabilities; zero profiles is valid conservative knowledge. Without endpoint binding the descriptor has an empty endpoint and checked Prepare fails with ErrNoRuntimeDescriptor before translation/dispatch. Constructors without an injected client may describe a nominal default for inspection only; they cannot execute transport. Explicit binding is independent of option order and does not configure or introspect the SDK client. Caller assurance that the declaration matches transport remains a trust boundary, not a network probe or SDK-private-field dependency.

## Streams

### Forced local-tool selection

`ForcedTool` requires `forced_tool_selection` profile knowledge separately from ordinary `tools`, plus an actual `ForcedToolAdapter` mapping port. A profile override cannot implement that port; missing/unsupported knowledge or mapping fails before Translate/dispatch in every checked mode. OpenAI/Anthropic use their named native tool-choice fields. Gemini uses ANY with one allowed function name, not an added system instruction. Ollama has no such implemented mapping and rejects ForcedTool even on raw Translate; ordinary tools remain independent. This promises request mapping, not provider availability or infallibility. One-shot helpers still validate the entire returned batch against execution scope/ForcedTool before handlers. This is distinct from Anthropic's explicitly selected NewSyntheticOutput structured-output adapter, which does not execute a local handler or mix real tools.

The [Generate Content function-calling contract](https://ai.google.dev/gemini-api/docs/generate-content/function-calling) documents ANY and allowed function names. Mapping evidence uses the pinned SDK and local fixtures, not live endpoint certification.

### Logical identity

A stream owns one opaque local message identity, allocated at handle construction and stable across start/data/usage/finish/terminal and readable status. It is not a provider request/message ID and makes no physical-attempt or persistence claim. A content frame has one logical part reference per Content entry; sources must declare associations rather than core guessing from text adjacency. Repeated references across frames mean deltas of the same logical part; references within a single frame are unique. OpenAI content and refusal use distinct logical parts, and tool-index correlation binds a logical tool part to one provider call ID. Buffered content is a set of complete, distinct parts, not incremental native frames.

Message/part references must survive event cloning, consumer mutation, early-stop status and terminal tool materialization. Source-fragment offsets remain separate from logical part references and still govern exact state/annotation bindings. Materialization must retain the reference for a collapsed tool call rather than invent another identity; it must not merge opaque state-bound fragments to obtain this property. Missing/malformed/conflicting logical associations fail with ErrStreamProtocol and preserve the last accepted snapshot. A failed source cannot rewrite that snapshot. Canonical capture must carry the same references without reclassifying them as provider IDs or transport bytes.

`ResponseChunk.Identity` and `StreamStatus.Identity` use StreamIdentity: a nonempty local MessageID and PartIDs aligned with Content, or Result.Content for status/terminal. NewStream allocates MessageID without dispatch. Sources supply explicit PartIDs and leave MessageID empty; core stamps its own ID without mutating source-owned frames. Frame references are nonblank/unique and preserve canonical part type and tool-call binding. A tool ID cannot acquire another logical part reference. Validation of references, state associations and usage precedes an atomic accepted-snapshot update.

Text fragments remain separate canonical Content entries to preserve exact state/annotation associations; retained/terminal PartIDs can therefore repeat the same logical reference. Tool materialization collapses deltas and remaps references with the same source-to-result association used for annotations. The terminal frame's references describe Result.Content, not its empty Content field. Neither consumer nor observer mutation changes subsequent events or status.

WithStreamFrameObserver runs on a cloned, identity-checked accepted frame without the status mutex. Its failure retains/yields that frame, releases the source and fails the lifecycle without replay. WithStreamCaptureObserver now receives StreamStatus, including references, after finalization; the old Response-only signature is removed. Native and explicitly buffered canonical captures include stream_identity and belong to the original handle's single terminal. Buffered provider observation remains a synchronous provider response, not a native chunk claim.

ObserveLifecycle attaches lazy middleware to that same handle. Start returns a context containing additional values but retaining the same cancellation channel, plus a mandatory closeout callback. Callbacks run in nested order on cloned final/canceled status without a held status mutex. Detached cancellation, nil context/callback or attachment after consumption fails explicitly. Source, finalizer and capture see the observation context. Tracing uses this port instead of reconstructing a second stream; it preserves original identity/final validation and known sync response on error.

Core and actual SDK fixtures cover transport segmentation, text/refusal separation, interleaved tools and terminal remapping, malformed/conflicting refs with atomic snapshot retention, same-client parallel handles, cloning, consumer stop, observer failures/unwind, lifecycle context/order/cancellation, native/buffered capture references and one closeout. These are local conformance evidence, not live endpoint/model claims or independent final audit approval.

### Implemented lifecycle

Consumer stop cancels the producer immediately but keeps an otherwise-live invocation context for one capture closeout after source unwind. It never yields again and skips whole-document finalization. Existing frame-observer failures and closeout errors remain alongside cancellation in final status. Close on a running handle is a nonblocking cancellation request: final immutable status is published after the iterator's unwind/closeout, not prematurely. Close before consumption immediately cancels the created handle without dispatch. Close/caller cancellation must still cancel observer work; no detached context or background recorder is added.

Invoker.ExecuteStream returns a single-use Stream handle. Explicit native/buffered mode is required. The handle exposes Events, Status and Close. Dispatch is lazy; Events owns one execution and a child context. No background producer or unbounded queue is required. Ordered data/control events accumulate partial results and usage; a provider finish marker is not a terminal until trailing usage has been consumed. Consumption exhaustion, transport failure and early consumer stop produce exactly one completed/failed/canceled status. Status snapshots are cloned and remain readable after Close. No completion retry after partial output.

Tool identity is request-local candidate/index plus provider call ID, never function name or adjacency. Complete args and incremental deltas have distinct meaning. Final args must be a JSON object before execution. Providers without IDs receive stable local correlation only when the adapter can prove association.

The native stream's checked logical PartIDs are authoritative local correlation for ID-less tool fragments. Materialization may merge fragments with that same source-declared reference while leaving the provider ID empty; distinct references remain separate calls even with equal names. Standalone AssembleToolCallParts has no such proof and still rejects ID-less JSON deltas. Local and provider identities use disjoint namespaces. Argument snapshot/delta conflicts and incomplete final objects remain errors under either identity; this does not authorize name-only execution or handler admission.

Gemini partialArgs is path/value protocol data, not JSON text concatenation. Decode the original source value with Number; presence distinguishes false, empty string, null and absent closing marker. String continuation appends only to the same open path; final markers close it, and completed paths cannot be overwritten. Build nested objects/arrays without allocating an array from an untrusted index; reject conflicting shape, missing array entries or open strings at completion. Progress must remain provisional and correlated until the provider closes the function call. No handler executes a prefix, and no synthetic provider ID is introduced. This protocol accumulator stays in the provider adapter, reusing common final correlation/schema validation.

Native partial-function progress is carried by a ToolCallPart with empty Args and an exact `gemini.partial_function_call` PartAnnotation containing the source functionCall object. It is not mislabeled as ArgsChunk JSON text. On provider close, the same logical part receives one complete Args snapshot and common materialization/schema validation applies. ID-less continuation is valid only with a unique open ID-less call; named starts cannot silently replace it. Provider-ID calls correlate independently and can interleave. Signed incremental calls retain original ordered fragments/ordinals/signature positions in the separate partial-parts codec. Actual Vertex SDK continuation uses those original parts, never a re-signed or synthetic merged call. Missing/corrupt/provisional/edited state fails closed; final independent acceptance is still required.

OpenAI tool indices and provider IDs have a one-to-one binding within a request: changing the ID/name of an established index or reusing its provider ID at another index is an invalid response, not permission to merge calls. The assembler reports conflicting names/snapshots and ambiguous snapshot-plus-delta parts with ErrIncompleteToolCallArgs and static diagnostics. A call chooses snapshot or incremental mode at its first nonempty argument frame and cannot mix these representations across later frames, even if concatenation would accidentally produce valid JSON. Empty identity-only frames choose no mode; repeated equal snapshots are idempotent, not concatenated. Translate rejects simultaneous Args and ArgsChunk even when the strings are equal; there is no legacy equal-value shortcut.

Native OpenAI choice index is present and zero, and every tool delta carries a present nonnegative index. SDK scalar defaults do not establish presence: missing/null choice or tool indices are invalid responses. A continuation may omit ID/name only after its explicit index was bound. Negative/ambiguous choice/tool data, conflicting ID/name and uncorrelated continuation fail with ErrInvalidResponse and static diagnostics. Real SDK/body negative fixtures prove cleanup and no replay; a same-client concurrent SSE fixture blocks both consumers after initial identity before allowing continuations with the same provider ID/index but different function/arguments, proving request-local correlation with race checks.

Actual SDK/body conformance covers consumer break, caller cancellation and Close while unread deltas remain, plus a read failure after two accepted deltas. Each path closes the response body and dispatches once even when the injected SDK has retries configured. Consumer stop yields no later events; status retains canceled partial content. Source read failure retains its error identity and partial content in failed status. These local transport fixtures do not assert live endpoint/model support.

## Outcome, usage and state

Canonical outcomes distinguish completed, tool_calls, refusal and incomplete. Transport/protocol error and canceled stream state are distinct from provider outcome. Known/partial/unknown usage and per-counter presence prevent interpreting missing data as zero; cumulative replaces and delta adds exactly once. Cache and reasoning counters are breakdowns, not extra total tokens.

OpenAI declares completed only for the recognized stop reason; unknown/missing reasons map conservatively to incomplete, retaining source reason and content. Native missing-finish EOF remains a protocol failure; an observed unknown finish can complete transport with an incomplete provider outcome and still receive trailing usage. Structured validation must not promote either case to successful output.

Runtime and wire usage accept only nonnegative counters, unique recognized Known entries, and empty/cumulative/delta Mode. Empty mode is cumulative intent. A nonzero scalar whose presence is unknown may retain a provider's partial estimate, but never establishes Known. No universal prompt/completion/breakdown sum is inferred. Accumulation checks integer overflow and commits an entire update atomically; invalid usage fails the stream as protocol/usage error without changing previously accepted partial content or counters. Sync metadata validation retains the known response with an explicit error, never retries or pretends the provider was not invoked. Marshal and restore reject the same invalid metadata; structured helpers cannot accept malformed canonical metadata from a custom invoker.

Tool materialization maps every source part index to its assembled logical position. Annotations follow this mapping, including later text after collapsed interleaved deltas. Opaque state follows only if its associated content is unchanged; assembly of a state-bound tool delta fails closed. Unrelated state/annotations do not prevent other calls from assembling. All resulting associations are revalidated before replacing the accumulated partial result.

Reasoning controls remain provider-specific keys of the existing ModelOptions.ProviderSettings. RuntimeDescriptor identifies which setting keys require reasoning-profile knowledge. A profile's optional ModelOptionsSchema is an executable constraint on the complete ModelOptions object, scoped to that exact effective destination; it may constrain modes, enum values, budgets, visibility and combinations with temperature. Its existing ForbiddenCombinations rules receive the `reasoning` feature whenever these settings are requested, so tools/output/native-stream combinations can also fail before dispatch. A requested reasoning key without this schema is unknown and fails before translation/dispatch. Supplied schemas always validate options, with explicit caller validator/resources or the bundled non-network validator. Diagnostics and typed capability errors do not echo option values. This does not establish provider-wide or SDK-wide support; the actual module mapping must also exist. No model-name heuristics or silent option removal are permitted. OpenAI MaxTokens maps to max_completion_tokens consistently, including exposed and non-visible output, not a prefix-dependent legacy field.

Provider envelopes carry a format marker, provider/endpoint/model-family scope, part association, opaque bytes and optional expiry. They cannot contain credentials. Response wire and continuation projection preserve order/state through history, clone, normalization and execution wire. Persistence requires explicit host policy; logging excludes state by default. Unsupported/corrupt formats and incompatible routing fail explicitly.

### Wire/state implementation decisions

- Extend the existing execution wire instead of introducing another execution DTO. Both execution and response wires must contain an exact checked format discriminator; missing/foreign discriminators are migration errors, not legacy recovery.
- State association is explicit per canonical part index within a message/response. Signature-bound content cannot be merged or reordered by Normalize, AddMessage or scratchpad projection. Malformed/out-of-range associations are rejected.
- Each opaque envelope carries its own format discriminator, provider destination scope, codec, cloned bytes, continuation-required flag and optional expiry. Account scope references are nonsecret caller data, never credentials. Provider modules validate payload shape; core validates scope/association and never interprets private provider bytes.
- Response-to-history projection preserves ordered parts plus their associations. Checked high-level execution validates required state against the selected destination before translating or calling transport. A provider module must explicitly advertise and implement state translation; a knowledge override alone cannot create this implementation.
- Persistence projection requires explicit preservation consent. Logging projection never includes opaque bytes by default and must indicate any omitted required state; it is not reusable continuation wire. Canonical media/prompt/error content remains subject to caller redaction policy rather than a claimed universal secrets detector.
- Wire restoration does not serialize runtime ports: a restored execution must rebind its validator, transport and capture policy when their guarantees are required. Portable requirements themselves survive wire round-trip.

The contracts live in `provider_state.go`, `response_wire.go`, `wire.go` and `schemas/`. Gemini state fixtures exercise parse → response wire → restore → continuation execution wire → actual SDK translation, including signed incremental parts and original global order. Responses, batch and cache handles enforce explicit account/destination bindings; reasoning settings require destination-scoped executable profile constraints and actual adapter mapping. This evidence does not establish untested provider/model/state combinations or live account acceptance. Final paired audits remain mandatory.

## Schemas

Final original-schema validation distinguishes `ErrSchemaJSONDecode` (not one complete JSON document, preserving its decoder cause) from `ErrSchemaMismatch` (valid JSON violating source constraints). Invalid source schemas remain pre-dispatch `ErrSchemaUnsupported`. Refusal/incomplete are provider outcome errors and are checked before JSON validation; no implicit repair/retry is introduced. Canonical capture classifies result JSON decoding as `parse`, including errors wrapped by structured helpers, without exposing decoder text.

The one-shot structured helper returns original-validation JSON decode failures directly, not an ordinary repairable `ValidationError`. This prevents its executor wrapper from appending domain feedback for an invalid/truncated document. Incremental syntax/truncation still uses `ErrStructuredStream` and retains decoder causes when available; provisional items never establish final success. Valid-JSON schema mismatch and post-decode domain validation remain separate validation paths.

Each output/tool definition carries an optional `Dialect`, persisted as `dialect` in execution wire. Empty selects the bundled default (`SchemaDialect2020`); explicit `SchemaDialect7` is supported. A declared `$schema` must match the selected dialect. Checked preparation, one-shot structured output, structured streams and one-shot tool validation pass that selection to the validator port rather than silently substituting the default. A custom compiler is a trusted application port: compilation must fail when it cannot enforce the selected contract and may not return a nil validator as success. The bundled validator does not fetch implicit resources; format/content annotations do not become additional application business assertions.

OpenAI strict-output object checks descend into both schema-form and tuple-form `items`, `additionalItems`, and schema-form `dependencies`. Property-dependency name arrays are data, not schemas. Nested failures and permitted transformations retain source JSON pointers; traversal never treats arbitrary annotation objects as schemas. This is mapper fidelity, not a declaration that a destination accepts every schema keyword.

Schema-valued `additionalProperties`, `unevaluatedProperties` and `unevaluatedItems` are also traversed before object closing. Annotation values (including examples/default/enum instances and content annotations) are not generation-target schema children. The mapper checks these canonical structures without claiming live acceptance of every applicator by every endpoint/profile.

Strict fidelity rejects unsupported semantics before dispatch. Explicit lowering records JSON-pointer diagnostics and validates the result against the original immutable schema using an injected validator. A supported dialect and no implicit remote reference loading are mandatory. Domain Validate runs after schema validation. Native JSON, JSON-only, forced tool and caller validation have different guarantees. Refusal/incomplete must not enter JSON repair paths. Stream items are provisional until document validation and terminal success.

Anthropic response schemas map directly to `OutputConfig.Format.Schema`, not an injected tool. Tools and output format remain separate request fields; only explicit `ForcedTool` selects a tool. A user tool named `output_format` has ordinary tool-call semantics, never implicit structured-output semantics. Native support still requires a caller-owned capability profile on checked execution; the SDK field alone does not establish model support.

Provider schema mapping must reject unsupported keywords or shapes with a typed `SchemaFidelityError` carrying an RFC 6901 source pointer. Map traversal is deterministic. The current Gemini typed-schema mapper supports only its explicitly mapped subset; numeric enum, constraints, references, boolean schemas and type unions cannot be silently discarded. Such rejection describes the implemented mapper, not a provider-wide claim. Explicit lowering will be a separate opt-in contract, never an automatic fallback from strict failure.

Builtin schema compilation failures expose `SchemaCompilationError`, preserving ErrSchemaUnsupported and the original engine/decoder cause. Locations identify the input document (empty Resource for source; explicit URI for resources) and RFC 6901 source pointer, including known root as empty pointer. Metaschema errors use instance locations. Diagnostic-only engine metadata verifies active reference origins, excluding annotations and unreachable definitions; target URIs and generic JSON candidates are not source evidence. Locations are sorted/deduplicated; unknown positions stay empty, never fabricated. Original source/cause and zero network are retained. Checkpoint5 found enum-data collapse (IA08) and excessive diagnostic work (IA07); current regression corrections are not final independent acceptance. Malformed JSON can still have unknown position.

`CompileProviderSchema` owns the universal compilation guard, not provider mapping. An injected `ProviderSchemaMapper` receives cloned source/profile/role/policy data and returns canonical JSON Schema plus transformation diagnostics. Strict (including zero policy) requires structurally unchanged JSON and no transformations. Lowering requires an explicit allowlist of transformation IDs, a nonempty report for any changed schema, and an original-schema validator. The immutable compiled result retains exact-byte SHA-256 digests and cloned source/translated documents; original validation remains mandatory before accepting lowered results. This port is trusted adapter code, not a generic proof of a provider's schema semantics. Checked adapter binding is described below.

Portable policy is `execution.Requirements.Schema`; it survives execution wire, clone, normalization and history additions. Runtime validator instances are not serialized. The executable wire contract rejects unknown modes, duplicate permissions and transformation permissions outside explicit lowering. Provider mapper/prepared binding and final-result integration enforce this intent; a portable policy field alone does not establish provider/model lowering support.

Checked `Prepare` now requires an actual mapper port for output/tool schemas, independently of knowledge overrides. It compiles originals before Translate, maps only cloned working documents, stores validators and profile/role/name-associated digest reports in the prepared request, and attaches cloned reports to execution diagnostics. Sync completion validates output/tool arguments against originals; post-dispatch failure preserves the known response and never retries. All four provider modules implement mapper ports with their documented supported subsets; native-stream finalization is described below. This is not a claim of all model/profile schema support. Raw Translate/Execute remain lower-level mapping/transport primitives, not substitutes for the checked compilation path.

OpenAI output mapping now has two explicit transformations: `openai.close-object` replaces absent/allowed additional properties with `false`; `openai.required-nullable` replaces an optional property with a required `anyOf` of the entire original property schema and null. Neither is lossless. Nested schemas are visited before generated wrappers, so reports retain original source pointers. Strict output does not mutate schemas; raw Translate requires an already strict-compatible schema rather than silently lowering. Checked execution owns original validation and reports, including native-stream finalization. Tool schemas are passed through without constrained-output normalization; the original tool-args validator still applies.

Ollama tool schemas are checked against actual SDK decode/serialization before Translate or dispatch. Every source field must survive structurally, with exact numeric values. Loss or rounding is a typed source-pointer error, not silent lowering. Only empty properties and the SDK default object at the root are tolerated as the existing object-only tool-args contract; all other additions are rejected. Output schema and lossy lowering remain explicitly unsupported. This conservative SDK representability check is not a catalog of model capabilities or a live-provider conformance claim.

Native checked streams install `WithStreamFinalizer` with the prepared original validators. The callback runs only after normal EOF, trailing usage and complete tool-argument assembly, before terminal success; it receives an isolated result snapshot without a held status mutex. Consumer stop/protocol/transport error skips finalization. Concurrent Close cancels its context and retains canceled state. Original-schema failure produces failed terminal/status without losing accumulated usage/content/diagnostics or dispatching a second completion. Buffered post-dispatch validation failure similarly retains the known response rather than presenting an empty failed execution.

Structured stream parsing attaches before single-use consumption to the same handle. Incremental content validation runs after preserving the received chunk and before exposing provisional items; failure cancels source transport without canceling terminal observation, giving one failed terminal/status rather than consumer-canceled status. Parser completion and original document validation run before terminal success and compose with the existing prepared finalizer. Refusal/incomplete takes precedence over document-success interpretation. Consumer break still cancels without a new yield; previously yielded items are not revoked.

Incremental syntax/shape errors carry ErrStructuredStream and preserve the decoder's typed cause when available. Caller-owned Validate failures instead use the existing ValidationError with the original error chain intact; they are not JSON syntax errors or cancellations. Stream domain failures have no fabricated full assistant message. Capture records only a safe validation classification, not caller error text. Original-schema failures remain distinguishable via ErrSchemaMismatch.

## Gemini response-error privacy

After a completely observed HTTP 2xx response, SDK decoding/conversion failures return static ErrProviderState without the body-bearing SDK cause. This boundary does not depend on successful JSON parsing or detecting a signature: malformed unsigned/unknown bodies are private too. Caller cancellation/deadline remains independently identifiable without retaining raw SDK error text. Incomplete body-read and transport/host-policy failures retain their original causes. Observation overflow does not permit response-error disclosure or a replay.

The SDK converter can also panic on an invalid provider shape after successful response EOF. Only that observed-response boundary is isolated as a typed safe failure; the panic value is not retained or logged. A caller transport panic before complete HTTP 2xx observation is not converted to a provider response error. No replacement SDK/schema decoder is introduced.

## Scope and media safety

OpenAI Chat Completions projects exactly one choice at index zero. Refusal is a separate canonical text part annotated `openai.refusal` with its original string value, including an explicitly present empty string; it overrides the stop reason's outcome. Exposed message annotations retain their full provider JSON and bind to the content text part as `openai.annotation`. Annotations without a content part are rejected rather than assigned to an unrelated tool/refusal part. The SDK's separate content/refusal fields project in content-then-refusal order; this is not a claim of interleaving unavailable on that API. Native refusal deltas use the same part annotation and request-local refusal outcome, including when the finish event arrives separately and usage arrives later. No hidden reasoning is reconstructed.

Anthropic signed `thinking` and opaque `redacted_thinking` blocks project to ordered ReasoningPart values and required `anthropic.thinking.json` envelopes bound to the effective request destination. Redacted data is never canonical text. Restore checks exact block shape, nonempty signature/data, canonical reasoning text, assistant role and absence of cache controls on thinking. Unsigned reasoning cannot be manufactured into signed history; missing/omitted state fails closed. Text citations retain full provider JSON on their own text part. Known stop reasons map to completed/tool_calls/refusal/incomplete; unknown reasons are incomplete, never success. Usage is cumulative; prompt totals include uncached input + cache read + cache creation only when all three are known, and total is derived only with known output. Missing cache counters cannot become known zero.

The thinking codec accepts exactly the case-sensitive keys `type`, `thinking`, `signature` for signed thinking, or `type`, `data` for redacted thinking. Duplicate keys (including escaped equivalents), cross-variant fields even when null, case aliases and unknown fields fail closed. Required values must be strings, not null; an empty signed thinking string remains valid. JSON whitespace and key order are not part of the semantic SDK continuation guarantee; signed string values are preserved.

Gemini signed-state codec precision is separate from transport representability. The current SDK internally decodes request numbers through float64. Before translation, the state validator checks that this encoding preserves exact JSON numeric values; otherwise it returns ErrProviderState with no dispatch, rather than sending rounded args with the original signature. Numeric spelling may change without changing value (for example 1.00 and 1); caller keys and saved payload remain untouched. This is an adapter SDK limitation, not a restriction on portable core JSON numbers or a live-provider capability claim.

Gemini response ingress must decode signed parts from an authoritative original response payload, not a float-normalized SDK DTO. The adapter may observe one response through an isolated copy of the injected public SDK/HTTP configuration; caller transport/auth/redirect/timeout policy remain unchanged, no SDK-private inspection or endpoint inference. Observation is per invocation with a bounded buffer and no queue/persistence/replay. WithResponsePayloadLimit selects a positive byte bound (default 4 MiB). Incomplete/oversized observation cannot certify a signed part. A raw caller-supplied DTO containing signed float args without original payload fails closed rather than being presumed lossless; exact caller-owned Number/integer values remain semantic DTO input. Original signed JSON keys/values and association must match the SDK-known part; malformed or incompatible payload fails with ErrProviderState, not repair.

A signed reasoning block uses its explicit part policy, or inherits the message default when its own policy is nil. A valid effective `CacheOmit` is absence of a marker, not a signed-payload edit. All other nonnil effective policies on signed blocks fail before dispatch. A message marker is not itself a signed-block control: explicit omission on those blocks may suppress it while other blocks inherit normally. A fully overridden default is not_applicable and needs no profile support. Portable policy shape remains validated even for unused defaults. No implicit-cache guarantee or change to thinking/signature/redacted data is introduced.

Validate an entire local tool batch against execution.Tools and ForcedTool before any handler. Empty Tools means no calls are allowed; registry membership only describes available handlers. Nonempty call IDs must be unique; idless calls require safe profile correlation. This is contract enforcement, not a user permissions service.

MediaFetcher has an instance HTTP client. HTTPS is mandatory on the original request and every redirect before dispatch; caller redirect restrictions remain effective. Client injection owns network/address/proxy policy. Fetcher does not mutate caller clients or global transport settings.

Redirect limits follow the injected client's CheckRedirect when supplied; without it, the standard ten-request limit remains. The HTTPS invariant applies before every redirect independently of that limit. Context cancellation between hops prevents the next request. Separate instances retain their own caller policies under concurrent use. Positive body limits accept exactly the limit and reject one byte more; arithmetic at the int64 boundary must not wrap into an empty successful download.

## Capture and cache

A message CachePolicy is an inherited default, not an independent SDK block. If every top-level content block has its own explicit policy (including omit), or there are no blocks, the default is not selected: report not_applicable at its original path and replace it with omission in the prepared clone. Do not require profile/control support or include unused defaults in combinations. If even one block inherits it, normal required/advisory checks still apply. A tool-result block itself inherits its message default unless it has an override; child overrides do not suppress the enclosing block's marker. Portable malformed policy shape still fails even when unused. This determines canonical inheritance only, not provider support for a part type.

CachePolicy `mechanism: "omit"` explicitly suppresses an inherited block marker. It has no provider, mode or controls, makes no cache-hit/bypass/deletion/implicit-prefix guarantee and needs no remote capability. Nil still means inherit, not suppress. Checked removal of unsupported/conflicting advisory controls and accepted implicit-prefix intent uses this explicit omission rather than nil, so Translate cannot reactivate an ancestor's marker. Diagnostics retain the original intent/disposition; source execution remains unchanged. The omission policy is portable and survives wire/clone/Normalize, not a hidden runtime flag.

Selected cache mechanisms and control keys participate in effective-profile ForbiddenCombinations alongside tools/output/native stream/state/reasoning requirements. Individual support is insufficient for a combination. First omit all advisory policies involved in forbidden combinations, reporting unsupported without choosing a preferred advisory subset or routing policy. Reevaluate the remaining selected intent: a required conflict fails before Translate/dispatch with ErrCacheRequirement and ErrUnsupportedCombination. Required controls compatible after advisory removal and independent selected policies remain. Use stable source paths and static reasons, never control values. Check only selected policies: already omitted unknown/unsupported/foreign advisory hints cannot introduce a combination failure. Mapper validation receives isolated policy/controls and cannot rewrite the intent subsequently enforced or translated. This is request preflight, not a cache-resource lifecycle.

Capture reports are semantic snapshots, not arbitrary persistence flags. Invocation identity is nonblank; coverage has unique recognized stages/accuracies, and observed stages are unique and covered. Closed is true exactly when a terminal record was acknowledged. Complete requires Closed and no recorded failures; it does not assert every available stage was requested. Failure stages must be recognized and classification nonblank; an unsupported stage can fail without being in coverage. Incomplete/open/failed snapshots remain persistable. Runtime response metadata validation and both response-wire directions reject contradictions with ErrCaptureReport. A report's validity is not proof of durable persistence or crash recovery.

CaptureProjection.Metadata contains only observed response identity, optional HTTP status/request ID, usage presence/outcome and schema digest associations. ProviderResponseID is not RequestID. Missing HTTP status remains nil; SDK JSON hooks cannot infer it. Schema associations omit endpoint/provenance/raw schema values. Metadata is part of the host-approved projection: a projector may omit/redact it; default projection omits response/request IDs and retains isolated usage/outcome/digests. Mutable pointers and slices are cloned before projector and sink calls. Usage validity is checked before delivery. No metadata implies transport coverage or a physical attempt identity.

Capture uses a caller-owned synchronous sink and an explicit projection port, not a storage backend. Each invocation has a generated identity and monotonic sequence; correlation is caller-owned nonsecret data. Stage coverage and maximum accuracy are declared independently from model capabilities. DTO/SDK JSON is semantic capture, never transport bytes. Unobservable physical attempts have no invented ID; required physical-attempt coverage is rejected before dispatch. Sink durability is snapshotted before dispatch and every requested durable acknowledgement is checked.

The default projection is metadata-only: payload and endpoint are omitted, redaction/incompleteness marked. Full prompt/raw output/state requires an explicit host projector; no auth headers are supplied by the generic path. Projectors receive isolated data and must not mutate execution or imply byte accuracy after redaction/truncation. Payload size is bounded, callbacks apply synchronous backpressure and must honor context cancellation. There are no background queues; ports must be safe for concurrent invocations and must not re-enter emission on the same session. Typed capture errors distinguish pre-/post-dispatch; post-dispatch errors retain known provider results and never trigger replay. Advisory failures are recorded without turning execution into failure.

Checked execution now binds these ports: `Requirements.Capture` is portable intent; `execution.Capture` is a cloned runtime binding and never persisted. Required stages/accuracy/durability/attempt coverage fail before Translate/dispatch when unavailable. Canonical execution is snapshotted before provider schema lowering; translated request is a separate SDK JSON projection. Sync provider response uses exposed RawJSON when available, otherwise explicitly semantic SDK DTO JSON. Native provider chunks require an actual declared SDK hook; OpenAI supplies one before normalization. Stream usage options are applied by a pure native request preparer before translated capture. Canonical frame capture uses checked wire projections, including exact state associations and partial usage, not arbitrary JSON of interface-valued parts. Unknown-outcome partial snapshots are non-continuable execution wires rather than fabricated completed/refusal outcomes. Canonical terminal capture follows assembly and original validation, and carries a safe error category. Post-dispatch capture errors preserve known parsed parts/outcome/usage and join original errors without replay. Diagnostics capture reports survive response wire and clone without runtime ports. Generic transport bytes/physical attempts remain unsupported; model knowledge overrides cannot install missing hooks.

Optional capture records canonical/translated/provider stages and actual supported transport/physical attempt coverage. Unsupported required stages fail before dispatch. Required sink failure before dispatch prevents execution; after dispatch it preserves execution outcome and never triggers retry. Projection excludes credentials and records redaction/completeness; no durable ack without sink capability.

Cache controls separate advisory/required intent, accepted/unsupported/not-applicable/unknown disposition and provider-specific options. No cache hit promise. Resource handles are scoped and optional; remote timeout can be unknown rather than justification for a second create. Scheduler and application response cache are external.

The replacement CachePolicy carries `mechanism` (`implicit_prefix` or `block_marker`), `mode` (empty/advisory or required), optional `provider`, and caller-owned JSON `controls`. Controls require a provider, cannot be reinterpreted for another provider, and need explicit model-profile knowledge for each key. Block markers require an actual provider mapper as well as supported profile knowledge. Required unknown/unsupported/not-applicable policies stop before dispatch; advisory policies are removed from translation and reported with deterministic source paths. Diagnostics contain neither control values nor prompt content. Accepted means request controls were selected, not that a remote cache was hit or data persisted. Gemini resource lifecycle is separately selected and implemented, not implied by either mechanism. Message inheritance and part overrides retain their existing explicit locations; incompatible policies prevent Normalize from merging messages rather than discarding a required policy. Whole-request RequestCache and CacheResource survive Normalize independently.

## Completion

Message-level provider metadata uses `MessageAnnotation{Kind, Value}` separately
from `PartAnnotation`. `Response`, `ResponseChunk`, assistant history and both
wire envelopes carry `MessageAnnotations`. Values are valid JSON, kinds nonblank;
cloning/persistence preserve exact values and order. Stream updates append in
source order, including metadata-only events after provider finish. They never
create content, infer a content-part index or reinterpret provider-relative
indices as canonical part indices. Terminal result/capture/partial failure retain
the accumulated annotations. Metadata remains subject to host redaction policy.
Gemini candidate citation/grounding metadata belongs here; each stream update
retains its source JSON and provider indices as an ordered metadata update, not
an invented merged snapshot. Grounding chunk indices reference provider stream
history, not prompty content-part indices. Consumers resolve them using ordered
provider updates. Provider association validation stays in the provider adapter;
the core validates only the universal envelope.

Reference origins are enriched only from the bundled engine's compiled metadata, never error strings or guessed target-as-origin. Missing resources use a diagnostic-only false-schema loader. Fragment/anchor diagnostics index generic JSON copies but probe only actual edges from an all-neutralized engine graph. A compiler-only observation interrupts after builtin resolution, before queued whole-resource processing; its private abort cannot become a caller error or validator. A separate stable all-neutralized compiler checks the engine-resolved physical target and discovers further edges, including referenced extension schemas. Restore valid/inconclusive candidates and build a fresh final root graph to project active failed origins. Assertions/annotations/unreachable definitions are data, not assumed schema nodes; injective placeholders retain equality distinctions. IDs, URI resolution, anchors and dialects stay engine-owned. Original source/resources/cause and zero network remain authoritative. Diagnostic metadata and graphs never reach a mapper/provider or validate caller data. Unknown origins remain unknown. The observation pipeline is current parent evidence pending paired audit, not final acceptance.

Normal native capture observation follows source shutdown, argument assembly and final original-schema validation, but precedes terminal delivery. Consumer early stop skips assembly/whole-document finalization, cancels and unwinds the producer, then invokes capture closeout once without another yield. It receives an isolated canceled partial snapshot and can update only capture diagnostics, not canonical content or provider outcome. An acknowledged canceled terminal may produce a Closed report; Complete describes capture acknowledgements, never successful execution. Failed/redacted/canceled-context observation remains visibly incomplete rather than fabricating persistence. Source cleanup cancellation is distinct from invocation cancellation: transport/protocol errors and consumer break release the producer without canceling the context needed for bounded terminal observation. Close/caller cancellation still cancels both source and observer. Updated report and joined frame/closeout errors are installed before immutable terminal status. Native SDK JSON hooks observe chunks before normalization, never claim SSE/HTTP byte coverage, and preserve a successfully parsed chunk before reporting required sink failure.

All requirement rows need implementation evidence or explicit trigger disposition. Final independent audits evaluate the frozen original task and this matrix. make lint and make test cover all modules. Release and issue closeout are prepared but not published without separate authorization.
