# OpenAI adapter for prompty

## Whole-request cache controls

PromptExecution.RequestCache selects prompt_cache_key and prompt_cache_retention
on Chat Completions or Responses Create. Use implicit_prefix with provider openai
or openai.responses respectively. Retention accepts in-memory or 24h; key is a
nonblank caller-owned string. These fields are separate from message block markers
and managed cache resources. Declare cache_implicit_prefix and each selected
cache_control:<key> on the actual endpoint/model profile. Required unsupported or
unknown intent fails before dispatch; advisory intent is omitted with diagnostics.
Neither SDK field availability nor accepted settings guarantees a hit, durability
or savings. There is no bypass/delete/TTL emulation or content-derived key builder.
Get/Cancel do not inherit generation cache intent. Credentials and reuse policy
remain host-owned. ExampleAdapter_requestCache is a local SDK runnable example;
request_cache_test.go covers HTTP mapping and native/buffered usage composition.

## Native file-backed Batch

NewBatches(accountBinding, files, opts...) uses the native Batch SDK with checked
Chat Completions requests, not background Responses or synchronous fallback.
NewBatchFiles(accountBinding, endpoint, sdkClient) supplies optional SDK-backed
upload/download transport; a caller implementation of BatchFileTransport may
replace it with the same explicit provider/endpoint/account binding. Nil or foreign
file ports fail before upload. Credentials/egress/redirect policy remain on host
clients; no file store, cleanup daemon or implicit delete is introduced.

