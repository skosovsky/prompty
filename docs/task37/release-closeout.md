# Task37 release and issue closeout

Status: preparation only. No release, tag, GitHub comment or issue closure is authorized by this document. The owner reply selects all five directions now. Expanded implementation is not complete; historical audits certify only their named snapshots/old scope.

Source: https://github.com/skosovsky/prompty/issues/4 and `.cursor/docs/task37.md`. The full source specification takes precedence over the extracted [requirements matrix](requirements.md). [Contracts](contracts.md) and [migration](migration.md) describe implemented interfaces, not an approval to publish.

Author migration must also cover native local-tool selection: ForcedTool requires the separate `CapabilityForcedToolSelection` profile and actual mapping port. Gemini no longer injects a forcing system instruction; Ollama rejects this unavailable mapping. Ordinary tools and the required explicit Anthropic synthetic-output profile remain distinct; no legacy fallback. See [tool selection evidence](tool-selection-conformance.md).

Implemented synthetic checkpoint: callers select `NewSyntheticOutput`, set
`Requirements.OutputMode = OutputSyntheticTool`, name `ResponseFormat`, and bind
synthetic-output/tools/forced-selection profile knowledge. Checked sync/buffered
execution validates received arguments against the original schema and never
returns an executable synthetic ToolCallPart. Do not register an output handler
or mix real tools in this base profile. Native selection is separate; raw parse
does not establish schema acceptance. Structural, schema and outcome failures
retain observed partial/usage and do not authorize blind retries. Broader
conformance/reasoning/capture and paired final audits remain required before
this becomes a release/issue-closeout claim.

Cache migration must distinguish nil/inherit from explicit `CacheOmit`/no block marker. Checked omission no longer substitutes an ancestor's TTL for rejected or implicit-only part intent. This portable policy has no provider/mode/controls and does not disable implicit caching or delete resources; source intent/disposition remain visible. See [cache conformance](cache-conformance.md).

## Acceptance before release

IA16 migration: do not invent max_tokens or strip ModelOptionsSchema to read/cancel
an accepted response, inspect/cancel/read batch results, or use cache lifecycle.
Operation-only preflight retains the complete profile snapshot and independent
capability/binding checks. Cache Create rejects generation settings while checking
content/tools/state/MIME and schemas; generation Create/Submit still validate the
generation contract. Custom operation adapters may use PreflightOperation or
PreflightInputOperation, but must retain their actual IO/handle/result validation.
Shared/SDK regressions and all-module gates pass; final independent audits remain
required before making a release/author-message/issue-closeout claim.

Responses author migration must mention the new explicit handle boolean
ProviderStateRequired/provider_state_required. Old wire missing it is rejected,
not restored with a fabricated default. Required state survives Create/handle
wire/Get and composes encrypted/hosted includes; a fresh Create with Previous
sets requirements anew. Keep reasoning Content and required ProviderState
together for stateless history, never substitute summary text for encrypted
items. Cancel has no native include; missing state returns an error alongside
known cancellation/completion and may require explicit later Get, not auto-retry.
Get/Cancel original-schema validation must not require current generation schema
support. These are prepared migration instructions, not authorization to message
authors or close the issue before full acceptance.

Local Responses functions require original Tools rebinding in Get/Cancel; handles
retain ToolSchemaDigests/ForcedTool, never handlers. Native call_id differs from
the provider item ID retained in opaque state. Strict=false preserves caller
schema semantics with local received-result validation, not native constrained
decoding. Check errors/outcomes before execution. Completed function history now
replays original scoped state; text results require exact call_id from history or
Previous.ToolCalls. The handle contains call names, not arguments/handlers. Preserve
native text/media array boundaries and InputMIME profile gates. Images use URL/bytes,
documents URL/inline PDF without local fetch/upload. IsError=true, audio/video/cache/
state decorations fail explicitly; hosts define their own text/JSON error payload,
not an adapter-invented wrapper. The updated mixed-result example is runnable;
remaining expanded conformance remain required work before full closeout. Current instructions and
tools remain explicit; no handler or agent loop executes automatically.