Declare separate batch_submit/batch_inspect/batch_cancel/batch_results capabilities
on the actual OpenAI endpoint/model profiles. A Batch input has one model; duplicate
IDs, mixed models, invalid item/profile/schema/state/cache/capture contracts fail
before upload. This port targets /v1/chat/completions with a native completion
window; it does not declare embeddings/Responses batch or other endpoints supported.
The protocol basis is [OpenAI Batch](https://developers.openai.com/api/docs/guides/batch).

Input JSONL is generated from isolated checked SDK requests. Each item requires a
caller ID and AttemptID, retains OriginalID/ParentAttemptID and an input digest.
Upload purpose is batch. Known uploaded file IDs remain in BatchResult.Resources
if later capture or submit fails; SubmissionAttempted distinguishes actual job
submission from file upload. ErrBatchFileUnknown and ErrBatchUnknown are separate;
neither permits blind retry. SDK mutation retries are disabled. No file is silently
deleted after failure; host owns reconciliation/cleanup and provider retention.

EncodeBatchHandle/DecodeBatchHandle persist scoped item bindings, resources and
both canonical status and original ProviderStatus. Get/Cancel do not require file
ports. Results requires original BatchResultBinding executions/validators and a
compatible files port; changed contracts fail before download. Both error/output
file references are read in handle order. The original line JSON, provider request
IDs, per-item errors/expired/refusal/incomplete and presence-aware usage survive;
hosted/file handlers are not invoked. Unsupported cancellation item codes are not
invented from a canceled batch status. Missing items stay visible.

DownloadComplete/CorrelationComplete are distinct from remote completion and item
success. Missing/duplicate/unexpected IDs and read/close/limit failures retain
observed items/known remote status. MaxLineBytes defaults to 4 MiB and MaxBytes to
64 MiB across all downloaded files; source input has an independent bounded size.
Semantic capture distinguishes the upload plan, known upload response and actual
batch request, with common redaction/acknowledgements and one terminal. It is not
HTTP-byte/physical-attempt evidence. Required capture failure after upload can
leave a known file without a batch and never proceeds to create the job.

ExampleNewBatches and local multipart/SDK/JSONL fault fixtures are runnable. Live account/model support remains unverified. The current [matrix](../../docs/provider-capability-matrix.md) links all implemented batch ports; task37 reports retain their historical checkpoint.

## Separate Responses operations

Responses accepts its own allowlisted `ProviderSettings.reasoning_effort`
(none/minimal/low/medium/high/xhigh) and `reasoning_summary`
(auto/concise/detailed), validated by the endpoint/model profile's
`ModelOptionsSchema` and `reasoning` capability. Chat settings are not inherited.
Unsupported combinations or unknown controls fail before dispatch. Summary is
independent of encrypted provider-state inclusion; deprecated generate_summary
is rejected. See [reasoning controls](https://developers.openai.com/api/docs/guides/reasoning).

The native strict local-tool subset accepts object root, closed objects and all
properties required; nested schemas use type/$ref/anyOf. Allowed keywords:
`$schema`, `$id`, `$ref`, `$defs`, `definitions`, `type`, `title`, `description`,
`properties`, `required`, `additionalProperties`, `items`, `anyOf`, `enum`,
`const`, `format`, `pattern`, `minimum`, `maximum`, `exclusiveMinimum`,
`exclusiveMaximum`, `multipleOf`, `minItems`, `maxItems`. Unknown keywords and
boolean child schemas are rejected. This mapping boundary does not certify that
every model supports each constraint; the host verifies its profile. Strict mode
requires original compatible schemas, never hidden closing/nullability changes.
See [native strict tools](https://developers.openai.com/api/docs/guides/function-calling).
Run `go test -run ExampleNewResponses_strictReasoning` for an offline actual SDK
example with original-schema rejection before dispatch.

Local ToolDefinition inputs now map to native function declarations alongside
hosted search. ForcedTool uses native named choice with its separate capability.
Tool schemas remain original with explicit strict=false by default. Set
`RemoteResponseRequest.StrictTools=true` to require native strict declarations
for every local tool; declare `strict_tools` in the endpoint/model profile.
Strict mode checks closed objects, required properties and the adapter's explicit
keyword subset before dispatch, without lowering even if lowering was authorized.
Hosted tools are unaffected. Completed arguments
must pass original-schema validation before successful tool_calls outcome. No
handler executes. Native call_id is the canonical tool ID, not the output item ID.
Original function items remain required scoped opaque state. Handles persist
ToolSchemaDigests/ForcedTool; Get/Cancel require original Tools in operation
options and reject changed/missing schemas before I/O, without generation tools
support. ExampleNewResponses_localFunctions runs locally. Completed function items
can be replayed with their original scoped state; RoleTool results correlate by
call_id against earlier history or Previous.ToolCalls. Native text arrays retain
part boundaries; duplicate/unknown/premature results fail before I/O. This does
not execute handlers or inherit current declarations. ExampleNewResponses_toolResults
is runnable offline with text and image results. Result arrays support approved
image URL/bytes and document URL/inline PDF, preserving order without fetch/upload.
InputMIME gates also apply inside results. IsError=true has no independent native
flag and is rejected rather than dropped or wrapped. Audio/video/cache/state
decorations remain unsupported; local SDK fixtures cover implemented shapes.

Hosted stateless assistant history now retains a scoped native output-order manifest.
Keep Content, Annotations, MessageAnnotations and required ProviderState together
via AssistantMessage and response wire. Original public message/search items replay
with citations, result presence and exact JSON; private reasoning/function items
are referenced, never duplicated in public metadata. Edited/missing/foreign history
and duplicate native IDs fail before I/O. Current tools remain explicit, even empty.
ExampleNewResponses_hostedHistory is runnable offline; no corpus CRUD or tool handler
is introduced. Historical metadata without the manifest is not silently bridged.

Hosted history records and the manifest use required scoped MessageAnnotations.
Persist their markers with the payload; unmarked legacy hosted history is rejected.
History reuse requires confirmed provider_state support as well as hosted feature
knowledge, while it does not request new encrypted reasoning automatically.
StateOmit removes this continuation metadata and marks the checkpoint unavailable.

`NewResponses(accountBinding, opts...)` implements caller-driven Create/Get/Cancel
through the Responses SDK, not Chat Completions or an Invoker fallback. Declare
`openai.responses` endpoint/model profiles with separate create/retrieve/cancel,
continuation and background capabilities. RemoteResponseRequest reuses canonical
execution and requires an explicit Store pointer. Previous selects a scoped
completed handle; new instructions are sent explicitly, including an empty value
when absent, rather than locally inheriting old settings.

EncodeRemoteResponseHandle/DecodeRemoteResponseHandle persist only binding,
identity, observed status, storage/background selection, selected hosted kinds and
original schema digest. Get requests exposed sources/results for those kinds;
new Create never inherits them.
Host owns account credentials, access, persistence and when to Get/Cancel. Cancel
can race completion and does not roll back effects. Unknown acceptance preserves
available identity for reconciliation, never blindly resubmits. SDK retries are
disabled. The operation-local body observer limits responses to 16 MiB before SDK
decoding and closes the original body; truncated/unavailable source is not complete
semantic capture. Auth/redirect policy remains on the injected client.

Structured output reuses schema fidelity and original validation. Get/Cancel of a
structured handle requires the same original OutputSchema; missing/mismatching
bindings fail before I/O. Retrieval does not remap/re-authorize request lowering.
Usage presence distinguishes zero from absence and never adds cached/reasoning
subsets to total. Semantic capture uses five common stages, metadata-only default
redaction, safe errors and retained accepted results on post-dispatch sink failure.
HTTP byte fidelity/physical attempts are unsupported, not inferred from JSON.

PromptExecution.HostedTools accepts HostedWebSearch/HostedFileSearch with separate profile
capabilities. Settings are checked JSON, not request overrides: web search supports
context size, approximate location, allowed domains and external_web_access;
file search requires host-owned vector_store_ids and supports result limits,
comparison/compound metadata filters and ranking options. Numeric filter values
are sent without float rounding. Unknown/reserved/duplicate settings fail before
I/O. No corpus CRUD, vector-store deployment or local handlers are introduced.

Migration: hosted declarations now belong to the canonical execution; the former
RemoteResponseRequest.HostedTools field is removed. Clone, execution wire and batch
identity preserve declaration settings independently of local ToolScope.

HostedSearchItems(response) projects ordered provider items from MessageAnnotations
with original output indices/JSON, typed sources/results and honest coverage:
unknown (missing), unavailable (null), available (completed array, even empty),
partial (array on unfinished/unsuccessful call). Available is not exhaustive search
or proof of tool success. These records survive response wire/history and are not
ToolCallParts. Citations remain bound to canonical text parts. Tools/instructions
are resent explicitly on continuation, including empty tools when none selected.

Input text parts remain ordered native input_text items, not one concatenated
string. User MediaPart accepts profile-approved PNG/JPEG/WebP/GIF via HTTP(S) URL
or raw bytes, and document URLs or inline PDF bytes via input_file. Inline PDF uses
input.pdf and a MIME-bound base64 data URL. Exactly one URL/data source is required;
assistant/instruction media, contradictory kinds, cache markers and unimplemented
types reject before I/O. No local fetch/upload or file store is introduced; host
owns egress/access policy and the profile's InputMIME knowledge. URL userinfo and
fragments are rejected without echoing sensitive values. Declared MIME knowledge
does not make an unimplemented mapping available. Request support does not promise
output media, audio/video or realtime. ExampleNewResponses_mediaInput runs locally.
Protocol references: [vision](https://developers.openai.com/api/docs/guides/images-vision)
and [file inputs](https://developers.openai.com/api/docs/guides/file-inputs).

Current implementation checkpoint supports ordered text/media inputs, text output,
native output schemas, hosted search and explicit remote lifecycle/continuation.
Reasoning output now preserves one ordered summary part and a required opaque
original-JSON envelope for each encrypted item. Keep content and ProviderState
together when restoring assistant history; stateless continuation checks their
binding and sends original native items, not a summary reconstruction. Missing
encrypted state marks ContinuationUnavailable. Explicit CapabilityProviderState
on Create requests encrypted content and fails with the known response if state
is unavailable. StateOmit projections exclude encrypted/private payloads and
cannot restore continuation. ExampleNewResponses_reasoningState runs offline.
No model-wide reasoning support or storage guarantee is inferred.
ProviderStateRequired is an explicit portable handle boolean: Get checks its
state capability and requests encrypted content, alongside hosted-tool includes.
Missing state remains an error with known output. Cancel has no include parameter
and retains its observed outcome and missing-state error without auto-fetching.
A new Create with Previous uses only the new execution's state requirement.
Get/Cancel original-schema validation does not require current native generation
schema support. Old wire without provider_state_required is rejected, not defaulted.
Local/hosted history uses checked source-order manifests and scoped provider state.
The runnable examples use local actual SDK fixtures; live account/model support
is not verified.

`Responses.CreateStream(ctx, request)` implements the optional
`prompty.RemoteResponseStreamer` port for foreground native SSE. Its lazy,
single-use handle delegates Events/Close/Status to the common Stream and exposes
RemoteResult for an owned remote handle plus canonical partial/final response.
The request is prepared and snapshotted without IO; consumption dispatches one
SDK streaming request with retries disabled. Require confirmed `response_create`
and `native_stream`, together with capabilities/settings of the actual request.
Background streaming is rejected before IO; Get/Cancel remain explicit operations.

Text, reasoning summaries, local function arguments and declared web/file-search
records use stable logical part IDs. Complete native items finalize existing
fragments without publishing completed content twice. Available encrypted state
and citations survive late errors. Terminal output uses the same original schema,
tool selection and outcome validation as Create. Native usage comes from present
Response.Usage fields; known zero differs from absence. EOF before completion,
contradictory identities/content and unsupported events fail with the retained
prefix. Close cancels local transport and releases its body; it does not claim
remote cancellation. Persist/reconcile a known remote handle explicitly.

SSE capture records decoded semantic events and a separate stream end summary;
it does not preread the body or claim HTTP-byte/physical-attempt coverage. Capture
failure after observing an accepted resource retains its identity. Run
`go test -run ExampleResponses_CreateStream` for an offline mixed-output example
and a rejected background request. Task43 passed independent offline completeness/error acceptance; this does not certify live model/account support. See the provider's
[streaming guide](https://developers.openai.com/api/docs/guides/streaming-responses).

Store intent does not guarantee durability or no retention. Current provider
background policy can retain data temporarily even with store=false; host owns
that decision. See [background mode](https://developers.openai.com/api/docs/guides/background).

Checked `ForcedTool` needs `CapabilityForcedToolSelection` in the effective profile in addition to `CapabilityTools`; the adapter maps native named tool choice. This is separate from structured-output guarantees and does not promise an infallible provider response.

Maps prompty's `PromptExecution` to OpenAI Chat Completions and returns a full `prompty.Response` with ordered parts, usage presence, outcome and annotations.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/openai
```

## Configuration

- **API key:** set `OPENAI_API_KEY` or configure the SDK client with `option.WithAPIKey(key)` and inject its pointer through `WithClient`. Checked `adapter.NewClient` performs preflight, translation, SDK execution and parsing. Direct SDK primitives remain available without checked guarantees.
- **Default model:** `New()` uses `gpt-4o`. Override with `WithModel(shared.ChatModel(...))`. You can also set the model per execution via `exec.ModelOptions.Model`.
- **HTTP client:** configured on the OpenAI client you create, not on the adapter.

## Implemented mappings

These mappings are not provider-wide/model-wide capability claims. Bind caller-owned `WithProfiles(endpoint, profiles...)` to the actual injected transport and effective model. Required unknown/unsupported features fail before dispatch. See the [migration guide](../../docs/migration.md).

`WithClient` alone no longer implies the default endpoint. Checked execution requires `WithProfiles(actualEndpoint)` even without capability profiles; configure the SDK URL from the same caller-owned value. Missing/blank binding fails before translation or dispatch. An execution override cannot replace this binding, and the adapter does not inspect SDK-private configuration.

- **Types:** `Translate` returns `*openai.ChatCompletionNewParams`; `ParseResponse(raw)` expects `*openai.ChatCompletion`. Native source is `StreamerAdapter.StreamEvents`; checked streaming returns `client.ExecuteStream(ctx, exec, prompty.StreamNative)`. Close the lazy single-use handle and inspect its final/partial Status. SDK retries are disabled; trailing usage is captured before the sole lifecycle terminal.
- **Messages:** system, user, assistant; text and tool calls. `MediaPart` is routed by MIME type: `image/*` (URL/base64), `audio/*` (inline input audio), other MIME types as inline file blocks.
- **Tools:** native tool definitions/calls and text-only result arrays. Chat rejects MediaPart results and IsError=true explicitly; separate Responses supports the bounded image/PDF result shapes described above. Core multimodal Content does not imply every endpoint can encode it.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Output/state:** strict/lowering schema diagnostics and original validation, refusal and full citation annotations. MaxTokens maps to max_completion_tokens; reasoning effort requires an effective profile options schema and never silently removes temperature.
- **Capture:** optional sync/native SDK JSON hooks plus canonical stages. Required HTTP bytes/physical attempts fail preflight because those boundaries are not instrumented. SDK response ID is not HTTP request ID.
- **Cache control:** checked execution reports block markers as unsupported and implicit-prefix knowledge from the effective caller-owned profile; it never fabricates a hit or request field. Raw Translate rejects required unmapped intent. See the task37 migration guide for the replacement of `CachePolicy.Type`.
- **Helpers:** With NewClient+Execute use `resp.StrictText()`. With direct Translate/Execute/ParseResponse use `prompty.StrictTextFromParts(resp.Content)`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/openai) for the full API.

The [unified capability matrix](../../docs/provider-capability-matrix.md) records pinned SDKs, operation/backend limits, checked/raw guarantees and linked offline evidence. The [clear-break migration](../../docs/migration.md) covers accepted tasks38–43; historical task37 evidence is not rewritten as current certification.