Hosted stateless migration requires AssistantMessage's full Content/Annotations/
MessageAnnotations/required ProviderState, including the scoped native-order
manifest. Public source items preserve citations, coverage and extensions; private
state is referenced, not copied into annotations. Edited, foreign, unfinished,
duplicate or source-less history fails before dispatch. The runnable hosted-history
example uses explicit empty current tools and no corpus/resource management.

- Reconcile every source requirement and acceptance criterion with a stable matrix ID. Preserve all seven PRM areas and AUD-01–05. A partial row does not earn completeness credit.
- Implement all five directions selected in the [owner reply](https://github.com/skosovsky/prompty/issues/4#issuecomment-6008749968) and [scope](scope.md). No intermediate MVP closure or deferral. Optional means consumer opt-in; application orchestration/storage/IAM stay excluded. Explicit bounded caller-invoked Wait is allowed, not a daemon.
- Finish implementation, documentation, compiled examples and permanent AAA regressions. Keep routing, agent loops, retry orchestration, scheduling, IAM and storage backends outside core; profiles/transport/types/policy are caller-owned.
- Run the two independent auditors on the same final worktree snapshot. Completeness must compare the original specification, matrix and actual code; report `100 × fully completed / applicable requirements` with row-level evidence and conditional dispositions. Error audit must include severity, exact file/line, trigger, consequences and reproduction for every confirmed finding, including AUD-01–05.
- Fix findings and rerun both auditors after changes. Earlier verdicts are not final evidence for modified code. Require 100% applicable completeness and no open confirmed findings, not a claim that software can never contain a bug.
- Run `make lint` and `make test` across all modules, including race checks and generator fixtures, after the final changes. Record actual exit status, snapshot identity and limitations. Local SDK/HTTP/SSE fixtures are not live account/model conformance evidence.
- Verify migration examples against the final exported signatures and wire schemas. Unsupported historical formats fail explicitly; no legacy bridge is introduced.

## Required expanded scope and library boundaries

All five directions are selected now, not roadmap. See task37 and the extension matrix for granular acceptance.

| Area | Required implementation | Current gap |
| --- | --- | --- |
| Native | Anthropic/Gemini/Ollama native transports, per-provider fault fixtures/examples | All four initial transports/fixtures/examples, unsigned Gemini partialArgs wiring/Vertex opt-in and ordered message metadata exist. Signed incremental original parts/global order and file-backed batch have later implementation evidence; exact Ollama ingress and active interruption/parallel/state fixtures exist. Final paired acceptance remains required. |
| Responses | Separate OpenAI adapter: continuation, web_search/file_search, background lifecycle/handles | NewResponses has actual SDK Create/Get/Cancel, scoped wire handle, explicit storage/continuation, original schema binding, capture and local fixtures/examples. Hosted web/file search now maps checked settings/includes and preserves typed items/citations/coverage through wire/history. Ordered text/media input, exact encrypted reasoning state/stateless replay and required-state Get includes are implemented with fixtures/examples. Hosted/local function history/results and stateless hosted order/citations are implemented with SDK fixtures/examples; final paired acceptance remains required. |
| Batch | OpenAI/Anthropic/Gemini working adapters, IDs/outcomes/completeness, injected file ports | Core scoped wire/lineage/input digest/completeness, actual Anthropic lifecycle, OpenAI Chat Completions file-backed Batch and Gemini Developer inline BatchManager exist with SDK fixtures/examples. Gemini file-backed transport is implemented with injected host ports and SDK/source fixtures; schema/state/capture/fault and active download/cancel fixtures exist. Final paired acceptance remains mandatory. No live profile acceptance or publication is claimed. |
| Cache | Gemini CRUD/use scoped cache handle; supported request controls distinct | SDK lifecycle/reuse/scoped wire/capture/redaction exist, including IA11 delayed-expiry guards. OpenAI whole-request key/retention, Anthropic block marker/TTL and explicit unsupported Gemini controls have actual SDK/profile/usage evidence; multi-question immutable resource reuse and lifecycle fault fixtures exist. Final paired acceptance remains required. |
| Synthetic output | Explicit Anthropic profile, one dedicated validated output call, no real tools in base profile | NewSyntheticOutput implements separate explicit sync/buffered extraction and original-schema validation with SDK fixtures/example; five-stage capture failure/redaction and checked reasoning/tool restrictions have fixtures. Final paired acceptance remains required. |

Reference consumers suffice without production credentials. Every selected provider needs real implementation plus offline protocol/fault fixtures and runnable examples. No credentials means live integration unverified, not an adapter stub. Reuse existing capture/redaction/schema/state/usage guarantees. Cancellation is not rollback. Hosts retain auth/egress/persistence/cleanup policy; no scheduler, application store, dataset runner, corpus CRUD, realtime sessions, automatic retries/recache or silent fallback.

## Release command and authority

Use `make release-break` for the complete task: execution/result/stream/schema/cache contracts change. `make release-patch` is only suitable for an independently scoped insignificant fix that does not change the public contract; it cannot publish or close the full task37.

Obtain explicit publication permission after acceptance. Do not run either release command as a diagnostic. The current release target runs gates and invokes `scripts/release.sh`, which fetches tags, requires a clean tracked checkout, detaches HEAD, rewrites module dependencies, stages files, may create a release commit, creates root/submodule tags, pushes all local tags and returns to the original branch. Inspect the script and all proposed tags/module changes at release time; its confirmation prompt is not a substitute for the user's authorization.

Before authorized execution, use a reviewed clean release snapshot. The present dirty worktree contains implementation and potentially user-owned changes; do not stash/reset/delete or indiscriminately commit them to satisfy the script. Resolve untracked/ignored deliverables and verify that everything referenced publicly is actually included in the intended published snapshot. Inspect unrelated/unpushed tags because the script uses `git push origin --tags`. If the target would publish anything outside approved scope, stop and request direction instead of widening permission.

After a successful authorized release, verify the remote root/submodule tags and inspect the published source/docs. Replace the placeholders below with actual evidence. A local green test or prepared draft is not evidence that publication happened.

## Author-facing comment draft — do not post yet

Publish only after final acceptance and explicit authorization to comment/close. Fill all bracketed fields and remove this instruction. Until then this is a migration checklist, not an announcement of completion.

> Implemented the accepted scope of issue #4 with a clear API break, without compatibility bridges.
>
> Implementation: [actual published implementation link]. Migration: [actual published migration link]. Completeness: [completed/applicable] = [percentage], with [final independent audit evidence]. Error audit: [final report link], no open confirmed findings. Final checks: [snapshot and actual make lint / make test results].
>
> Update caller code as follows:
>
> Gemini native migration: remove deleted ParseStreamChunk calls. Use checked ExecuteStream with supported endpoint/model native knowledge. For unsigned incremental calls add gemini.CapabilityIncrementalToolArguments to execution requirements and supported profile knowledge alongside tools/native; this sends the flag via Vertex, not the unavailable MLDev mapper. Inspect gemini.partial_function_call part annotations for exact provisional path/value events; empty Args is not handler-ready and ArgsChunk is not used for this protocol. Complete calls use Args snapshots after closure. Read candidate citations/grounding from MessageAnnotations, not content part zero; preserve ordered metadata updates and provider-relative indices, including late metadata-only events. Preserve the handle/state, close on stop and inspect partial status on error; WithResponsePayloadLimit bounds each SSE line including framing. Signed incremental continuation requires the original ordered partial-parts codec, source ordinals and exact signature positions; incomplete/canceled/edited state fails. No silent fallback/replay.
>
> 1. Replace provider-name capability assumptions with caller-owned `CapabilityProfile` bound to the actual SDK endpoint/effective model through `WithProfiles`. Supply an explicit endpoint even with zero profiles: `WithClient` cannot reveal or imply SDK destination, and missing binding fails with `adapter.ErrNoRuntimeDescriptor` before dispatch. Configure SDK URL and binding from the same caller-owned value. Handle `adapter.ErrUnknownProfile` and `ErrUnsupportedCapability` before dispatch. Overrides supply knowledge, not missing implementation or transport identity.
> 2. Replace the iterator/IsFinished API with `ExecuteStream(ctx, execution, prompty.StreamNative)` or explicitly `StreamBuffered`. Consume `Events()`, call `Close()` on early stop and inspect `Status()` for partial content/usage/errors and `Identity`. Custom sources must supply PartIDs aligned with Content; terminal/status references align with Result.Content. Keep logical references separate from fragment offsets/provider IDs, and attach middleware to the original handle through ObserveLifecycle. Update capture observers to the StreamStatus signature; native/buffered canonical records now include stream_identity. A finish marker is not terminal; no native-to-sync fallback remains.
> 3. Use `AssembleToolCallParts` for correlation-aware tools. Incremental arguments use `ArgsChunk`; `Args` is a complete object snapshot. Supply execution-scoped `Tools`/`ForcedTool`; handler registration is not permission. The whole batch is checked before side effects. ID-less complete calls require explicit `CapabilityToolNameCorrelation`, supported checked profile knowledge and the adapter's name-preserving port; every such name must be unique across the batch. Do not fabricate provider IDs or omit checked diagnostics in custom invokers.
> 4. Read `Response.Outcome` and presence-aware `Usage.Known`/`Availability()`. Unknown counters are not zero, and cache/reasoning breakdowns must not be added to total again. Retain known responses returned with post-dispatch errors; do not blindly retry.
> 5. Persist through `MarshalResponse`/`DecodeResponseJSON` and explicit `WirePolicy{State: StatePreserve}`; build history through `AssistantMessage`. Rebind runtime ports after restore. Logging omits required opaque state and cannot become a continuation checkpoint. Do not edit signed content or repair incompatible saved state silently.
> 6. Use strict schema fidelity by default. Explicit `SchemaLowering` requires named `AllowedTransforms` and final original-schema validation. Set `SchemaDefinition.Dialect`/`ToolDefinition.Dialect` explicitly for `SchemaDialect7`; empty selects `SchemaDialect2020`, and conflicting declarations fail before dispatch. Persisted execution wire retains each selection; rebind custom/resource-aware validators after restore. Domain `Validate()`/Go decoding are separate checks; streamed items remain provisional until terminal/document validation. Native Anthropic output is ordinary text from `OutputConfig.Format`, not a synthetic output tool; a real `output_format` tool stays a tool call.
> 7. Keep reasoning controls in `ModelOptions.ProviderSettings` and define the effective profile's `ModelOptionsSchema`. Treat mode, budget, visibility and effort independently. OpenAI MaxTokens maps to max_completion_tokens and effort no longer silently removes temperature. Custom numeric settings readers must handle `json.Number`.
> 8. Replace `CachePolicy.Type` with `Mechanism`, advisory/required `Mode`, provider scope and `Controls`. Inspect `Diagnostics.Cache`; accepted controls are not a hit. Marker TTL is checked against the profile and actual mapper. Resource lifecycle is separate.
> 9. Bind an optional `CaptureBinding` and explicit `Requirements.Capture`. The host projector controls payload/metadata privacy. Inspect `Diagnostics.Capture`; SDK JSON is not wire bytes or physical-attempt evidence. HTTP status/request ID remain absent unless observed. Post-dispatch capture errors do not mean not-executed. Persisted reports must satisfy `CaptureReport.Validate()`.
> 10. Inject `mediafetch.DefaultFetcher{Client: client, MaxBodySize: limit}` instead of mutating global clients. HTTPS is checked on every redirect without weakening caller egress/redirect restrictions.
> 11. For exact Ollama numeric tools, inject NewHTTPChatClient(endpointURL, hostHTTPClient, positiveFrameByteLimit) through WithClient. Host transport supplies auth/redirect/egress/timeout policy; SDK automatic auth is not inherited. Unverified SDK float args now fail instead of being used as original values. Dynamic DecodeToolArgs map/interface numbers are json.Number; update numeric assertions/conversions. Explicit numeric DTO fields remain your selected Go conversion semantics.
>
> JSON error handling: handle `ErrSchemaJSONDecode` separately from `ErrSchemaMismatch`. Invalid/truncated/multiple documents are no longer ordinary repairable `ValidationError`, and the structured executor does not append domain feedback for them. Incremental syntax failures retain `ErrStructuredStream`; refusal/incomplete remain provider outcome errors. Keep known checked responses returned alongside post-dispatch errors and do not infer that the request was never executed.
>
> Gemini caller changes: construct the SDK first, then bind `WithClient(sdk)` and the same explicit `WithProfiles(endpoint, profiles...)`. Original signed response observation is per invocation and bounded; set `WithResponsePayloadLimit(bytes)` when the default 4 MiB is unsuitable, using a positive limit. Rebind after public client/configuration changes. Signed raw float DTOs without original `SDKHTTPResponse.Body` cannot prove fidelity; do not convert already rounded floats into `json.Number` and call them original state. Preserve exact response/execution wire, and handle `ErrProviderState` before attempting continuation when the outgoing SDK cannot represent signed values. Malformed/unconvertible completely received HTTP 2xx bodies now return a safe state error instead of body-bearing SDK errors or conversion panic. Cancellation/deadline remains identifiable; a read/transport failure does not become a successful signed response or trigger automatic replay.
>
> Compiler diagnostic changes: inspect `*SchemaCompilationError.Locations` as source document/resource RFC 6901 positions, not provider destinations or failing target URIs. Missing resource/fragment/anchor diagnostics are engine-verified reachable origins; unknown position remains distinct from known root. Original engine/decoder causes and caller resource/pointer values still require host logging policy. Supplying explicit resources permits local validation only, not network fetch, upload or provider support.
>
> Stream closeout changes: `Close` on a running handle requests cancellation without waiting; consume/unwind before treating `Status` as immutable terminal evidence. Consumer break cancels the producer and performs one capture closeout without another yield or whole-document finalization. An acknowledged closed/complete capture report can describe a canceled execution; it is not execution success. Preserve cancellation together with any frame/capture error, and do not detach caller cancellation to force recording.
>
> Expanded caller migration: construct `openai.NewResponses(accountBinding, opts...)`, call `Create(ctx, prompty.RemoteResponseRequest{...})`, and persist `EncodeRemoteResponseHandle`/restore `DecodeRemoteResponseHandle`. Set Store explicitly, resend instructions/tools, and supply HostedTools with host vector-store references. Use `Get`/`Cancel` explicitly; operations rebind original schema/tools when required. Construct `openai.NewBatches(accountBinding, files, opts...)`, `anthropic.NewMessageBatches(accountBinding, opts...)`, `gemini.NewBatches(accountBinding, opts...)` or `gemini.NewFileBatches(accountBinding, files, gemini.BatchFileOptions{...}, opts...)`; use Submit/Get/Cancel/Results and unique BatchItem IDs/attempt lineage. Inspect per-item outcomes and retrieval completeness; retry is host policy, as in ExampleNewMessageBatches_failedOnlyRetry, never blind unknown-submit replay. Construct `gemini.NewCacheResources(accountBinding, opts...)`, call Create/Get/UpdateExpiry/Delete, assign the returned handle to execution.CacheResource, and bind generation WithCacheAccountBinding. Retain immutable content/model/account identity and expiry; RequestCache or block controls do not delete resources. Choose `anthropic.NewSyntheticOutput(opts...)` and the explicit synthetic output mode only for its weaker received-result guarantee and checked tool/reasoning restrictions; no silent fallback or real tools in the base profile. Application scheduling/storage/IAM/dataset/corpus workflows remain outside prompty. Local SDK fixtures do not verify live credentials/account/model support; live checks require separate explicit authorization. Unsupported old formats require explicit migration, no legacy decoding.

## Evidence checkpoint — not final approval

Historical paired checkpoint9 confirmed its production/tests and migration evidence, before the owner-selected expansion. Complete make lint/test after last test-only addition both terminal EXIT0 across all modules/race/generator fixtures: `/private/tmp/prompty-task37-unique-baseline-final-lint.log`, `/private/tmp/prompty-task37-unique-baseline-final-tests.log`. git diff --check clean. Author instructions above are prepared draft, not published delivery.

[Checkpoint9](audit-checkpoint9.md) is a historical paired verification: 143/160 required whole rows (89.38%). IA07/08/09 were independently corrected in that snapshot. The later full-scope frozen audit recorded 203/223 (91.03%) and IA19; subsequent implementation corrected IA19/20 and added cache/lifecycle/outcome evidence. See progress.md and operation-conformance.md for current checkpoints. Neither historical percentage is current expanded-scope acceptance; a final paired audit of the new snapshot remains required. No publication authorization exists.

## Closing the issue

Close https://github.com/skosovsky/prompty/issues/4 only after accepted complete implementation covers every selected source item, the author-facing migration message has been published with actual evidence, and closure is separately authorized. Preparing this document, passing checkpoint gates or creating continuation tasks does not close the issue by itself.
