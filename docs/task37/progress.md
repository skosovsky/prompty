# Task37 implementation checkpoint

This is implementation evidence, not final acceptance. The original task and its full scope remain authoritative.

## Active operations and batch outcome conformance checkpoint

Previous turn made implementation progress: extended cache SDK/mode fixtures,
immutable Gemini two-question reuse, explicit failed-only batch retry example,
and reconciled current migration/closeout evidence. It was not final acceptance.

New deterministic actual SDK interruption fixtures cover Responses Create/Get/
Cancel, OpenAI Batch Cancel/file Results, and Anthropic Message Batches Cancel/
Results. They prove an accepted HTTP body was actively blocked before cancellation
or deadline, not merely canceled before preflight. Each operation dispatches once
and closes the original body once. Unknown mutations retain known IDs without
inventing status from incomplete JSON. Responses known IDs are explicitly reconciled
with a new host Get; adapters neither replay nor poll automatically.
Accepted first JSONL records remain with original SourceJSON; two remaining IDs
are missing, both completeness dimensions false. OpenAI stops before a second
file; ended remote status/resources remain independent of failed retrieval.
Gemini host file interruption likewise retains prefix/unknown usage and remote
status, with one download/close and no upload/job/generation operation.

Native truncation fixtures now prove Anthropic max_tokens, OpenAI length and
Gemini MAX_TOKENS in inline/file modes directly, not through schema rejection.
Absent per-item usage remains unknown. Anthropic native canceled and existing
expired variants are explicit; OpenAI expiration is explicit and unknown item
cancellation mappings are not promised. Gemini RPC cancellation source remains a
provider error rather than invented per-item lifecycle enum; operation-conformance.md
records the explicit unsupported normalized item-canceled/expired guarantee.
That inventory pins installed SDKs, observed API fixture paths and caller profiles,
links fault/example evidence, and excludes live account/model claims.

All-module make lint/test completed after the last Go edit with observed exit 0:
/private/tmp/prompty-batch-response-conformance-lint.log and
/private/tmp/prompty-batch-response-conformance-test.log. git diff --check passed.
No production change was required by these new interruption/outcome fixtures.
Matrix reconciliation, full-document consistency and both repeated independent
audits remain required; this checkpoint does not assert current 100% completeness.

## Request-level cache controls and IA20

The latest owner reply was fetched again: no newer scope decision exists, and all
five directions remain required. The preceding reply-only turn confirmed existing
specification reconciliation but did not advance implementation acceptance.

PromptExecution.RequestCache now separates whole-request implicit-prefix intent
from block markers and managed resources. Portable validation, wire schema and
clone paths preserve provider-scoped control JSON. The optional RequestCacheMapper
and profile capability/control gates select or reject settings; advisory removal
is reported, required rejection is fail-closed, and cache combinations are checked.
Unsupported raw translators reject request intent instead of pretending to map it.
OpenAI Chat and Responses map distinct scoped key/retention controls to native SDK
fields; invalid/duplicate/unknown knobs cannot fabricate bypass or remote deletion.

Permanent core/shared tests cover wire/ownership/invalid mechanisms, supported vs
unknown vs missing mapper, required/advisory and forbidden combinations. Actual
local SDK HTTP fixtures cover both OpenAI APIs, native request fields, zero-I/O
required failure and advisory omission without source mutation. No live credentials
or model support is inferred. Native/buffered cache conformance, runnable example,
exact migration, remaining full-scope rows and final paired audits remain required.

The new ownership regression also found IA20: Normalize's manual execution literal
lost the existing CacheResource binding and new RequestCache. It now delegates to
the common cloning path. Dedicated AAA tests retain independent remote-resource
identity and request control bytes. Current matrix has 225 required stable rows;
historical 203/223 audit percentage remains historical, not a current result.

After the last Go change, full all-module make lint and make test both completed
with observed terminal exit 0. Logs: /private/tmp/prompty-request-cache-lint-verified.log
and /private/tmp/prompty-request-cache-test-verified.log. git diff --check passed.
These gates support this checkpoint, not full-scope acceptance or a current
independent completeness percentage. Native/buffered request-cache fixtures and
example remain pending before the affected extension rows receive full credit.

## Expanded cache and explicit failed-only batch retry checkpoint

Subsequent OpenAI SDK fixtures cover Chat native/buffered key/retention plus late
cached usage (total remains 12, cached 3), and unknown destination-key capability
rejects before a third HTTP request for both Chat and Responses. The profile-unknown
fixture initially mutated only Chat's cloned descriptor; it now explicitly mutates
both independent descriptors. This was a fixture setup error, not evidence of a
production support bypass. ExampleAdapter_requestCache is runnable and inspects
accepted disposition separately from observed usage. Gemini's actual SDK fixture
now creates exactly once and answers two independent questions through the same
unchanged resource/digest/config without document replay or hidden resource creation;
ExampleNewCacheResources follows both questions with update/inspect/delete.

ExampleNewMessageBatches_failedOnlyRetry supplies host retry policy, not a library
retry engine: actual SDK first Submit/Get/Results, then only error/expired item
executions are submitted under new IDs/attempt with original/parent lineage.
The successful item is absent from the second HTTP payload. Unknown submit and
incomplete retrieval are not treated as retry permission. The first draft tried
Results before a terminal Get; the runnable version explicitly observes ended
before retrieval, preserving the existing lifecycle contract.

Operative migration/contracts/closeout no longer contradict delivered signed
incremental codecs, Responses local tools/history or the owner's selected scope.
Closeout uses verified current constructor/operation names instead of a future
signature placeholder. Further full-document reconciliation and fault cases remain.

After the last Go change, make lint and make test completed across all modules
with observed terminal exit 0: /private/tmp/prompty-cache-retry-lint-final.log and
/private/tmp/prompty-cache-retry-test-final.log. git diff --check passed. Matrix
EXT-C-03/06, S10-01/04, S13-05, EXT-B-08 and IA20 now have complete parent evidence,
not a new independent completeness percentage or final acceptance. Both final
auditors must recheck the eventual final snapshot and the remaining full scope.

## IA19 dispatch-time continuation correction

Shared PreparedRequest now retains an independent immutable state-bearing input
snapshot even when capture is disabled. ValidateDispatchState checks local scope/
expiry plus the actual adapter codec/canonical binding on an isolated copy, with
no network, generation preparation, state renewal or retry. Both sync/buffered
and native dispatch gates run after host capture and native StreamStart pauses.
The capture closeout retains pre-dispatch classification on a rejected lease.

Anthropic Message Batches checks all original items after capture. Gemini inline
and file-backed batches check before submit/upload and again after upload before
job creation. OpenAI file-backed batches use the same checks before upload and
job submission (no state transport is invented). Results-only validation remains
unchanged and does not require a generation lease to remain live.

The independent auditor repro now returns `calls=0` and ErrStateExpired for both
sync generation and Anthropic batch. Permanent actual Anthropic SDK regressions
cover sync/buffered/native, consumer pause without a sink, caller expiry mutation
after StreamStart, one terminal and no provider/job dispatch. Actual Gemini SDK
fixtures cover inline/before-upload/after-upload expiry: one accepted upload stays
in Resources, SubmissionAttempted remains false, no job handle or hidden retry.
Post-upload terminal is correctly after transport activity, not a claim that no
file was uploaded. Targeted race logs:
/private/tmp/prompty-state-dispatch-targeted.log and
/private/tmp/prompty-batch-state-dispatch-targeted.log. After the final Go edit,
all-module make lint/test completed with observed terminal exit 0 in
/private/tmp/prompty-state-dispatch-lint-verified.log and
/private/tmp/prompty-state-dispatch-test-verified.log. git diff --check passes.
This is parent correction, requiring both final independent audits; the previous
91.03% frozen-snapshot score is not updated without new full-scope verification.

## Independent full-scope audit checkpoint

Two read-only auditors inspected frozen tree
`f61a1f4720097dedf57b27ab395d87d989df528b8f439d039f6524312e723394`,
task hash `77b89ff25673760260bbab0644ce4b5512b725ec481da3f37cebf937fa9c9468`,
matrix hash `d812e3f30891cf451bf14b25da4da626efa39a8588800e36dddeb4c2b6781550`.
Their reports are in this chat under scope_completeness_audit and
scope_correctness_audit. No live calls or external writes occurred.

Completeness classified 203/223 = 91.03% fully implemented, 20 partial, none
entirely missing. The denominator includes 210 T37 rows plus 13 AUD rows, all
applicable. This is a historical frozen-snapshot score, not acceptance of later
changes. Existing pending/partial matrix labels were not used as implementation
proof; final row-level reconciliation is required. Independent make test passed:
/private/tmp/prompty-completeness-audit-test.log.

Remaining acceptance actions by stable IDs:

- EXT-C-06, S10-01/04, S13-05: actual OpenAI Chat/Responses request cache key and
  retention mapping, checked provider/profile intent and disposition, SDK fixtures
  and migration. Gemini automatic implicit cache requires no invented knobs.
- EXT-C-03: two independent questions on one immutable Gemini cache, one Create,
  unchanged digest/config, no content replay or hidden resource creation.
- EXT-B-08: runnable host-driven failed-only retry with new attempt and original/
  parent lineage; successful items must not be resubmitted.
- EXT-B-05: three-provider truncation/incomplete/unknown usage outcomes and
  expired/canceled or explicit unsupported-outcome inventory/fixtures.
- EXT-R-06, S07-08: active Responses cancel/deadline after dispatch, close, known
  identity/status or unknown acceptance, explicit reconciliation, no replay.
- EXT-X-01, S07-12, S12-05: batch unknown Cancel and active result interruption,
  retained prefix/remote status/body close; Gemini file-path evidence explicit.
- EXT-X-02, S05-20, S12-07: current API/SDK/profile conformance inventory; local
  fixtures are not live endpoint evidence.
- EXT-X-04, S12-01: reconcile all stable row evidence and link final paired
  verdicts/gates without dropping original scope or IDs.
- S12-03/08, S13-07: remove contradictory operative migration/closeout statements
  about unimplemented signed incremental/local Responses state/tools and unresolved
  triggers; replace exported-signature placeholders with current exact APIs.

Correctness found one confirmed remaining P1, now IA19: generation/batch state
can expire between preparation and actual dispatch. Actual Anthropic SDK/local
HTTP repro /private/tmp/prompty-audit-state-dispatch.go reports
`expired=true calls=1 err=<nil>` for sync generation and batch. Buffered/native
share the affected dispatch code; their permanent regressions are still needed.
AUD01–05 and IA17/18 corrections passed the inspected paths. Auditor race tests
passed across core and four providers; this does not negate the new expiry repro.
Gemini retained-source observation bound is not a SDK-consumption bound; the
auditor recorded that documented limitation, not a second confirmed regression.

No release/closure is authorized. Both audits must repeat after fixes on one
new frozen snapshot. Adding IA19 below increases the current ledger denominator;
do not silently reuse the historical 223 score for the changed ledger.

## Synthetic output capture conformance and IA18

`synthetic_capture_test.go` exercises required sink failure at all five semantic
stages in sync and buffered modes. Default projection excludes payload; SDK build,
sequence and invocation identity remain visible without physical-attempt claims.
Post-dispatch capture failure retains completed schema-validated output and known
usage without retry. A semantic opt-in fixture retains SDK-unknown fields/exact
large numbers and distinguishes canonical execution, private output-tool request,
provider tool-use response and canonical text result. No handler or native decoding
guarantee is introduced. SyntheticOutputAdapter now declares the actual SDK build
but no native/transport/attempt capture hooks.

These fixtures reproduced IA18/P2: sync pre-dispatch capture failures returned
without terminal capture. adapter/client.go now joins captureSyncTerminal in
pre-dispatch failure/cancellation paths; buffered paths defer closeout to their
existing handle. The initial reproduction is in
/private/tmp/prompty-synthetic-capture-targeted.log; corrected focused race tests
pass in /private/tmp/prompty-synthetic-capture-complete.log. Preflight conflict
fixtures also assert caller source remains unchanged. After the last Go edit,
all-module make lint/test completed with observed terminal exit 0 in
/private/tmp/prompty-synthetic-conformance-lint-verified.log and
/private/tmp/prompty-synthetic-conformance-test-verified.log. git diff --check passes.
Final independent audits and full selected scope remain required.

## IA17: cache-content input state expires during capture

Actual SDK/local HTTP regression TestCacheCreateRejectsStateExpiredDuringCapture
proved that a signed input envelope expired inside CaptureTranslatedRequest but
Create returned success. The parent-confirmed P1 repro is recorded in
/private/tmp/prompty-cache-state-expiry-repro.log. Resource absolute expiry was
already rechecked; provider input state was not.

CacheResources.Create now validates the original input state snapshot after all
capture callbacks and context/resource-expiry checks, immediately before dispatch.
Scope, expiry, codec/payload and canonical binding remain checked; no generation
options validation is introduced. Expiry returns ErrStateExpired with zero HTTP,
no invented handle/unknown acceptance and a closed pre-dispatch capture terminal.
No renewal, retry, recache or host scheduling is added. The focused race regression
passes in /private/tmp/prompty-cache-state-expiry-fixed.log. All-module make lint
and make test completed with observed terminal exit 0 in
/private/tmp/prompty-cache-state-expiry-lint-final.log and
/private/tmp/prompty-cache-state-expiry-test-final.log. git diff --check passes.
Full selected scope and both independent final audits remain required.

## Native request-local cancellation conformance

`internal/streamtest/isolation.go` overlaps two native requests on one client,
using equal provider call IDs/indices and different text/arguments. Actual local
SDK/SSE Anthropic and Gemini fixtures and bounded HTTP/NDJSON Ollama fixtures
prove early tool publication before completion, independent cancellation/body
release, retained partial output, distinct canonical message identities, exactly
one terminal per stream, late usage without double counting and no replay.
Cancellation without a provider finish leaves the provider outcome unknown;
it does not fabricate an incomplete result or materialize unfinished arguments.

Each adapter's `TestNativeParallelCancellationKeepsCorrelationRequestLocal`
also checks unknown-model and unknown-tools intent with zero HTTP calls. Transport
availability cannot supply missing model capability knowledge. These checks do
not cover all schema/state/capture combinations or establish live model support.
The initial harness lint findings were corrected. Both all-module `make lint`
and `make test` completed with observed terminal exit 0, including race/example/
generator coverage. Logs: /private/tmp/prompty-native-isolation-lint-final.log and
/private/tmp/prompty-native-isolation-test-final.log. `git diff --check` passes.
Full scope and final paired audits remain required; NS-04/05 are partial, not complete.

## Operation-only preflight and IA16

Parent-confirmed SDK repro: a valid generation options schema requiring max_tokens
blocked Responses Get/Cancel before I/O. Shared generation Preflight was also used
by batch inspect/cancel/results and Gemini cache get/update/delete. These operations
do not create a generation request or supply model options.

adapter.PreflightOperation now reuses resolveProfile and the complete profile
snapshot, deduplicates operation requirements and checks capability combinations,
without applying generation options validation. Responses, all three native batch
adapters, shared result-only schema preparation and cache handle operations use it.
Generation Create/Submit retain full generation preflight; result schema,
tool selection, handle/destination/account bindings and capture remain independently
checked. No dummy generation settings, stripped profile, hidden resubmit or legacy
bridge is added.

operation_preflight_test.go covers snapshot equality with the full generation
profile, schema-dependent snapshot changes, clone isolation, overrides, unknown/
unsupported capabilities, malformed/ambiguous destinations and forbidden
combinations. Actual SDK Responses and three-provider batch/cache lifecycle fixtures
set a strict generation-only schema after acceptance and still read/cancel/results;
Responses new Create remains rejected before I/O. Repro evidence:
/private/tmp/prompty-operation-options-repro.log. Final paired audits remain required.

A second deterministic SDK repro confirmed the same conflict at cache-content
Create: generation requires max_tokens while cache validation forbids it. Resource
input now uses PreflightInputOperation, retaining inferred tools/state/MIME and
capability combinations while rejecting generation settings/output intent. Cache
schema compilation/mapping and exact immutable config digest remain unchanged;
no resource creation is replaced by generation, upload, hidden renewal or IAM.
The cache lifecycle fixture sets the strict schema before creation and exercises
Create/Get/UpdateExpiry/Delete. Repro:
/private/tmp/prompty-cache-create-operation-repro.log.

After the final Go/godoc edits, all-module make lint and make test completed with
observed terminal exit 0, including race/examples/generator fixtures. Logs:
/private/tmp/prompty-operation-preflight-lint-final.log and
/private/tmp/prompty-operation-preflight-test-final.log. git diff --check passes.
IA16 has complete parent regression evidence, not final paired audit acceptance.
The full selected scope, expanded conformance and final independent audits remain
required; no release, remote provider production call or issue closeout occurred.

## Responses function history and explicit text results

Later checkpoint: function-result media is now mapped through the native result
content union in responses_function_result.go. It reuses existing source/URL/MIME
validation, but emits input_text/input_image/input_file result items, not user-role
messages. Ordered text, image HTTP(S)/bytes, document URLs and inline PDF preserve
boundaries/precision without local fetch/upload. InputMIME preflight descends into
ToolResultPart.Content; pointer/value parts and caller isolation are covered.

responses_function_result_test.go verifies an actual SDK call -> Previous -> mixed
result array, exact bytes/large-number text and explicit empty current tools.
Negative fixtures prove zero dispatch for IsError, disallowed MIME, ambiguous
source, URL credentials, unsupported audio/inline kinds and nested reasoning.
ExampleNewResponses_toolResults now includes a profile-approved image result.
IsError=true remains explicitly unsupported because the native item has no
independent flag: no automatic error wrapper or silent flag loss. Hosts may define
their own text/JSON payload with IsError=false. Audio/video/cache/state combinations
also remain unsupported. This supersedes earlier all-media-gap statements; it
does not waive expanded conformance or either final independent audit.

All-module make lint/test finished after the last Go/example edit with observed
terminal exit 0, including race/example/generator fixtures. Evidence logs:
/private/tmp/prompty-tool-result-media-lint-final.log and
/private/tmp/prompty-tool-result-media-test-final.log. git diff --check passes.

Later checkpoint: hosted stateless replay is now implemented in
responses_hosted_history.go. A scoped output-order manifest keeps original public
message/search JSON while private reasoning/function items reference canonical
indices only. Replay validates canonical projection, citations, exact public
annotations and required private envelopes. Source-less old metadata, wrong
scope, unknown/duplicate manifest, edited text/citations and duplicate native IDs
across messages fail before I/O; current hosted/local declarations remain explicit.
Native SDK ToParam retains original public fields/extensions and Number precision.

Actual SDK tests cover response wire -> AssistantMessage -> next stateless Create,
search/message/search order, a mixed reasoning/function/search history with host
text result, and StateOmit privacy/unavailable continuation. Private payloads are
absent from public metadata. ExampleNewResponses_hostedHistory is runnable offline.
One fixture initially confused an include selector with an input item; its detector
now checks the native type. Validation also avoids appending a nested manifest
while projecting a single message. Focused regressions pass. This supersedes older
hosted-history gap statements; broader conformance and final audits remain required.
Protocol reference: https://developers.openai.com/api/docs/guides/conversation-state
(manual replay preserves original response output items, including reasoning).

All-module make lint and make test finished with observed terminal exit 0 after
the last Go edit, including race/example/generator fixtures. Logs:
/private/tmp/prompty-hosted-history-lint-verified.log and
/private/tmp/prompty-hosted-history-test-verified.log. git diff --check passes.
Earlier pre-fix logs are not acceptance evidence; no final audit credit is claimed.

Completed original function items now replay through their required scoped item
envelopes. Canonical call_id/name/arguments must match; unfinished, edited,
unsupported, expired or foreign state cannot be reconstructed from text. Input
ordering preserves reasoning/function/text items. Native item IDs and call IDs
are checked for duplicates across history, without treating them as one namespace.

RoleTool accepts explicit text ToolResultPart values, in caller order, correlated
against earlier checked calls or Previous.ToolCalls. Known completed validated
calls are retained in the portable handle as ID/name references only, with wire
validation and isolated clones. Results cannot precede their calls, repeat a call,
refer to a foreign ID or change its name. Current instructions/tools remain
explicit; no handler runs, no polling/agent loop is introduced. IsError, media
results and cache/state decorations are rejected explicitly in this initial
mapping, not discarded. Broader supported combinations remain required work.

responses_function_history_test.go exercises actual SDK Create, response/handle
wire, two equal-name calls, reversed results, exact numeric text and ordered
native text arrays through stateless and Previous continuation. Adversarial
history/reference fixtures assert zero dispatch. ExampleNewResponses_toolResults
is runnable offline. The remote handle validator now delegates its tool-binding
checks to a private helper, preserving behaviour while passing complexity limits.

This supersedes older statements below that all function history/results are
unsupported. Hosted history, remaining result combinations, expanded conformance
and both final independent audits remain pending; no full row credit is claimed.
Live account/model integration, publication and issue closeout remain unperformed.

After the last Go edits, all-module make lint and make test both finished with
observed terminal exit 0, including race checks and generator/example fixtures.
Logs: /private/tmp/prompty-function-history-lint-final.log and
/private/tmp/prompty-function-history-test-final.log. git diff --check passes.
These gates certify this checkpoint, not completeness or final audit acceptance.

## Owner answer rechecked

The authenticated issue comments were re-read on 6 October 2026. The
[owner answer](https://github.com/skosovsky/prompty/issues/4#issuecomment-6008749968)
requires all five directions now; no direction awaits a workload/consumer decision.
Task37 now has an explicit question-to-decision acceptance map. Scope.md was
reconciled against current adapter entry points: hosted search and three batch
implementations are present, but presence is not full acceptance. Gemini file-backed
transport and globally ordered signed incremental state have implementation
checkpoints below; their former absent-implementation statements are superseded.
Responses ordered media input and reasoning-state assistant input are present;
responses_reasoning.go handles state separately from responses_input.go.
Expanded conformance and final paired audits remain required. Re-fetching all
issue comments found no later scope decision.

The following entries are historical checkpoints. Their pending-user-input or
absent-implementation statements do not override the current selected scope or
later evidence. No fresh implementation test or audit result is claimed by this
documentation reconciliation; no provider calls, release or issue closure occurred.

## Current checkpoint: native Responses local function declarations/results

responses_tools.go maps existing ToolDefinition inputs through the original
schema compiler and native SDK function declarations. Strict=false preserves
caller schema semantics; original completed arguments must validate before
successful tool_calls outcome. ForcedTool maps native named choice after its
separate profile gate. Local and hosted declarations coexist without the hosted
empty-tools override shadowing functions. Unresolved RequiredTools and invalid
names/schemas fail before I/O. No handler, tool runner or agent loop is installed.

Function output keeps canonical call_id, original arguments/order, and required
scoped original-item envelopes under openai.responses.function_call.json.
Optional native item ID is not fabricated or confused with call correlation.
Undeclared/duplicate/invalid/unfinished calls and forced noncall output retain
known response identity/partial evidence with errors. Wire preserves exact parts
and state. Tool-call history/tool-result continuation is still rejected until its
native mapping is implemented; this checkpoint is not full tool acceptance.

Handles persist ToolSchemaDigests/ForcedTool; Get/Cancel rebind original Tools,
check exact contracts, and validate results without generation tools/selection
support. Core wire checks hashes/selection and clone isolates the digest map.
Parent-confirmed IA15: changing dialect with identical source was accepted by
Get. responses_schema_binding.go now hashes normalized dialect/role/name/source
digest separately from diagnostic SourceDigest, for both output and tool bindings.
SDK regressions reject changed source/dialect/name before I/O; no legacy source-
only hash fallback. This preserves interpretation, not just equal schema bytes.

Actual SDK fixtures cover combined hosted/local request, native named choice,
strict=false, exact large enum, response/handle wire, Get/Cancel after generation
support removal, malformed/duplicate/unfinished/schema-invalid returned calls,
forced noncall and zero-I/O preflight/rebind failures. Optional-item-ID fixture
uses call_id without inventing item identity. ExampleNewResponses_localFunctions
is runnable offline; migration/contracts/README/task/matrix/closeout updated.

Full make lint and make test finished with observed terminal exit 0 after the
last source edits across all modules/race/examples/generator fixtures. Logs:
/private/tmp/prompty-responses-functions-lint-final.log and
/private/tmp/prompty-responses-functions-test-final.log. git diff --check passes.
Tool-result/history mapping, hosted history, expanded conformance and both final
independent audits remain required. Live model/account integration is unverified;
no publication, provider production call, issue comment or closure occurred.

## Prior checkpoint: required Responses state retrieval

RemoteResponseHandle now persists explicit ProviderStateRequired, including false.
The executable wire schema requires this boolean; absent/null/string historical
fields fail without a legacy default. Create records the current checked state
requirement; a fresh Create with Previous does not inherit it. Get checks the
independent state capability and composes encrypted-content include with hosted
includes through the actual SDK query encoder. Known queued/terminal/error handles
retain the requirement. Missing encryption retains the answer with
ErrStateUnavailable. Cancel needs its own operation capability, does not invent
an unsupported include or require retrieval/state generation support, and retains
the observed completion/cancel outcome plus missing-state error without auto-Get.

Parent-confirmed IA14 reproduction: TestResponsesReasoningOriginalSchemaAndGet
failed with native_json_schema unsupported after accepted output. observe now
checks operation capabilities first and attaches the original output schema only
for local rebinding/validation afterward. The actual SDK fixture passes both Get
and Cancel with native generation schema support removed, while preserving the
original schema digest/validation and exact large enum. No generation replay,
lowering, SDK mutation or unsupported-capability fallback is introduced.

responses_state_retrieve_test.go covers queued acceptance -> handle wire ->
Get with encrypted/hosted includes -> fresh Create without implicit inheritance;
unknown/unsupported state profile has zero I/O. Omitted state, unavailable/failed
Get and independent Cancel retain known identity/requirement and do not retry.
Core remote_response_test.go rejects missing/null/string state booleans.
README/contracts/migration/task37/matrix and author closeout guidance are updated.

Full make lint and make test finished with observed terminal exit 0 across all
modules/race/examples/generator fixtures after the last source edits. Logs:
/private/tmp/prompty-responses-state-retrieve-lint-final.log and
/private/tmp/prompty-responses-state-retrieve-test-final.log. git diff --check passes.
This completes this bounded mapping/correction, not full selected scope acceptance.
Hosted/local-tool history, broader provider-state/native/operation conformance and
both final independent audits remain required. Live integration is unverified;
no publication, issue comment or closure occurred.

## Prior checkpoint: Responses reasoning projection and stateless replay

responses_reasoning.go validates original reasoning items and projects only
exposed summaries to ordered ReasoningPart values. Encrypted/private source
remains in required destination-scoped opaque envelopes, without interpreting
encrypted content or copying it into public annotations. Missing/null/empty state
marks continuation unavailable; explicit provider_state Create requirements
request encrypted content and return ErrStateUnavailable with known output if
omitted. Malformed/unfinished items retain earlier content and incomplete outcome.

Assistant stateless replay checks expiry/scope/required codec, canonical summary,
part type, status and duplicate native reasoning IDs within the message, then
uses SDK ToParam for the original item. Text runs keep their positions around
reasoning; unsupported annotations/hosted/local-tool history remain fail-closed.
State expiry is checked again after pre-dispatch capture. Output-schema validation
uses answer text, not exposed reasoning summaries. No new core/provider types,
model-name heuristics, storage, automatic replay or hidden reasoning are added.

Actual SDK local fixtures cover Create/Get, original schema above float precision,
response wire -> restore -> next store=false Create, exact original source bytes,
source extensions, empty summaries, missing/null/empty encryption, malformed
summary/status, unfinished items, retained prefix, unknown state profile and
invalid/edited/foreign/expired history before I/O. StateOmit removes encrypted
and private content; ExampleNewResponses_reasoningState is runnable offline.

Both full make lint and make test completed with observed terminal exit 0 after
the final source edits, across all modules/race/examples/generator fixtures.
Logs: /private/tmp/prompty-responses-reasoning-lint-final.log and
/private/tmp/prompty-responses-reasoning-test-final.log. git diff --check passes.
This advances required state projection, not complete Responses acceptance.
Get encrypted-content include intent, hosted/local-tool history, expanded
conformance and final paired audits remain required. No live provider calls,
publication, issue comment or closure occurred.

## Prior checkpoint: ordered Responses media input

responses_input.go maps ordered text parts and user media through native SDK
content unions. Image HTTP(S)/raw bytes and document HTTP(S)/inline PDF map
without local fetch/upload/store. Shared actual-profile InputMIME gates run before
mapping; source exclusivity, MIME/kind, role and unsupported combinations fail
before I/O. Caller-owned URLs/data/messages remain unchanged. SDK request shape
now uses content arrays rather than merging user/assistant text fragments.

Actual SDK/local HTTP tests preserve seven mixed text/image/file parts, exact
base64 bytes, explicit instructions and a host-owned URL query. Only the Responses
endpoint is called, not referenced media URLs. Negative fixtures prove unknown/
unsupported MIME, ambiguous/missing source, kind mismatch, unsupported audio/inline
document, URL credentials/non-HTTP schemes, assistant/instruction media and nil
parts reject with no provider call and no sensitive URL in errors. Runnable
ExampleNewResponses_mediaInput passes; README/contracts/migration/task37 updated.

Full make lint and make test both completed with exit 0, including all modules,
race/example/generator fixtures. Terminal handles observed; logs
/private/tmp/prompty-responses-input-lint.log and
/private/tmp/prompty-responses-input-test.log. git diff --check passes.
This is input-mapping evidence, not full Responses acceptance. Local tools,
provider-state projection, expanded native/operation conformance and both final
independent audits remain required. Live media integration is unverified; no
real provider calls, publication or issue closure occurred.

## Prior checkpoint: global signed incremental source order

Partial state now includes increasing original source ordinals. Native frames
preserve gemini.source_part provenance for every content part: unsigned original
JSON or ordinal only when signed. Signatures never enter these annotations.
partial_order.go validates unique complete source coverage, exact signed sequence
bindings, original canonical projections and SDK numeric representability, then
reconstructs whole-message original parts in global ordinal order. No source order
is inferred from call completion or function name. Unsigned empty args follow the
SDK's omission semantics without altering signed source.

The actual Vertex SDK SSE/wire/HTTP fixture now interleaves two incremental calls
of the same function, with a late signature and trailing usage. Five original
parts reach the next request in their original positions; no signed merged Args
call replaces them. Missing/duplicate provenance and edited canonical content fail
before another request. The earlier interleaving-negative fixture is now positive,
including an intervening unsigned zero-argument call. Core wire/assembler tests
remain unchanged apart from the provider's additional source annotations.

Both full make lint and make test completed with exit 0 after the latest fixture,
including all modules/race/examples/generator fixtures. Terminal handles observed;
logs /private/tmp/prompty-global-part-order-lint-final.log and
/private/tmp/prompty-global-part-order-test-final.log. git diff --check passes.
Expanded native/cache/schema/capture/cancellation conformance and other required
directions remain incomplete, followed by both independent final audits. Live
integration is unverified; no publication, provider execution or issue closure.

## Prior checkpoint: contiguous Gemini signed partial continuation

partial_state.go adds a separate bounded ordered-parts codec, not a normalized
signed Args snapshot. partial_stream.go emits explicit StateUpdates once a
signature is observed, retaining unsigned source fragments too. Restore replays
the original partial-argument semantics and checks canonical content, completion,
required state, destination/backend and SDK numeric representability before I/O.
The assistant request expands the original parts without relocating signatures.

Actual Vertex SDK/local SSE+HTTP proves stream -> response wire -> restore ->
continuation: three original fragments/signature position survive, while edited
canonical content fails before another request. Consumer stop retains the latest
provisional signed sequence but cannot make it continuable. A separate fixture
proves the current per-call codec rejects global interleaving instead of silently
moving intervening parts. Full global-order interleaving remains a required gap,
not deferred or satisfied by rejection. NS-02 stays partial.

Full make lint and make test completed with exit 0 after the final formatting and
strict outer-codec checks. Both terminal handles were observed; all modules,
race checks, examples and generator fixtures ran. Logs:
/private/tmp/prompty-signed-partial-lint-final.log and
/private/tmp/prompty-signed-partial-test-final.log. git diff --check passes.
These checkpoint gates are not final independent acceptance. No live provider
request/release/issue closure occurred. Next work is the global ordering contract,
expanded conformance and other pending requirements before paired final audits.

## Prior checkpoint: Gemini file-backed Batch

NewFileBatches now explicitly injects the common host file port. All items are
checked before upload; an isolated no-egress SDK encoder produces native keyed
GenerateContent JSONL, then one upload precedes actual SDK job creation. Missing
port/scope/duplicates/input bounds fail before host I/O. Known upload Resources
survive unknown job acceptance, with SubmissionAttempted distinguishing job from
file effects. No fallback, retry, file store or cleanup worker is introduced.

Get retains safe observed output references from REST response or SDK
metadata.output. Ambiguous sources/file-inline unions fail explicitly. Results
downloads the observed output only, reuses result-only original validation,
preserves key correlation/usage/refusal/error/original unknown JSON fields, and
separately reports download/correlation completeness. HTTP status, line/total
limits, malformed suffix and close failures retain known prefix/status and close
the body exactly once. Fixtures and ExampleNewFileBatches exercise these paths.

Full make lint and make test completed with exit 0 for the current code after
the example formatting correction, including all modules/race/generator fixtures.
Terminal handles were observed. Logs: /private/tmp/prompty-gemini-file-lint-final.log
and /private/tmp/prompty-gemini-file-test-final.log. git diff --check passes.
This is checkpoint evidence, not final acceptance: expanded batch/cache/schema/
capture/cancellation conformance, signed Gemini incremental continuation, richer
Responses input/state and the final paired audits remain required. Live integration
is unverified; no real provider calls, publication or issue closure occurred.

## Prior checkpoint: result-only batch validation and IA13 correction

The previous goal turn delivered Gemini inline lifecycle/fixtures/example and
all-module gates. A deterministic actual-SDK repro now confirms IA13: cache valid
at Submit expires after remote acceptance, but old Results refuses retrieval.
All three adapters now use non-dispatchable PreparedBatchResultItem, checking
only the independent results capability and compiling original schema validation.
No request translation, input state replay/lease renewal or provider schema
lowering is reapplied. Digest/profile matching remains before retrieval. The old
submission-coupled ParseBatchItem path was removed, without a compatibility alias.

The red-to-green SDK regression proves Results after expiry and still rejects a
new Submit before I/O. Shared AAA fixtures cover expired opaque original state,
unknown results capability, zero mapper/Translate/Execute, original minimum/large
numeric enum and forced-tool selection despite unavailable generation support.
Diagnostics are results/local-validation evidence, not a native generation promise.
Final all-module make lint and make test both finished with exit 0 after the
exhaustive mapper literal correction, including race tests, runnable examples
and generator fixtures. Both handles were observed terminal; logs are
/private/tmp/prompty-batch-result-only-lint-final2.log and
/private/tmp/prompty-batch-result-only-test-final2.log. git diff --check is clean.
These are checkpoint gates, not final paired-audit acceptance.
The full file-backed/native/Responses/
cache/synthetic scope and final paired audits remain required, not deferred.

## Prior checkpoint: Gemini native inline BatchManager

The prior operation foundation now backs NewBatches with actual SDK Submit/Get/
Cancel and source-based inline Results, plus a compile-time BatchManager assertion.
All items are checked before Create; one-model scope, unique caller keys, lineage,
normalized execution digests and original Results rebinding are enforced. Exact
content JSON numbers are restored after SDK conversion. Cache expiry is rechecked
after operation capture and unknown acceptance retains safe observed identity
without retry. Results correlates metadata keys, validates canonical output through
shared prepared schemas and preserves usage presence/refusal/provider errors.

SDK fixtures verify the full lifecycle, reverse refusal/error/success, wire handles,
known-zero cached usage, duplicate inputs and changed Results contracts before I/O,
unknown accepted status, missing/duplicate/unexpected results and malformed item
prefix retention despite whole-job SDK conversion failure. ExampleNewBatches is
runnable. Native SDK content-number fixture now covers its actual []map converter
representation, not just JSON-decoded []any. The codec rejects mismatched shapes
and an oversized complete envelope before HTTP dispatch. Final all-module make
lint and make test both finished with exit 0 after these request/source regressions,
including race tests, examples and generator fixtures. Both handles were observed
terminal; logs are /private/tmp/prompty-gemini-inline-batch-lint-final3.log and
/private/tmp/prompty-gemini-inline-batch-test-final3.log. git diff --check is clean.
These are checkpoint gates, not final independent acceptance. Source inspection
also adds required pending B-RESULT-CONTRACT: result rebinding must not revalidate
generation-only remote leases. A deterministic expired cache/state repro and
result-only preparation remain required, without weakening Submit or changing the
original digest. Native file-backed transport and expanded
schema/state/options/capture/fault conformance remain mandatory; other providers,
signed incremental state and remaining Responses/cache/synthetic work are not
deferred or removed. No final independent acceptance, live execution or release.

## Prior checkpoint: Gemini native batch operation foundation

Previous goal turn made progress by reconciling owner scope/current evidence,
checking native OpenAI file capture coverage and completing all-module gates.
The next mandatory provider now has actual SDK inspection/cancellation paths,
checked destination/account/model profiles, scoped resource names, operation
capture and original bounded source observation. These paths are private until
the complete Submit/Results manager is wired; no completed Gemini BatchManager
or file/Vertex support is advertised.

SDK fixtures verify native URLs, cancel acknowledgement versus terminal outcome,
terminal cancel race, scope/capture zero-I/O, prior handle retention on malformed/
duplicate/foreign/unknown metadata, one call for unknown mutation and no raw error
leak. Original source retains integers beyond float precision. The batch-specific
reader bounds SDK consumption before decoding, rejects one-byte excess, limits
JSON nesting before duplicate checking and observes SDK-discarded Close errors.
Final all-module make lint and make test both finished with exit 0, including
race tests, examples and generator fixtures. Both process handles were observed
terminal; logs are /private/tmp/prompty-gemini-batch-operations-lint.log and
/private/tmp/prompty-gemini-batch-operations-test.log. git diff --check is clean.
These gates certify the checkpoint only, not the complete selected scope or
final independent acceptance. Submit, correlated
inline/file results, runnable Gemini batch example and expanded conformance remain
mandatory. This foundation is not acceptance and does not narrow the selected
scope. The full goal and both final independent audits remain outstanding.

## Prior checkpoint: OpenAI file-backed native Batch

The owner reply was fetched again and remains authoritative: all five selected
directions are required now, with consumer opt-in rather than implementation
deferral. NewBatches and NewBatchFiles now implement actual SDK multipart upload,
Chat Completions batch create/get/cancel and both error/output file downloads.
Core file ports declare destination/account binding without becoming a store.
Known upload resources and SubmissionAttempted distinguish file effects from
job submission; unknown acceptance does not authorize automatic retries.

Local SDK fixtures and runnable example cover reversed correlation, both files,
missing/duplicate/malformed/truncated results with retained prefix, exact original
schema validation beyond float precision, refusal/usage, rejected scope/IDs before
I/O, accepted unknown identity and capture failure after upload without job submit.
Required source capture rejects an opaque custom file port before I/O rather than
inheriting native SDK accuracy. Current all-module make lint and make test both
finished with exit 0; both process handles were observed terminal. Logs are
/private/tmp/prompty-openai-batch-lint-verified.log and
/private/tmp/prompty-openai-batch-test-verified.log. Tests include race checks,
examples and generator fixtures. git diff --check is clean. These are checkpoint
gates, not final independent acceptance. Gemini batch, broader conformance and
the remaining Responses/native/cache/synthetic scope remain mandatory. No final
paired audits, completeness percentage, live execution, release or closure claimed.

## Prior checkpoint: batch contract and Anthropic native transport

Previous goal turn made concrete progress: actual hosted search, source projection,
wire/history tests/docs and complete all-module gates. This turn continues the
full required batch family with a provider-independent explicit port/handle/schema,
lineage, input digest and independent download/correlation completeness. Native
Anthropic NewMessageBatches uses actual SDK Submit/Get/Cancel/Results; shared checked
item preparation and validation reuse the existing Messages profiles/schema/state/
cache/usage contracts, without invoking handlers. Capture is operation-level with
common policy and retained known results after failure. Per-item capture requirements
fail explicitly. Native SDK retries are disabled; accepted unknown ID is retained
when safely observed, while known request rejection is distinct.

SDK source inspection found JSONL Stream.Err omits scanner.Err. Results therefore
reads bounded original JSONL from the same native SDK request, using actual EOF,
line/total limits and close errors. Local SDK fixtures cover reversed success/error/
expired, duplicate/missing/partial retrieval, actual HTTP body truncation, a line
beyond SDK scanner capacity, limits, schema/refusal/known usage, digest/scope mismatch
before I/O and all five capture failure stages. ExampleNewMessageBatches is runnable.

Complete all-module make lint and make test finished with exit 0 after the final
rejection/classification and switch edits, including race tests, examples and
generator fixtures. Terminal handles observed; logs are
`/private/tmp/prompty-batch-lint-final.log` and
`/private/tmp/prompty-batch-test-final.log`. git diff --check is clean. These gates
are checkpoint evidence, not final task acceptance. No live provider execution,
release, issue comment/closure or final independent audit is claimed. OpenAI/Gemini
batch transports/file ports, wider batch conformance, signed incremental state and
remaining Responses/native/cache/synthetic scope are still mandatory.

## Prior checkpoint: Responses hosted web/file search

After refetching the owner reply, implementation resumed the required hosted
slice instead of repeating the scope decision. Actual SDK Create/Get now maps
separately checked hosted kinds/settings, requests sources/results and persists
selected kinds in isolated wire handles. Next Create sends explicit fresh tools,
including an empty list; no local inheritance or handler/corpus workflow exists.
HostedSearchItems retains provider output indices and original JSON through
MessageAnnotations and response wire/history, with typed sources/results and
presence-aware coverage. Exact numeric filters/attributes and known-zero score
are covered by local fixtures. Malformed results retain already observed text/
usage and completed remote handle but fail canonical success. Unsupported kinds/
profiles and malformed/reserved/duplicate settings dispatch zero calls.

Complete make lint/test finished with exit 0 after the final exact-field
regression, including every module, race tests, runnable examples and generated
fixtures. Logs: `/private/tmp/prompty-hosted-lint-final3.log` and
`/private/tmp/prompty-hosted-test-final3.log`; both process handles were observed
terminal. git diff --check is clean. The gates certify this checkpoint, not final
task acceptance. Broader Responses state/input/tools, selected batch transports, signed
incremental state and expanded final conformance remain required. No final paired
audits, completeness claim, live provider execution, release or issue closure.

## Prior checkpoint: OpenAI Responses remote lifecycle and continuation

Previous turn made concrete progress by reproducing/fixing IA11 and completing
all-module gates. This turn implements the next required operation family; it
does not substitute a smaller text-only target for the selected Responses scope.
All five owner-selected directions remain mandatory. Hosted tools, richer input/
state projection, three batch transports, signed incremental state, request-cache
controls and expanded conformance/final paired audits are still required.

Spec-first RemoteResponseManager reuses canonical execution and existing profiles,
with optional caller-driven Create/Get/Cancel and a strict secret-free handle wire.
Independent capabilities separate create/retrieve/cancel, continuation/background
and hosted web/file search. The separate NewResponses adapter uses an
openai.responses profile and actual Responses SDK ports, not Chat Completions or
an Invoker fallback. No SDK types enter core. Explicit Store is required; Previous
maps a scoped completed handle to previous_response_id and current instructions
are resent explicitly, including empty absence rather than local inheritance.

Create/Get/Cancel disable SDK retries. Operation-local middleware observes and
bounds the original response body before SDK decoding, closes it and retains
available unique safe identity on unknown accepted output. Duplicate identity is
not guessed. Unknown create/cancel never authorizes resubmission; failed retrieval
retains prior observation. Remote queued/running/terminal status is distinct from
canonical output outcome. Provider cancelled maps to canonical canceled; terminal
failure/incomplete, refusal and schema errors retain partial content/usage. Empty
completed output is not successful extraction. Usage presence keeps absent/null
unknown and explicit zero known, without double counting cached/reasoning subsets.

Native output schema reuses fidelity compilation and original validation. Handle
stores the original exact source digest; structured Get/Cancel require that source
again before I/O. Retrieval validates original schema without reapplying request
lowering. Actual SDK fixtures preserve an integer beyond float precision in schema
and output. Foreign endpoint/account/model/provider and path IDs fail before I/O.

All three operations use common semantic capture stages, default metadata-only
redaction, observed HTTP status, original provider JSON and one terminal attempt.
Required early sink failure means zero calls; post-dispatch failure keeps accepted
handle/output without retry or rollback. An unacknowledged terminal remains not
Closed/Complete, rather than claiming a durable closeout. Truncated/unavailable
source emits only observed metadata when available. No HTTP bytes/physical attempt
coverage is claimed. Host owns credentials, data/egress/access/persistence and
when to inspect; no scheduler/store/IAM worker or implicit Wait loop is introduced.
Store=false is not a no-retention guarantee; current primary background guidance
was checked and linked in the adapter README.

Evidence: remote_response_test.go; Responses SDK lifecycle/continuation/scope/
unknown acceptance/schema/outcome fixtures in responses_test.go; capture stage
faults/default redaction/source metadata in responses_capture_test.go; executable
ExampleNewResponses. Input remains text/native-output only at this checkpoint;
unsupported hosted/local tools/richer parts/settings/state fail before I/O, not
silently disappear. Their implementation is required next, not rejected/deferred.
The matrix records partial evidence, never full completion credit or final audits.

Final all-module make lint exited 0
(/private/tmp/prompty-responses-lint-final2.log) and make test exited 0
(/private/tmp/prompty-responses-test-final2.log), including race, runnable examples
and generator fixtures. Both process handles were polled to terminal success;
no checks remain running. git diff --check exited 0. Earlier lint failures were
formatting/assertion style fixes; initial capture fixtures needed JSON content type
and an accurate unacknowledged-terminal expectation. No live provider call, commit,
release, external comment or issue closure occurred. Live account/model conformance
is unverified. No final completeness percentage is claimed.

Source SHA-256: remote_response.go
67e3ef62e733970c4dba8e6b43e183239108282d9699e692325b8eb40f8aba59;
adapter/openai/responses.go
5d91a5d42a62c8acc471fc5e1fa14191c1417b5a3d277b658cac7580f65fcaa5;
adapter/openai/responses_parse.go
c1c37d9e8efcf9f8b3bf6d0e122419f612690ef7974bae578e5720185019ccf2;
adapter/openai/responses_source.go
ee422c54bdc04ed9dda2f30cc1a18ec1c1da57020340038256f27bdd9c0fa0d5;
adapter/openai/responses_capture.go
afce1811c97c5a3700c9643e16198256a8364dffa84cc32815084486739a57ed;
schemas/remote-response-wire.json
3f3e50a53d8d59d6b1dbe018fb626a00410d236f3531d7c2cf2e7cbcae31fd71.

## Previous checkpoint: IA11 cache expiry at logical dispatch

Previous turn made progress by fetching current issue comments, reconciling
implementation inventory with the owner reply and polling both cache-capture
gates to terminal success. This turn confirms and fixes a time-of-check gap,
without reducing the required five-direction scope.

Before correction, actual SDK fixtures allowed cache reuse after a handle expired
inside a host capture callback or a native StreamStart consumer pause. Create and
update likewise submitted expired absolute expiry after capture. These were
real pre-dispatch contract violations, not live provider tests. PreparedRequest
now keeps a private handle snapshot independent of optional capture and checks
it through the actual mapping port immediately before sync/buffered/native
dispatch. Lifecycle create/update recheck absolute expiry after capture.
Sync terminal capture now uses the actual prepared.dispatched flag rather than
unconditionally claiming post-dispatch. Stream closeout stays owned by its
original handle. No retry, recache, scheduler, IAM, remote lease or rollback is
introduced.

Permanent AAA TestCacheReuseExpiryAtDispatch covers all three modes and requires
typed expiry error, zero provider calls and exactly one pre-dispatch capture
terminal. TestCacheAbsoluteExpiryAtMutationDispatch covers create/update and
requires a closed capture report without unknown remote acceptance. The initial
fixtures failed on the previous source; they pass after correction. IA11/P2 is
recorded in the task, contracts, matrix and migration, subject to final audits.

Final all-module make lint exited 0
(/private/tmp/prompty-cache-dispatch-lint-final2.log), and make test exited 0
(/private/tmp/prompty-cache-dispatch-test-final.log), including race, runnable
examples and generator fixtures. Both original process handles were polled to
terminal success; no checks remain running. Earlier lint attempts found only
formatting and embedded-field layout issues; these were fixed before the final
successful gates. git diff --check exited 0. No live calls, commits, release,
external comment or issue closure occurred. Expanded conformance and final
paired audits remain required; no final completeness percentage is claimed.

Source SHA-256: adapter/prepared.go
c29f3a6a983757add2eba464c980675b918321dc37e088ca077fd43074592066;
adapter/client.go
b9caa61872929dabf3ea2ddd5e7c0f0a0b37f2a5f51f195923ac4834c7504499;
adapter/capture.go
5a51943891618e53537ab95f436b2e39ff315359ec2ea5ef427c69fb95b62d1c;
adapter/gemini/cache_resources.go
bf397a66c563530e81b8a896e11c6ba1ff250e045273ff46005d5bea510904c2;
adapter/gemini/cache_dispatch_test.go
03ece6b6fe667deae802d53bff14a6c6c3a8d4e9827d93a2e8425c6093079b3d.

## Previous checkpoint: owner reply revalidated and cache operation capture

Fetched all current issue #4 comments again and confirmed the authoritative owner
reply remains https://github.com/skosovsky/prompty/issues/4#issuecomment-6008749968.
All five directions are required now. Optional means consumer opt-in, not deferral;
the complete expanded scope, library boundaries and clear break remain unchanged.
Main task inventory, scope gaps and matrix now describe actual cache/synthetic
implementation rather than obsolete missing-adapter/capture notes. No issue
comment, release, live provider call or closure was performed.

Cache lifecycle now uses the common CaptureSession for create/inspect/update/delete.
Create takes execution capture intent; the other operations accept at most one
runtime CacheResourceOperationOptions with a non-persisted binding. Five semantic
stages preserve original bounded provider JSON and observed HTTP status. The default
projection omits payload, identifiers and endpoint; no transport byte/physical
attempt fidelity is claimed. Required coverage rejects unsupported intent before
dispatch. Sink/cancellation/durable-ack failures before dispatch do not call the
provider; capture failures after dispatch retain known accepted resource results
and do not trigger rollback or retry. Malformed/partial source capture reports
only observed metadata rather than fabricated JSON or complete source fidelity.

Evidence: adapter/gemini/cache_capture.go and cache_capture_test.go, plus existing
lifecycle/example fixtures. Both completed all-module processes were polled to
exit 0: make lint (/private/tmp/prompty-cache-capture-lint-final.log) and make test
(/private/tmp/prompty-cache-capture-test-final.log). These are local SDK fixtures,
not live model/account verification or final paired-audit acceptance. EXT-C-01
remains partial; expanded lifecycle conformance, Responses, three batch adapters,
signed incremental state, request-cache controls and synthetic conformance remain
required. No final completeness percentage is claimed.

Source SHA-256: cache_resource.go
ad6cf9a45957a06eebf03eac820e41f50fea2cb93b4719cb79b36bca7ba21b12;
adapter/gemini/cache_capture.go
6ebbd6bba26f4c69e69ba540694aa1b5988276ba09e7b8330cdd67707c83c18b;
adapter/gemini/cache_capture_test.go
1fe288e5eb7e4e3f324ae2262727170c9d11a4d36f56aa4c158c899d1876a73b;
adapter/gemini/cache_resources.go
406abc2795ba52b1253eec742359c8f863739e0fc8c43495c9ca32b8ec505c92.

## Previous checkpoint: Gemini managed cache lifecycle and checked reuse

Previous turn made implementation progress on the explicit synthetic-output
adapter and all-module gates. This turn adds the next required provider-specific
operation family, not a smaller acceptance target. All five owner-selected
directions remain mandatory; EXT-C-01 is partial pending operation capture/
redaction, broader protocol/cancellation conformance and final paired audits.

Spec-first optional CacheResourceManager reuses canonical immutable execution
input and existing ProfileIdentity. Its portable format-marked handle stores
host account binding, provider ID, content/config digest, observed expiry/state;
no SDK types, credentials, daemon, scheduler, application store or IAM enters
core. CacheResourceExpiry requires exactly one positive TTL/future expiry.
Executable handle schema, strict restore semantics, pointer-isolated clone and
execution wire preserve the binding without recreating runtime transport/account.
Resource TokenCount distinguishes absent/null from explicit zero and is not
generation billing usage.

Gemini NewCacheResources maps actual SDK Create/Get/Update/Delete through an
isolated injected client and bounded original response observer. Independent
operation capabilities and account/destination binding fail before dispatch.
Generation-only options, output schema and cache hints are rejected, not silently
repurposed. Tool schemas reuse original compilation/fidelity checks; system-only
cache input does not acquire the adapter's synthetic generation user trigger.
Digest covers immutable translated SDK configuration, excluding expiry/auth.
Source response identity/model/name, unique fields/case-sensitive reserved keys
and counter presence are checked before accepting active state. Unknown mutation
keeps reconciliation identity where available and never retries. SDK decode
failure after accepted create can retain a valid handle as unknown; it cannot
become usable success. Invalid/malformed/oversized mutation result cannot replace
known identity or become active. SDK errors are not echoed with possible secrets.

PromptExecution.CacheResource plus WithCacheAccountBinding maps explicit
generation reuse to SDK CachedContent. Profile knowledge alone cannot invent
the actual mapping port. Other adapters reject the resource even on raw
Translate; effective foreign model/endpoint/account, expired/unknown/deleted
state fails before I/O in sync/buffered/native preparation. Base reuse does not
override cached system/tools configuration; conflicts are explicit. Common
generation usage keeps cached tokens as a subset without double counting.

Evidence: cache_resource_test.go; Gemini cache_resources_test.go and executable
cache_resources_example_test.go. Real SDK/local fixtures exercise full lifecycle,
wire restore/rebinding, creation→generation, invalid bindings/no hidden upload,
known zero/unknown counters, source faults/bounds/identity and no retry. Docs and
main task inventory reflect implemented APIs and limitations. Local fixture
profiles use google.golang.org/genai v1.48.0, Gemini API backend v1beta with model
fixture; this is not live model/account conformance and not arbitrary Vertex
endpoint verification. Live calls, release and issue closure remain unauthorized.

Final all-module checkpoint `make lint` exited 0
(/private/tmp/prompty-cache-resource-lint-final.log); `make test` exited 0
(/private/tmp/prompty-cache-resource-test-final2.log), including race, runnable
examples and generator fixtures. `git diff --check` exited 0. No process handles
remain running. This is a checkpoint, not final acceptance. Cache resource core
SHA-256: b7dc291ef595bd2b4b5167cf28b42b888d4655fe4ea68f14654f00c32f96e8c2;
Gemini cache_resources.go SHA-256:
8b2ace7f69acec3d36618f9093b8099d60e103db14cf263eb69353d5a74b8713.
Scope still includes Responses,
three batch adapters, expanded native/signed incremental state, request-cache
controls and remaining synthetic conformance. No final percentage is claimed.

## Current checkpoint: explicit Anthropic synthetic-output adapter

Previous turn revalidated the owner reply but made no implementation changes.
This turn advances required EXT-S scope with a separate adapter, not a reduction
of the goal. All five owner-selected directions remain mandatory. No final
completeness percentage or paired final-audit claim is made.

Spec-first OutputMode distinguishes native output from explicit synthetic_tool,
with wire/clone support and typed portable-shape validation. Required synthetic
output checks separate synthetic/tools/forced-selection capabilities and actual
adapter ports. Ordinary raw adapters reject the selection. NewSyntheticOutput
owns a private per-request output name and sends exactly one SDK output tool
plus native named choice, without OutputConfig.Format. Real tools/collision,
invalid name/object schema and manual enabled thinking fail before I/O. Unknown
remote acceptance is not retried; no output handler is invoked. Successful
extraction publishes JSON text, not an executable ToolCallPart; checked finalizer
validates the original schema. Raw parse is explicitly not schema acceptance.

Structural errors have stable SyntheticOutputError reasons. Refusal/truncation
keep OutcomeError, partial output and usage presence. Duplicate object keys are
rejected without numeric conversion; original large schema/argument numbers
survive actual SDK HTTP fixtures. Unexpected signed thinking keeps scoped state
before structural rejection, text citations are retained, and SDK RawJSON stays
available for semantic source capture. Request binding is not mutable shared
adapter state. Sync and explicit buffered modes have one terminal and preserve
known JSON on original-schema mismatch. Separate synthetic native streaming
is not implemented or claimed; profile knowledge cannot fabricate its port.

Evidence: output_mode_test.go; anthropic synthetic_test.go,
synthetic_buffered_test.go, synthetic_example_test.go. Executable example and
migration/adapter README/closeout describe the implemented API. Expanded
capture/redaction/reasoning/concurrency/protocol conformance and final independent
audits remain required; EXT-S-01 is partial, not complete. Dedicated Responses,
three batch adapters, Gemini managed cache and signed incremental state still
remain required. No live provider calls, release, external comment or closure occurred.

All-module final checkpoint `make lint` exited 0
(/private/tmp/prompty-synthetic-lint-final3.log); `make test` exited 0
(/private/tmp/prompty-synthetic-test-final3.log), including race tests, runnable
example and generator fixtures. `git diff --check` exited 0. These are local
checkpoint gates, not final goal acceptance or live conformance. No process
handles remain running. Core output_mode.go SHA-256:
04367ad987ad1b8cd29d47934a4b9dfbe632447c432c17f3aeda09dd21306eb8.
Anthropic synthetic.go SHA-256:
19728eaffa04b032fd5eca9916be676787446a677f2278e5c2a147afb13a978d.

## Current checkpoint: explicit tool-state updates foundation

Previous goal turn made implementation progress (Gemini message metadata, actual
regressions and all-module gates). This turn added the missing common state-update
contract needed for signed incremental argument binding. Full selected scope and
NS-02 remain unchanged/partial; provider integration is not claimed complete.

ResponseChunk.StateUpdates binds a current frame tool part to an explicit opaque
state replacement using validated logical identity. updatedPartStates builds a
pure replacement plan before usage/content/state commit. It rejects changed
scope/codec/required/expiry semantics, malformed envelopes, duplicate/mixed
declarations, non-tool updates and ambiguous previous bindings. Ordinary state
remains append-only. Latest state survives correlated argument assembly,
cancellation, clone/history/wire and host state omission. No SDK types, signature
interpretation, daemon or state storage backend enters core. Canonical source
capture includes current state plus explicit state_update_parts indices, keeping
ordinary vs replacement semantics distinct.

Parent-confirmed defect corrected: common assembly converts *ToolCallPart to its
value representation; old reflect.DeepEqual wrongly treated an unchanged signed
pointer call as changed state-bound content. sameStateBoundContent normalizes only
tool pointer/value representation and still compares every actual field.
Permanent ID-less pointer/interleaved/state-binding tests prove this fix and the
unchanged-content-without-update failure invariant.

Evidence: stream_state_update_test.go, stream_state_update_example_test.go and
adapter/capture_test.go. Final all-module `make lint` exited 0
(/private/tmp/prompty-state-updates-lint-final2.log); `make test` exited 0
(/private/tmp/prompty-state-updates-test-final2.log), including all modules, race
checks and generator fixtures. `git diff --check` exited 0. This is a checkpoint,
not final acceptance. HEAD remains the original snapshot plus dirty worktree.
Code SHA256: stream_state_update.go
`599758da36cc91acc85b73179fcfdbcbddb6ae0118b683cc1be78c6195e7f510`;
stream_state_update_test.go
`2943a638e909e35cefac91cf15ac3e7d2e928180bb8011bedd805137c9d6b41b`;
stream.go
`0bdc23fcb8f9acf300809783bfa2e9d80b59b6d2071efde2c244d50499e6365e`;
adapter/capture.go
`fea5dc34cdb1b8a4737f41fa319fecfc729a34e9b2dad606b3906a99d7a65e6f`.

Next required slice remains Gemini signed incremental codec and continuation
transport, using the update port. Source protocol guidance requires returning
original signed parts, not attaching a signature to a newly invented aggregate
part. Preserve source event order/required state and reject unprovable SDK
encoding before dispatch. Fixture support is not live endpoint/model evidence.
No live provider call, release, issue comment/closure or final paired audit.

## Previous checkpoint: Gemini message-level metadata

6 October 2026, HEAD `860146dd6e74154abbaad415d9a44e112a05c16d`
plus the dirty worktree. The preceding reply-fetch turn reconfirmed already
recorded scope but made no implementation progress; this turn implemented the
next safe required slice, without changing scope or requesting external action.

New provider-independent `MessageAnnotation{Kind, Value}` is carried by response,
chunks, history, clone paths and both wire schemas. Stream acceptance validates
the envelope before committing any content/usage/metadata and retains ordered
message updates through finish, materialization and failed partial status.
Canonical full/provisional capture includes message metadata; host policy still
owns redaction. No content carrier, fake part binding or provider SDK type enters
the core. Metadata is JSON data, not an HTTP-byte replay promise.

Gemini candidate citations/grounding now use MessageAnnotations instead of part
zero. Source values retain numbers/explicit zeros/empty strings; REST
citationSources is normalized only in the private SDK decode copy. Native updates
can arrive without content after finish. Grounding indices refer to previous and
current provider chunks, checked independently of canonical content. Invalid
null/range/negative/score-count/segment/case-alias associations fail without
dropping accepted earlier metadata/text. SDK-only input without observed source
has SDK-semantic fidelity, not fabricated source bytes. Runnable native example
reads late citations; migration/closeout drafts describe the new field.

Evidence: message_annotation_test.go; adapter/capture_test.go;
adapter/gemini/message_metadata_test.go; updated state/native example tests.
Full `make lint` exited 0: /private/tmp/prompty-message-annotations-lint-final2.log.
Full `make test` exited 0: /private/tmp/prompty-message-annotations-test-final2.log,
including all modules, race checks and generator fixtures. `git diff --check` 0.
Final code hashes: provider_state.go
`77a3fa30086139723807001cdf3b6d0f18fa9f632f480e16271e26c600c387ca`;
Gemini message_metadata.go
`2f470b999809420c5ae88f4b49ac45e655137ac2ee965add620a34ca2c834429`;
Gemini message_metadata_test.go
`2eb5e9a4914286c590b372dad021e2c8c6eca61bc446a91a0ac8e9370ca882be`;
Gemini stream.go
`b5222d5f3fd005f052c27499dac906f3e2879051749c6aeb47e243567600c9c0`.

NS-02 remains partial: signed incremental binding and expanded conformance are
required. Responses/batch/managed cache/synthetic output and final paired audits
remain mandatory. No final completeness percentage, live request, release,
GitHub comment or issue closure. Next implementation slice: signed incremental
Gemini continuation association, preserving both exact provider state and final
tool argument semantics rather than attaching state to a changed provisional part.

## Previous checkpoint: unsigned Gemini partialArgs native wiring

Connected partial_stream.go to native canonical mapping. Provider-ID calls can interleave; ID-less continuation is permitted only for one unique open call and ambiguous named restarts fail. Exact source functionCall progress is retained as gemini.partial_function_call annotation on an empty-Args provisional call. Provider close produces one complete snapshot on the same logical reference, reused by common correlation/schema finalization; no invented IDs, fake JSON ArgsChunk text or handler execution. Transactional path/value accumulator retains numeric lexemes, string fragments and presence. Complete raw call snapshots remain a separate mode. Signed incremental calls still fail explicitly until required continuation binding is implemented.

Added CapabilityIncrementalToolArguments/gemini.partial_args endpoint/model opt-in and optional boolean control. Actual Vertex SDK JSON includes streamFunctionCallArguments=true. Unknown/unsupported profile knowledge rejects before I/O, installed MLDev cannot invent its absent mapper, sync/buffered reject incremental requests, and forced native choice is retained. Added nil raw Execute request guard. Permanent partial_stream_test.go/control_test.go prove request mapper/source/identity/progress/precision, reversed interleaving, sequential same-name ID-less calls, retained bad/open/EOF cases, preflight zero-I/O and sync rejection. partial_example_test.go is executable local Vertex protocol example. No credentials/account/model live evidence.

Final module race suite EXIT0 (1.902s), lint EXIT0 zero issues. Full all-module make lint `/private/tmp/prompty-gemini-partial-lint.DGJEaU` and make test `/private/tmp/prompty-gemini-partial-test.5uR4XB` both terminal EXIT0, including race/generator fixtures; all production/test source changes completed before those gates, afterward docs reconciliation only. git diff --check EXIT0. SHA256 partial_stream.go `50938963798dd52dc770388e2f927ecaa56a75479745f552d91ee19579ae3491`; partial_control.go `b8209e2598627f339ae47952a62bdd05adbe8c6a6e71bb335c1a36d62c8c8c24`; partial_stream_test.go `6f76f118ddffc7dc882f4895be693aa8dcb20c567653aa0d661d86305cbc63e2`; partial_control_test.go `8c815e611c9714d37bf0d3f98f79ea48d65ff9f70512586b889656e785bd47d3`; stream.go `fc31497003d9a8f2a859fbc3a0da4181f4721e6289d54ec51d09fafa0f84be20`. No final independent audit verdict or completed row credit. Signed incremental binding, metadata-only associations and expanded native conformance remain REQUIRED; Responses/batch/cache/synthetic scope also remains intact. No publication, new issue comment or closure.

## Previous checkpoint: private Gemini partialArgs accumulator

Added partial_value.go/path.go/arguments.go before native wiring. Decode original scalar JSON with Number and explicit oneof presence; distinguish empty/false/REST null from absent closing marker. Private singular-path parser supports dot/quoted member and nonnegative array indices, rejects unbound queries and depth exceeding installed encoding/json validation. Sparse array storage depends on actual entries, not untrusted index magnitude; final materialization requires contiguous indices and closed strings. Transactional updates clone before applying a whole fragment batch, reject shape/type/closed-path overwrites and never fabricate missing nulls. Provider-specific accumulator remains outside core and reuses common final argument/correlation validation when connected.

Permanent partial_arguments_test.go covers exact large integers/long decimals, nested out-of-order arrays, string continuation and valueless closure, false/null/empty presence, quoted/Unicode keys, atomic rollback on bad later field, duplicate/case-aliased/unknown fields, invalid/query paths and huge index/depth safety. This is private unit evidence only; native StreamEvents/request/correlation/progress/signed-state wiring remains REQUIRED and unimplemented. No completed native credit or final audit claim. Final current module race/lint results follow when terminal; previous all-module gates do not certify this delta. No live provider operation or external publication.

Final Gemini module `go test -race ./... -count=1` EXIT0 (1.900s), `golangci-lint run --allow-serial-runners ./...` EXIT0 zero issues and git diff --check EXIT0. Confirmed installed encoding/json/scanner.go maxNestingDepth=10000 before applying the matching private path bound. SHA256 partial_arguments.go `735d906becba32614bd19a4bb68b1b34c61db543c900e897d5318d41c550158d`; partial_path.go `916d5d3e3bca8022a905440d6079de08c3be1f517b7c1a1740d33000d85f1be0`; partial_value.go `5abb829a1e52f30d96cae28f20b07d8569b51413baeb8005240bc80b9082c965`; partial_arguments_test.go `5f454cc09e3e31ff0035264d9b596da8795efecc6b96c9838519626fd43fb695`. Final all-module frozen gates and both audits remain mandatory.

## Previous checkpoint: checked local tool correlation foundation

Before incremental Gemini protocol assembly, inspected the common stream materializer and confirmed it ignored already-checked logical PartIDs for ID-less tools. Added private assembleToolCallPartsWithReferences; native materialize supplies the owning handle's checked references. Provider and local keys use disjoint namespaces; final calls retain the original empty provider ID. Standalone assembly still rejects unproven ID-less deltas. Snapshot/delta/name/object validation is unchanged; handler admission still requires independently checked explicit unique-name correlation. This adds no provider SDK types to core and no name/adjacency guesses.

New stream_local_tool_test.go proves two same-name ID-less interleaved calls, reversed finish order, large exact integer, annotation remapping/single terminal, disjoint provider/local identities and conflicting/mixed/incomplete failures. Initial targeted root/shared-adapter/Gemini race suites EXIT0 (5.427s/1.798s/1.765s). Final root lint EXIT0 zero issues after extracting source/identity/mode preparation to satisfy complexity lint. Full make test `/private/tmp/prompty-local-tool-test.tmSVRw` terminal EXIT0; the final semantics-preserving helper extraction occurred while it ran, so this mixed-timing run does not certify a frozen final snapshot. Fresh targeted race verification follows below when terminal. Gemini partialArgs/JSONPath protocol assembly is still REQUIRED and unimplemented, not waived or complete. Final all-module frozen gates and paired independent audits remain required.

SHA256 tool_args.go `77f5bed63e1f87545cf4798c797aa73929ee361e12dcce1a46e3d21f7f4bc296`; stream.go `042866aa09d01883e85fd22ddb6b98f1245fc78fd4904db284112102f4beab50`; stream_local_tool_test.go `fe89753dbd1341461b71ed4a1444a21001dc36d46ba31330252d14bcf72e84e3`. No live provider request, release, comment or issue closure.

Fresh final-source `go test -race . ./adapter ./adapter/gemini -count=1` EXIT0 (5.155s/1.782s/1.765s); no production/test change afterward. Root lint EXIT0 and git diff --check EXIT0. This is bounded parent evidence, not full final acceptance.

## Previous checkpoint: Gemini initial native transport

Implemented stream.go StreamEvents/StreamEventsForProfile using the actual isolated genai GenerateContent SSE path and shared checked lifecycle. Canonical source decoding bypasses response custom UnmarshalJSON with typed alias/UseNumber; signed source codec checks precede SDK conversion. Ordered complete parts/tool snapshots preserve exact integers, scoped state and wire; source semantic capture and late usage are retained. Caller config/system instruction/headers are copied where SDK mutates defaults. No sync fallback/replay. Source retains scanner read errors because this SDK logs rather than yields them; a prior finish cannot convert read failure into success. Malformed/body-bearing conversion failures have safe static errors; HTTP API failures retain status without body echo. Removed lossy ParseStreamChunk and its obsolete tests, replacing them with actual native fixtures and executable example.

Permanent SDK/local SSE tests cover ordered early text/two same-name calls, exact large args, full model-family signed scope/wire, late usage/known zero, refusal, missing/duplicate finish, post-finish content, malformed shapes/duplicate keys/signed aliases, retained partial failure and read failure after finish. Blocked-server break/cancel proves early publication and body release without caller config mutation. Source bounds/read failures remain covered. Incremental partialArgs and metadata-only citation/grounding binding still fail explicitly and remain REQUIRED implementation; broader capture/schema/concurrency/model-profile conformance and final independent audits remain pending. No complete row credit, live/provider call or publication.

Final current module race suite EXIT0 (1.761s); module lint EXIT0 zero issues. Full all-module `make lint` `/private/tmp/prompty-gemini-native-lint.PCcX3M` and `make test` `/private/tmp/prompty-gemini-native-test.X6oIgZ` both terminal EXIT0, including race/generator fixtures. Source/test changes were complete before full gates; later edits are docs reconciliation only. git diff --check EXIT0. SHA256 stream.go `4a67fbef6c25d5dfcdfff9f110ffb55aaeee0efc9d86e90279ab2e64ebcfda02`; stream_source.go `ee91e8cf5f283f2e583d36704c20e35c07fc4bdc365210324940657263e7ae84`; stream_test.go `0543bf375d675c736d4b3de9bb43582cf59d0e9dd027093121921754a5399c85`. Full scope remains active; historical percentages/paired verdicts do not certify these additions.

## Previous checkpoint: Gemini native source observation foundation

Added invocation-local bounded SSE source observation to the isolated Gemini response transport. Each body read exposes at most one source line, preventing SDK scanner read-ahead from associating a later JSON payload with the current callback. Only the current source JSON is retained; no whole-stream accumulation, provider retry, host client mutation or transport-byte accuracy claim. The actual installed SDK fixture verifies callback/source association for early text and an exact large tool integer, consumer break, one dispatch and one body close. Bound/small-read/EOF/interrupted-line fixtures cover rejection without truncated publication. This is foundation only: Gemini StreamEvents/lifecycle, incremental arguments, state/usage/capture mapping and runnable checked example remain required and unimplemented. No complete requirement credit or final audit claim.

Final foundation module checks: `go test -race ./... -count=1` in adapter/gemini EXIT0 (1.725s), `golangci-lint run --allow-serial-runners ./...` EXIT0 with zero issues, git diff --check EXIT0. Source SHA256 stream_source.go `82fe45b3344d19f02da2b5221441cec9f2adfe112dd92a94c04398a2543df8e1`; stream_source_test.go `bda42af8c1913fc5047cf0e236cdf3f876b0d5d2592630a97e60a468be45be2a`; response_source.go `d78973265cf2fb407a893293f749ff7d5c903633d702ef7bd4d8d50b0138db9c`. Earlier all-module gates precede this production/test delta; final full gates and paired audits remain mandatory. No provider live call, publication or issue closure.

## Previous checkpoint: IA10 parent correction

Implemented provider-scoped ChatClient and NewHTTPChatClient with injected endpoint/host HTTP policy and positive per-frame limit. Raw JSON argument objects are decoded with Number before SDK conversion. Sync/native/buffered and response-wire → actual next HTTP request preserve nested integers, long decimals and 1e1000. SDK-decoded unverified floats fail closed; the real SDK precision fixture confirms one HTTP call and zero tool handlers. Outbound history no longer uses the SDK float decoder. Core assembler uses the existing Number-preserving object decoder; dynamic DecodeToolArgs retains Number, with permanent root regression.

Targeted final root/Ollama/shared-adapter race suites EXIT0. Full all-module make lint/test subsequently terminated EXIT0 as recorded below; those gates precede the new Gemini source/test changes and do not certify them. IA10 has parent correction evidence, not final independent acceptance. Remaining five-direction implementation and final paired audits stay required.

## Previous checkpoint: Ollama native transport and original precision finding

IA10 full checkpoint gates terminal EXIT0: make lint `/private/tmp/prompty-ia10-lint.h1MyQg`; make test `/private/tmp/prompty-ia10-tests.bLFsEm`. All modules, race and generator fixtures included; git diff --check EXIT0. No source/test changes after these commands started; subsequent edits reconcile evidence/docs only. Parent correction is not final independent approval. Broader Ollama conformance, Gemini native, Responses/background/hosted, three batch transports, Gemini cache lifecycle and explicit Anthropic synthetic output remain required; full goal stays active.

Checkpoint SHA256: chat_http.go `0091ce046813605e1f828096cb576de13a26bd136d3b52d57c4e62d662b0b0db`; precision_test.go `c75a1b8645360b5ca99a4551c4d036dc9b256040867bac319ad41668a6d523db`; tool_args.go `7c17ae72b14a3fe4c19dd6a6e3a28d78ad42850fc2369c30aefebc9498edb291`; tool_binding.go `005f2ae1c2906cf3f9392e4de91af52a6b5a3d1c1022264bbc967d3b73b3e448`; tool_number_fidelity_test.go `442d55b8cd193c3fab7a10f8db6791ee430caabdc031a61b2c511888e2d4a9bf`. Both final auditors must verify the eventual full worktree, not extrapolate these bounded hashes/gates.

Implemented actual Ollama Chat native callbacks with explicit stream=true, no fallback/replay, body cleanup, snapshot argument representation and shared lifecycle. SDK indices are not used as fabricated correlation. Tool results preserve tool_name; explicit checked unique-name helper policy remains required. Sync now copies the request rather than mutating caller Stream. Local SDK/NDJSON tests cover repeated/conflicting snapshots, late usage, transport/protocol faults, body closure, true early text from a blocked server, and runnable example.

Parent-confirmed IA10/P1: installed SDK decodes argument numbers through float before callbacks; actual HTTP fixture sends 9007199254740993 and canonical args silently contain 9007199254740992. The characterization assertion proves the defect, not fidelity acceptance. Required next correction: authoritative number-preserving ingress or explicit fail-closed rejection, including sync/native and request continuation, before handlers can use unverified values. Ollama requirement remains partial. Historical independent verdicts do not certify this new implementation/finding; final full gates and paired audits remain mandatory.

## Previous checkpoint: Anthropic native transport implementation

Ollama checkpoint terminal checks after characterization/source changes: `go test -race ./adapter/ollama ./adapter -count=1` EXIT0 (1.495s/1.541s package summaries); Ollama `golangci-lint run --allow-serial-runners ./...` EXIT0, zero issues; git diff --check EXIT0. IA10 remains open despite green characterization/gates. No live provider call or publication; full goal remains active.

Implemented actual Messages SSE transport in adapter/anthropic/stream.go, with no retry/fallback and deferred Close. Reuses the checked lifecycle/tool assembler; added ProfileStreamerAdapter optional port so opaque state retains full effective destination/model family. Removed obsolete ParseStreamChunk stub/test; updated provider docs. Permanent local SDK/SSE fixtures cover interleaved tool IDs, text, late presence-aware usage, signed reasoning/refusal, protocol faults, retained partial/break, body closure/no replay and a runnable offline example. Targeted adapter/Anthropic race suites pass at the checkpoint; current lint and final source race verification are recorded below when terminal. This is parent evidence only; remaining conformance and expanded implementation/final paired audits are pending.

## Owner reply selects all five directions

Anthropic checkpoint terminal verification: `go test -race ./adapter/anthropic ./adapter -count=1` EXIT0 after final source changes (1.640s/1.780s package summaries). `golangci-lint run --allow-serial-runners ./...` in each of adapter and adapter/anthropic EXIT0, zero issues; git diff --check EXIT0. Final stream.go SHA256 `b58d6d0b0df36f4053c236e5de25d13fc87135c9588e50b1372d6db08c506f03`; stream_test.go `b19b92572c5c86d1ca85f7789e2725a8c9b39b225847c8f10fce3dad6f0c8659`. No full expanded-scope acceptance or final paired code-audit claim.

Owner reply received on 6 October: https://github.com/skosovsky/prompty/issues/4#issuecomment-6008749968. All five formerly conditional directions are required now. task37, scope, requirement matrix and closeout are reconciled with explicit provider-specific contracts/acceptance; optional means consumer opt-in, not deferral. Reference consumers suffice without production credentials. No stub/MVP acceptance or legacy bridge.

Code inspection confirms existing reusable profile/stream/wire/schema/state/usage/capture infrastructure and OpenAI native transport. Missing: three additional native transports, dedicated Responses adapter, three batch transports, Gemini resource lifecycle/handle and explicit Anthropic synthetic-output profile. Expanded implementation remains pending. Historical percentages below are not current completeness. No provider call, release, new GitHub comment or closure performed.

Both independent reply-reconciliation reviewers rechecked the same final task snapshot: 38/38 grouped reply obligations covered by specification (100%); no remaining confirmed spec contradiction in inspected scope. See owner-reply-audit.md. This is not implementation completeness or acceptance; new transports/lifecycle work remains pending.

## Historical checkpoint: IA07 independently corrected; test-only baseline verification

Both final-source checkpoint9 reviewers completed against the identical frozen snapshot; see audit-checkpoint9.md. Current full make lint/test terminal EXIT0 (unique-baseline-final-lint/tests logs), independent repeated race tests and matched unique-target benchmark pass. IA07/08/09 are independently corrected; no open confirmed finding in inspected scope. Source whole-row completeness remains143/160 (89.38%), plus17 pending triggers. Remaining goal readiness depends on five human conditional decisions and approved continuation dispositions, not unauthorized publication. Async question asked again; no answer recorded. Issue4 reread OPEN/unchanged/no comments. Subsequent edits are evidence-document reconciliation only; no production/test mutation after this paired snapshot, release or external action.

Both checkpoint8 reviewers confirmed IA07/S09-11 complete and no open confirmed findings in inspected scope; see audit-checkpoint8.md. Source whole-row completeness143/160 (89.38%), plus17 pending-trigger rows. Five human conditional demand decisions remain unanswered. Concrete closeout instructions are prepared; future external delivery is not authorized or claimed.

After paired reports, only benchmark/test source changed: added exact valid unique-target baseline and a regression proving that valid/rejected inputs differ in precisely one reference. Runtime is unchanged. Current full gates and paired verification are being repeated for this final test-only snapshot. Historical statuses below refer to their named snapshots, not current open IA07 claims.

## Checkpoint history: reference diagnostics and independent findings

Current parent IA07 correction removes the delayed sentinel and repeated whole-root probe queue. A private compiler-only vocabulary callback observes builtin-resolved edges and aborts before queued resource processing; separate stable all-neutralized compilation checks physical targets using engine resolution. Only actual graph edges enter a deduplicated discovery worklist; valid targets expose extension/transitive edges without probing annotation data. Fresh final graph proves original-root reachability. No custom URI resolver, validation semantics, successful temporary probe cache or arbitrary schema/ref-count cap.

Both-dialect invalid→valid→invalid observation tests, 1,000-edge local/remote/mixed/annotation cases, explicit resources with true/false and transitive missing targets pass targeted race. Original standalone 1,000-missing-ref reproduction is approximately61ms, versus56.81s before IA07 and3.44s at checkpoint7. Benchmarks compare local/mixed/annotation/unique-Boolean patterns up to10,000 with valid-engine baseline; intrinsic engine cost is not claimed constant or globally bounded. Annotation-heavy first prototype still showed9GB diagnostic allocations; actual-edge worklist reduced that case to~472MB (includes full metadata graphs). Current source needs full gates and both independent audits; IA07/S09-11 are not completed by parent evidence.

Latest completed independent evidence: checkpoint7, both reviewers confirm IA09 corrected. Whole-row completeness is 141/160 required (88.13%) plus seventeen pending triggers; final applicable denominator remains unresolved. IA07 CPU amplification and S09-11 stay partial. Current full make lint/test both terminal EXIT0 across all modules/race/generator fixtures in inconclusive-lint/tests logs; independent repeated race fixtures pass. No new confirmed finding in the IA09 correction. This is not final acceptance or publication; production freeze ends after both reports, so remaining CPU work can continue.

Both checkpoint6 reviewers completed; see audit-checkpoint6.md. Historical completeness 140/159 (88.05%), restoring IA08 and S12-03. Correctness found distinct IA09: legal non-schema examples/enum values abort all origins. Required ledger now160 rows plus17 pending. Parent correction restores inconclusive candidates unchanged rather than aborting other results; both dialect regressions pass targeted race checks. An invalid referenced extension regression separately retains rejection rather than claiming a validator. IA07 remains open. Full gates and both reruns must verify the subsequent snapshot; historical percentages are not final acceptance.

Parent correction checkpoint: distinct original reference strings now receive injective diagnostic false-resource identities; enum/const/default/examples regressions pass for both dialects. Diagnostic resources are registered as parsed private copies once. A shared compiler's delayed unsupplied sentinel forces every edge probe to fail, avoiding cached temporary successful graphs; permanent invalid → valid → invalid fixture proves re-reading. A fresh compiler builds the final root-reachable graph. Two nested sentinel levels are necessary to observe target lookup, not merely reference enqueue. Original source/cause and zero network remain unchanged.

Fresh targeted race tests pass, including eight local/remote/mixed/annotation scale cases. Standalone original 1,000-ref reproduction improved from 56.81s to 3.44s; **IA07 remains open** because residual superlinear whole-root work is not resolved. IA08 has parent regression evidence only. New findings increase required ledger rows to 159; checkpoint5's 138/157 remains historical, not a current final percentage. Full current gates and both final reruns remain required.

Both fresh reviewers completed; see audit-checkpoint5.md. Completeness is 138/157 required whole rows (87.90%), plus seventeen pending triggers. S09-11 and S12-03 lose parent completion credit. Correctness found diagnostic CPU amplification (IA07) and draft7 enum-placeholder collapse losing reference origin (IA08). Earlier all-module gates remain green but are not correctness acceptance. Both findings require correction, full gates and fresh independent reviews; unanswered human conditional decisions are unchanged. IA04–06 were independently confirmed. No external delivery occurred.

The initial independent audits finished; see `audit-initial.md`. Their historical completeness result is 133/151 required whole rows (88.08%), with seventeen additional pending-trigger rows and unresolved mixed conditional scope. Correctness confirmed IA-01–03. Parent implementation resumed to address them; those verdicts do not certify changed code. Both same agents must rerun against the actual final state.

Both checkpoint reruns finished; see `audit-checkpoint2.md`. Correctness confirmed IA-01–03 in bounded scope but found IA-04/P1 ingress signed-number loss. Completeness independently found a flaky registry fixture and two stale capture paragraphs, revoking their affected completion credit. The production freeze ended after both reports. Historical initial/checkpoint percentages do not certify the subsequently changed code or unresolved conditional scope.

## Previous parent checkpoint: engine-verified fragment/anchor origins

The missing fragment/anchor gap now has a diagnostic-only engine probe after original compilation rejection. Generic isolated JSON copies enumerate candidate fields, not assumed schema nodes. Other candidates are temporarily neutralized to a local false placeholder while the existing engine verifies each direct edge. Valid references are restored, and only root-reachable failed edges in the final engine graph produce physical source positions. Engine owns IDs/URI/dialect semantics; annotation/default/example and unreferenced definitions are excluded by reachability, including draft7 ref-sibling rules. No diagnostic schema becomes a validator or reaches a mapper/provider; original cause and input/resources are retained. Fragment URLs are encoded through net/url so hashes, spaces and newlines in property keys are not misinterpreted.

Twenty-two source/resource cases across both dialects failed before the implementation and now pass: local/remote fragments/anchors, transitive explicit resources, scoped IDs, referenced extension schema, escaped paths and mixed targets. DynamicRef/draft semantics and repeated public-position determinism are additional fixtures. Both output/tool HTTP fixtures now reject transitive fragments/anchors with zero HTTP/mapper calls. Unknown-position assertions remain for genuinely malformed JSON instead of wrongly treating missing fragments as unknowable. Parent targeted race and lint pass; fresh full make lint/test after the final source/test changes exited zero across all modules and generator fixtures including race checks: `/private/tmp/prompty-task37-fragment-origins-lint.log`, `/private/tmp/prompty-task37-fragment-origins-tests.log`. Both handles are terminal and git diff --check is clean.

This is parent implementation evidence, not independent acceptance. Both checkpoint4 auditors terminated with usage-limit errors and issued no verdict; no audit is running or being claimed as a verified wait. The latest independent report is historical checkpoint3. Full final audits must recheck ingress privacy/panic changes and this later diagnostic probe when available. Conditional decisions remain unanswered; no release, commit, external comment or issue closure occurred.

## Previous work: IA-05/06 response-error privacy and ingress conformance

Checkpoint4 reruns were dispatched on manifest ae68e5b14ee53cd212e4a58c02c3e59acfc35a2b1476f3da0d2b2a40c3e6d63a and task SHA-256 dff830d5b5affe2b3df2b68e6acc5f856e497a63294c827017924b658e1b434c. Both agents terminated with a usage-limit error before issuing new verdicts. They are not running and this is not a clean/complete audit or verified wait. Historical checkpoint3 remains the latest independent report. The runtime freeze is ended so independent mandatory schema-origin work can continue; both agents must ultimately recheck the later final snapshot when available.

Both checkpoint3 agents finished on the frozen manifest recorded in audit-checkpoint3.md. Historical completeness is 135/155 required whole rows (87.10%), plus seventeen pending triggers; this is not the current denominator after adding findings, not final applicable acceptance. IA01–03 and the original IA04 numeric reproduction passed independently. Correctness found IA05/P1 malformed HTTP200 signed body leaked raw private response/signature in SDK error; completeness confirmed the same contract mismatch and absent negative association/auth/timeout/cancel fixtures. Source rows with stale partial placeholders were reconciled to their independently confirmed bounded completions without excluding conditional scope.

Spec first now defines complete observed HTTP2xx conversion failure privacy independently of signature parsing. Permanent actual SDK sync/buffered tests failed before the fix, reproducing the body leak. An additional malformed candidates shape triggered SDK converter panic (IA06/P1); the same regression retains that case. After the fix all sixteen below/above-bound malformed/overflow/shape cases pass with static ErrProviderState and one dispatch. Recovery is limited to SDK conversion after fully observed successful response; caller transport panic before that boundary is rethrown, not hidden. Cancellation identity is safely separate; body-bearing SDK causes are not retained.

Permanent caller auth/header immutability, timeout/cancel during body read with Close/no-replay, negative original-body/SDK text/signature/variant association and safe error-chain fixtures pass targeted race tests. Existing original-ingress/precision/read/redirect/concurrency assertions remain. Migration's obsolete S04-04 question is removed and response-error behavior documented. Initial all-module lint found named-return style in the isolated SDK wrapper; refactored without suppression. Full final make lint/test after that last source edit exited zero across every module and generator fixtures, including race checks: `/private/tmp/prompty-task37-response-error-final-lint.log` and `/private/tmp/prompty-task37-response-error-final-tests.log`. Both command handles are terminal; git diff --check clean. Both auditors must recheck this changed state after terminal gate results; IA05/06 parent evidence is not independent approval. Missing schema fragments/anchors and conditional decisions remain open. No release, commit or external issue action occurred.

## Previous work: checkpoint findings and engine-derived missing-resource origins

IA-04 implementation now observes original response bytes through an isolated snapshot of public SDK configuration and a copied HTTP client. It keeps caller transport/auth/redirect/timeout policy and explicit endpoint binding; no SDK-private inspection, mutation of injected client, queue/persistence/retry or transport-capture claim. Invocation-local source memory has a positive configurable bound (default 4 MiB). Parse reconstructs each signed part from original Number-preserving JSON and verifies its association with the SDK-known part; raw signed float DTO without source fails closed. Exact original numbers survive canonical/envelope/both wires, while the separate outbound gate rejects unrepresentable next requests without a second call. SDK signed-number overflow errors (also when observation overflows) are sanitized to typed state failure; body-read/host-policy errors retain their causes.

The original independent IA-04 reproduction now prints original 9007199254740993 in canonical Args/envelope, and second checked call ErrProviderState with calls=1. Permanent eight actual SDK sync/buffered ingress-to-continuation cases retain exact/unsupported values through both wires. A constructed semantic DTO fixture now uses caller-owned Number instead of unverified SDK float; all its ordering/state assertions remain, and a new raw-float negative proves the clear-break requirement. Bounds, ambiguous/lossy signed body, safe overflow, twenty shared-client invocations, read-failure cleanup and caller redirect denial are permanent fixtures. Initial lint findings were formatting, constant naming, modernization and test idioms only; corrected without suppressions. Full all-module gates after the final ingress source/tests exited zero: `/private/tmp/prompty-task37-ingress-final5-lint.log` and `/private/tmp/prompty-task37-ingress-final5-tests.log`, including race checks and generator fixtures. Both shell handles are terminal and git diff --check is clean. These are parent checkpoint gates, not independent approval. Both auditors must recheck this substantial ingress boundary plus reference-origin/fixture/doc changes.

The registry fixture's select/default/close notifications were non-atomic for concurrent compose probes. Replaced started/canceled notifications in both adjacent cancellation fixtures with sync.Once; retained all cancellation/isolation/cache assertions. Targeted race repetition of both fixtures passed one hundred runs. Full make lint/test then exited zero across all modules and generator fixtures: `/private/tmp/prompty-task37-checkpoint-fixture-final-lint.log`, `/private/tmp/prompty-task37-checkpoint-fixture-final-tests.log`. Capture completion/migration paragraphs now distinguish normal finalization, producer-only cancellation on consumer break, acknowledged canceled closeout and observer cancellation on Close/caller cancel. These changes require independent recheck, not automatic restoration from historical gates.

For S09-11, read the engine's public compiled Schema metadata. After a real LoadURLError rejection, an isolated diagnostic-only compiler now supplies false-schema placeholders for unavailable resources and exposes verified ref edges/source locations. That compiled graph is never returned as a validator or used by mapper/provider. Original error/cause and no-network/explicit-resource rules remain intact; no reference resolver, error-string parser or guessed target-as-origin pointer was added. Permanent fixtures initially failed for all missing-resource origins, then passed for root/repeated/escaped/array/relative-scoped/transitive-resource/multiple-target origins in both dialects. HTTP fixtures now cover output and tool roles and assert exact origins, zero HTTP and zero mapper on failure. The old unknown-position missing-root-ref case now has a known /$ref origin; the unknown-position assertion remains for a genuinely unresolved fragment and malformed JSON instead of being removed. Missing fragments/anchors still prevent complete diagnostic graph compilation and remain unknown positions; S09-11 stays partial. Fresh full make lint/test exited zero across every module and generator fixtures after the last production/test edit: `/private/tmp/prompty-task37-reference-origins-lint.log`, `/private/tmp/prompty-task37-reference-origins-tests.log`. All shell handles are terminal; git diff --check clean. Both final independent audits remain required after remaining fixes.

The historical IA-04/P1 evidence and source requirement remain preserved; its new parent implementation assessment is not final independent approval. Broad signed-input fidelity and matrix statuses must be reconciled from current source/tests and rerun audits, not prior DTO-only claims. Missing fragment/anchor origin and conditional dispositions remain open. No final percentage/clean audit, release, commit, external message or issue closure is claimed.

## Latest checkpoint: IA-01 consumer closeout and IA-02 effective signed-block cache policy

IA-01 permanent root/checked native-buffered fixtures reproduced skipped capture closeout and lost frame errors on consumer break. Source cancellation now precedes one capture closeout without extra yield or whole-document finalization; errors are retained with cancellation and the report is stored before terminal status freezes. Running Close requests cancellation without prematurely freezing the status; created Close still cancels immediately without dispatch. Full all-module lint/race tests exited zero after the last IA-01 code edit: `/private/tmp/prompty-task37-consumer-closeout-final2-lint.log`, `/private/tmp/prompty-task37-consumer-closeout-final2-tests.log`.

IA-02 actual SDK/local HTTP regression initially failed: every signed-history block omitted the message default, yet the state gate rejected its required TTL before cache preparation. Spec first now defines effective signed-block policy: part override, otherwise inheritance. The state validator checks portable default shape even when unused, while only effective signed-block controls are forbidden. The successful SDK request preserves signed/redacted payload and source wire and reports the unused TTL default not_applicable without TTL capability knowledge. Direct negatives cover malformed shadowed defaults; a permanent checked fixture also proves malformed omission has zero actual HTTP calls. No signed SDK field/cache guarantee or legacy bridge was introduced.

Full all-module make lint/test exited zero after the final IA-02 checked malformed-default fixture: `/private/tmp/prompty-task37-signed-cache-effective-final-lint.log`, `/private/tmp/prompty-task37-signed-cache-effective-final-tests.log`. Both processes are terminal. The independent completeness audit also reconciled S04-04: no new arbitrary-Invoker proof-bearing interface is required by the original task. Current contracts/matrix remove that false gap without expanding provider logic into core.

IA-03 now has permanent actual SDK/local HTTP regressions. Before the fix, rounded integer/decimal continuations dispatched successfully, while an overflowing number reached an SDK encoding error containing the request body. Codec-only fidelity was insufficient. Spec first adds the SDK encoding boundary: state validation simulates the actual SDK float normalization and compares exact numeric values, failing with static ErrProviderState before Translate/dispatch when they change or overflow. Numeric spelling differences alone remain valid. The exact-number codec test remains intact; no core precision limit or caller-payload rewrite was introduced.

Twelve checked sync/buffered fixtures prove exact large integers, decimal/exponent spelling, nested caller keys and signature on real SDK wire; rounded integers/decimals and overflow have zero Translate/HTTP calls and safe errors. Source execution wire remains unchanged in all cases. Independent expected wire args are decoded with Number, not float64 equality. The entire Gemini module race suite passes. The first all-module lint caught only two shadowed error-variable names; renamed them without suppressions. After the final production/test edits, full make lint/test exited zero across every module and generator fixtures including race checks: `/private/tmp/prompty-task37-signed-sdk-encoding-final-lint.log`, `/private/tmp/prompty-task37-signed-sdk-encoding-final-tests.log`. Git diff --check is clean, all handles are terminal. IA-01–03 have parent implementation evidence, not final independent approval; both auditors must rerun. Compiler reference-origin diagnostics and conditional dispositions remain unresolved. No final percentage/clean audit, release, commit, external message or issue closure is claimed.

## Previous checkpoint: prepared diagnostics and current acceptance evidence

Re-read shared profile resolution, schema/cache preparation, Client/RuntimeClient execution and public raw-versus-checked documentation against remaining S04 rows. New permanent `TestPreparedSchemaCacheDiagnosticsStayProfileBoundAfterResultFailure` fixtures combine accepted required implicit-prefix policy with explicitly lowered output in sync/native/buffered. Original validation fails after one Translate/provider call, but effective profile/provenance/snapshot, original cache path/mechanism/disposition and schema destination/role/name/guarantee/digests/transform pointer remain attached to the known result/status. Before/after source execution wire is unchanged. Targeted race test passed.

S04-01/05/08 now record complete parent evidence rather than stale "schema/cache/final migration pending" placeholders. Source/fixture inspection also reconciles S12-02/04/05/06: permanent original AUD regressions, BYOT/core module imports without provider SDK/infra, local transport/attempt/zero-dispatch fixtures and exact current all-module gates. These assessments do not certify arbitrary custom Invokers, unresolved optional implementations or final audit acceptance. S04-04 and broader/conditional gaps remain explicit; no requirement was removed or denominator narrowed.

Full make lint and make test exited zero across all modules and generator fixtures including race checks after the last code/test edit. Logs: `/private/tmp/prompty-task37-prepared-binding-lint.log`, `/private/tmp/prompty-task37-prepared-binding-tests.log`; git diff --check is clean. All handles are terminal. Both final auditors, compiler reference-origin locations, wider documentation/profile reconciliation and explicit conditional dispositions remain outstanding. No release, commit, external message or issue closure occurred.

## Previous checkpoint: state/result evidence reconciliation

Re-read core envelopes/continuation gates, checked Prepare, four provider result/state parsers and permanent metadata/state fixtures against S06. Added eighteen all-mode checked negatives for foreign endpoint/model family, expiry, omitted required state, invalid association and absent actual state mapping despite supported knowledge. The tests consume lazy streams and assert failed status, typed errors, zero Translate/provider calls and unchanged source. This closes a real mode-coverage evidence gap without adding provider runtime or pretending SDK parsers imply native transport.

New `state-conformance.md` inventories core format/scope/association/history, Gemini signed codecs/media, actual Anthropic SDK/local HTTP thinking/citations, OpenAI refusal/native trailing usage, Ollama metadata, cross-provider outcome enforcement and explicit host credential/redaction policy. S06-02/04/06/13 now have complete parent evidence assessments instead of stale "remaining adapters/final audit pending" partial statuses; broad S06-01 and all genuinely unresolved requirements remain open. Complete parent rows are still subject to both final independent audits. No conditional requirement was removed from the denominator.

Fresh targeted race reconciliation exited zero across root/workspace packages and all four nested provider modules. After the final code/test edit, full make lint and make test exited zero across every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-state-reconciliation-final-tests.log`, `/private/tmp/prompty-task37-state-inventory-lint.log`, `/private/tmp/prompty-task37-state-inventory-tests.log`; git diff --check is clean. All handles are terminal. Explicit demand questions were presented again for extra native adapters, remote/background, independent offline batch, cache resources and separate forced-tool output; no response/disposition is presumed. No release, commit, GitHub comment or issue closure occurred.

## Previous checkpoint: helper composition and custom invocation boundary

Inspected core Invoker, structured/tool helpers and shared checked Prepare against the remaining S04-04 gap. Core has no knowledge of an arbitrary host implementation's SDK/profile/transport; fabricating a new universal provider gate there would either invent guarantees or duplicate provider preparation. Invoker godoc now states the fail-closed pre-dispatch obligation and requirement/mode preservation explicitly, while helper conformance distinguishes local schema/outcome/tool enforcement from checked transport guarantees. The wider proof-bearing arbitrary-invoker interpretation remains partial rather than being silently excluded.

Thirty permanent AAA fixtures put both NewClient and NewRuntimeClient behind a custom wrapper exposing only Invoker. Five helper paths (structured sync/native/buffered and both tool helpers) retain unknown extension-capability, unsupported output-MIME and unknown required-cache failures. All assert typed errors, zero Translate/dispatch and unchanged source execution wire. Targeted race test passed; full all-module lint/test gates are pending at this checkpoint. No additional SDK/core dependency, legacy fallback, release or external write was introduced. Both final independent audits and conditional decisions remain outstanding.

The initial lint rejected only a redundant explicit variable type in the new fixture; it was removed without relaxing checks. After the last code/test edit, full make lint and make test exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-helper-preflight-final-lint.log` and `/private/tmp/prompty-task37-helper-preflight-final-tests.log`. Both handles are terminal; no suite remains running. This is a checkpoint gate, not final acceptance or 100% completeness.

## Previous checkpoint: explicit omission on signed thinking

Inspected Anthropic state validation before modifying its signed-history boundary. Three actual SDK/local HTTP regressions reproduced that valid CacheOmit on message, reasoning parts or both was rejected as invalid provider state. Task/contract first clarified that omission is no marker, not a signed-payload edit. State validation now accepts only nil or portable-valid omission at these locations. Other cache policies remain rejected; no message-marker/per-thinking override relaxation or implicit-cache guarantee was added.

Permanent AAA tests parse actual SDK thinking/redacted blocks, persist/restore execution wire, dispatch through the checked client, compare ordered opaque payload against actual SDK request JSON, assert absence of cache_control and unchanged source wire. Ten direct-state-validator negative cases cover real policies and malformed omission at message/value-pointer part locations; checked negatives prove zero dispatch and distinguish generic cache-shape errors from signed-state errors. Migration, cache conformance and stable S10-SIGNED-OMISSION evidence reflect this additional requirement, not a smaller denominator.

Full make lint and make test exited zero across all modules and generator fixtures, including race checks, after the last code/test edit. Logs: `/private/tmp/prompty-task37-state-cache-omit-lint.log` and `/private/tmp/prompty-task37-state-cache-omit-tests.log`; git diff --check is clean. Wider requirements, compiler-reference origin diagnostics, conditional decisions and both independent final audits remain outstanding. No release, commit, external message or issue closure occurred.

## Previous checkpoint: fully shadowed message cache defaults

Read current cache preparation and native mapping before changing inheritance selection. A new regression failed in all checked modes: an unsupported required message default stopped preparation even though every block explicitly omitted it. Contracts/task now distinguish defaults from standalone SDK blocks. Read-only `MessageInheritsCachePolicy` evaluates canonical top-level inheritance (including value/pointer parts and enclosing tool-result blocks); checked cache preparation marks unused defaults not_applicable at original source paths and replaces them only in its clone. Their mapper/profile/control requirements and combination features are not selected. Empty content/no default are explicit query cases. Malformed portable shape is still rejected first.

Shared permanent fixtures cover all-mode shadowed unsupported mapping, an unused supported TTL creating no tools-combination failure, partial inheritance retaining required rejection and malformed shadowed policy still failing before Translate. Root tests ensure child overrides do not suppress an enclosing tool-result block and an enclosing override suppresses its message default. Actual Anthropic SDK/local HTTP fixture removes TTL profile knowledge and overrides every block: request succeeds without markers, diagnostics report the unused default as not_applicable, and before/after execution wire is unchanged. Earlier omission and partial-inheritance SDK fixtures remain intact. Matrix stable S10-SHADOWED-DEFAULT, migration and cache conformance account for these semantics without a resource/support claim.

After the final query/negative fixture changes, full `make lint` and `make test` both exited zero across all modules and generator fixtures including race checks. Logs: `/private/tmp/prompty-task37-cache-shadow-final-lint.log` and `/private/tmp/prompty-task37-cache-shadow-final-tests.log`; `git diff --check` is clean. Wider requirements, conditional decisions, original compiler-reference locations and both final independent audits remain outstanding. No release, commit, issue closure or external message occurred.

## Previous checkpoint: marker omission does not reactivate inheritance

Actual Anthropic SDK/local HTTP regression reproduced a wrong request: an unsupported advisory part TTL was pruned to nil, then Translate inherited the parent message's required 1h TTL for that same block. Contract/task were clarified before the fix: nil remains inherit, while portable `CacheOmit` explicitly suppresses a block marker. Checked removal of advisory controls/combination conflicts and accepted implicit-prefix intent now uses omission instead of nil. Original source diagnostics remain distinct; omission has no active cache feature and no provider/mode/controls, bypass/delete/retention/hit guarantee or resource semantics. No hidden runtime flag or compatibility bridge was introduced.

Root `cache_omission_test.go` exercises actual executable wire, clone, Normalize, nil-vs-omit distinction and rejection of extra remote guarantees in Go/serialized schema. Actual SDK/local HTTP fixtures cover unsupported advisory TTL, accepted implicit prefix, explicit omission and foreign advisory: the overridden block has no cache_control while a separate block inherits 1h normally. Before/after execution wire snapshots verify source isolation. Existing shared cache omission assertions now check the explicit portable policy rather than nil; no negative case or provider-wire assertion was removed. Migration, contract, conformance inventory, author closeout and stable S10-OMISSION ID account for the clear-break behavior.

The first lint gate reported only a typed/untyped constant grouping warning after the new godoc comment; CacheOmit was separated into a documented typed constant without relaxing checks. Wider effective-target/shadowed-default coverage still needs reconciliation; this fix does not certify every inheritance placement. Conditional resource scope, other requirements and both final audits remain outstanding. No release, commit, external message or issue closure occurred.

After the final source-isolation fixture edit, full `make lint` and `make test` both exited zero across every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-cache-omission-final2-lint.log` and `/private/tmp/prompty-task37-cache-omission-final2-tests.log`; `git diff --check` is clean. These are checkpoint gates, not final acceptance.

## Previous checkpoint: selected cache combinations and mapper isolation

Current cache source was inspected against PRM-007 and effective-profile combination rules. Individually accepted cache controls were absent from ForbiddenCombinations, allowing unsupported tools/output/reasoning/native/cache combinations to reach translation. Task/contracts first specified selected-only combination reconciliation: omit all participating advisory policies, then reevaluate remaining required intent; independent policies remain and required conflicts fail before Translate/dispatch with ErrCacheRequirement plus ErrUnsupportedCombination. Already omitted unknown/unsupported/foreign hints do not add active features. The implementation tracks accepted mechanism/control keys and stable paths before pruning implicit hints. It does not choose an advisory subset, route requests, retry or implement resources.

CacheControlMapper also received the same policy pointer later enforced/translated; it now receives an isolated policy and control bytes. A deliberate mutating mapper fixture cannot change provider/mode/controls in source or selected translation. Permanent generic port fixtures cover tools/output/reasoning combinations in every checked mode, native conflict, nested cache/cache combinations with both required/mixed/both advisory policies, preservation of an independent required marker, stable nested paths and zero translation/dispatch for remaining required conflicts. Source intent remains unchanged. These synthetic trusted-port tests are not actual provider support claims; existing Anthropic actual SDK/local HTTP marker/TTL/usage evidence remains distinct.

`cache-conformance.md` inventories controls, provider scope, selected combinations, isolation, actual SDK TTL/count/order, raw unmapped rejection and usage semantics. Request-policy S10 rows were reconciled against inspected implementation/tests; complete rows carry parent evidence assessments only. S10-01 remains partial for optional handles and S10-07 scope/log/resource reconciliation remains pending; all conditional resource rows remain pending-trigger. A stable S10-COMBINATIONS ID tracks the clarification without removing scope from the original requirements. Explicit-resource demand still needs a user decision and no resource lifecycle is advertised.

The first full lint gate identified only a complex nested fixture; assertions were extracted into a test helper with all cases retained, without policy/suppression changes. After final fixture edits, full `make lint` and `make test` both exited zero across every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-cache-combinations-final-lint.log` and `/private/tmp/prompty-task37-cache-combinations-final-tests.log`; `git diff --check` is clean. Wider requirements, conditional decisions and both final independent audits remain outstanding. No release, commit, issue closure or external message occurred.

## Previous checkpoint: actual forced local-tool selection

Code inspection found Gemini/Ollama implemented ForcedTool only as an injected system instruction. Task/contracts were clarified before changing behavior: native named local-tool selection requires a separate profile capability and actual adapter mapping, not a textual hint. `CapabilityForcedToolSelection` is inferred from ForcedTool; checked preparation verifies `ForcedToolAdapter` before Translate/dispatch and overrides cannot create that implementation. Port failure replaces the corresponding diagnostic check with a static unsupported result without mutating the profile. OpenAI/Anthropic expose their existing native named-choice mapping. Gemini now uses SDK ANY with a single allowed function name and leaves system instructions unchanged. Ollama rejects unavailable forced selection, including raw Translate, rather than injecting a prompt. Ordinary tools and helper batch scope remain independent; this does not implement conditional forced-tool structured output.

Shared trusted-port fixtures test unknown/unsupported knowledge and missing mapping in sync/buffered/native modes, plus declining/implemented mappings in each mode. Gemini actual SDK/local HTTP fixtures prove native fields on the wire for sync/buffered, unchanged execution, tool-call outcome and one request; no live model claim. Ollama raw/checked adversarial-override regressions fail before dispatch. Existing OpenAI/Anthropic raw SDK named-choice assertions remain green. `tool-selection-conformance.md`, provider READMEs, migration and author-closeout draft document these precise evidence boundaries. New stable requirement T37-S04-FORCED-SELECTION tracks the implementation-time clarification without hiding it from the denominator.

The first lint gate found embedded-field spacing and Prepare complexity, then one remaining complexity point. Mapping checks were extracted into focused helpers and fixture spacing fixed without suppressions or policy changes. After final code/fixture edits, full `make lint` and `make test` both exited zero for every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-forced-tool-final2-lint.log` and `/private/tmp/prompty-task37-forced-tool-final2-tests.log`; `git diff --check` is clean. No production forcing-instruction branch remains. Wider requirements, conditional decisions and both final audits remain outstanding; no release, commit, external comment or issue closure occurred.

## Previous checkpoint: authoritative schema compilation positions

The engine's actual exported error contract was inspected before implementation. Metaschema failures expose schema input InstanceLocation paths; reference loading failures expose target URLs without source origin. `SchemaCompilationError` now preserves ErrSchemaUnsupported plus the original cause, projecting only authoritative positions into document/RFC 6901 pointer pairs. Source document identity is distinct from explicit resource identity, a known root differs from unknown location, multiple positions are sorted/deduplicated, and resource registration is deterministic. The wrapper string is static; arbitrary underlying causes and caller-owned locations still require host logging policy. No source position is guessed from text and no schema/ref engine is reimplemented.

Permanent actual-engine fixtures cover escaped keys, array indices, known root, referenced extension subtree (engine subschema base plus relative instance location), explicit resource, both supported dialects, sorted multiple positions, declared dialect mismatch and preserved typed JSON decoder cause. Output/tool roles fail before mapper without source mutation. Repeated malformed-resource registration picks the sorted-first resource. Missing references and malformed JSON remain explicitly unknown positions; S09-11 remains partial for missing/transitive reference origin, not narrowed to mapper-only failures. Contract, migration, conformance inventory and requirement evidence are updated. Final independent audits remain outstanding.

After the final fixture changes, full `make lint` and `make test` exited zero across every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-compilation-locations-final2-lint.log` and `/private/tmp/prompty-task37-compilation-locations-final2-tests.log`. `git diff --check` is clean. Wider required work and conditional decisions remain active; these gates are not final acceptance. No release, commit, issue closure or external message occurred.

## Previous checkpoint: remaining schema applicators, refs and evidence reconciliation

Current OpenAI traversal still skipped schema-valued additionalProperties/unevaluatedProperties/unevaluatedItems. Contract was extended before implementation; these applicators now receive the same deterministic strict rejection/permitted-lowering checks with source pointers. Six supported-dialect source cases exercise strict plus explicit lowering, unchanged source, original versus translated acceptance. Annotation/default/examples/content data and property-dependency names are not treated as generation-target schemas. This is mapper behavior, not a live claim that each applicator is accepted by an endpoint. The official structured-output reference is linked separately from fixture evidence.

New OpenAI local-ref fixture combines a definition with an exact numeric enum and optional non-null property: missing is accepted by the original, null only by translated required-nullable, original enum rejects the rounded integer, and actual SDK request serialization retains reference/definition/exact number. A new available-HTTP-resource fixture proves the explicit-only validator loader never fetches missing/supplied/transitively missing references: supplied resources validate locally, missing resources fail before mapper, all HTTP request counts stay zero. Parallel subtest lifetime uses parent Cleanup so the server stays available for every regression. No global HTTP client or hidden resource upload was introduced. Tool lowering fixtures additionally prove Go JSON decode succeeds before original minimum/missing/null rejection.

Remaining stale S09 rows were reconciled against current compiler, immutable/SDK fixtures, no production magic-name branch, whole-document min/max constraints and actual native nullable finalization with trailing usage. Those rows now have complete parent evidence assessments, not final independent verdicts. S09-11 remains partial for original compiler-error portable source pointers. S09-06 remains partial for the explicit conditional forced-tool-output mode; its demand question was presented to the user, and closeout tracks it without silently deferring or advertising a mode. Wider S12 profile acceptance remains separate from these mapper fixtures.

Full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-schema-reconciliation-lint.log` and `/private/tmp/prompty-task37-schema-reconciliation-tests.log`; `git diff --check` is clean. Other required work, conditional dispositions and both final independent auditors remain outstanding. No release, external message or issue closure occurred.

## Previous checkpoint: original tool lowering and distinct document errors

New shared checked fixtures close a missing lowering-to-handler evidence path. A synthetic tool mapper explicitly drops minimum/required with a ledger; twelve sync/buffered/native cases retain original tool content/outcome/usage, exact source digest and destination/role/name association, unchanged source execution wire and one dispatch. Invalid minimum/missing/null values fail original validation; a valid positive control succeeds. Three typed-handler cases prove registry membership does not admit invalid arguments. This is universal port enforcement, not a claim that actual provider modules implement those synthetic transforms.

Source reconciliation also found final JSON decoding was categorized as schema mismatch and the one-shot structured helper converted invalid/truncated documents into ordinary repairable ValidationError. Contract-first changes introduce `ErrSchemaJSONDecode` for invalid/truncated/multiple documents with decoder causes retained; valid JSON constraint failure remains `ErrSchemaMismatch`, and provider outcomes stay separate. Checked sync/buffered/native preserve known result/diagnostics without replay; canonical capture uses safe parse classification. The structured helper returns decode failure directly, so its executor does not append domain feedback. Incremental parser syntax still uses ErrStructuredStream rather than an invented domain error.

Root and adapter decode fixtures prove these distinctions, one dispatch, failed stream status and known result retention. An executor fixture observes unchanged scratchpad across two explicitly initiated attempts. Five old tests expected invalid JSON to enter the repairable validation/feedback path; those expectations were replaced with direct decode errors, while the existing feedback test now exercises real domain validation and still verifies assistant/user feedback. No compatibility bridge or weakened assertions were added. `schema-conformance.md` inventories provider mapping/local fixture boundaries and separates synthetic port evidence from actual SDK and live endpoint claims. Migration/README/author-closeout draft explain the new error handling.

Final full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-schema-errors-final2-lint.log` and `/private/tmp/prompty-task37-schema-errors-final2-tests.log`; `git diff --check` is clean. S09-08/09/17/18 now have complete parent evidence assessments, not final independent acceptance. Wider schema-location/profile/cache/closeout reconciliation, conditional dispositions and both final auditors remain outstanding. No release, GitHub message or issue closure occurred.

## Previous checkpoint: explicit per-definition dialect and unavailable validation

Current checked schema preparation and structured/tool helpers always passed an empty dialect despite the low-level compiler's explicit dialect contract. Contract/wire specification was extended first: `SchemaDefinition.Dialect` and `ToolDefinition.Dialect` now persist independently through clone, execution wire and YAML, and reach checked compilation, structured sync/stream helpers and original tool-schema validation. Empty still selects the bundled default; declared/selected mismatch fails rather than silently switching engines. YAML nested output dialect rejects non-string input. SDK/profile mapping availability remains separate from validator support.

Permanent root fixtures cover selected-dialect original constraints, output/tool clone and executable-wire round-trip, isolated document bytes, and sync/stream/tool zero invocation on mismatch. Checked fixtures use different output/tool dialects in one prepared request and prove matching report/definition selection. Eighteen negative checked cases cover both schema roles across sync/buffered/native for unsupported dialect, rejecting compiler and nil validator, with zero translation and provider calls. YAML retains both definitions' selections. Custom validators remain trusted runtime ports and must reject contracts they cannot enforce; schema assertions, format/content annotations and domain validation boundaries are documented rather than advertising universal engine completeness.

Full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-schema-dialect-final2-lint.log` and `/private/tmp/prompty-task37-schema-dialect-final-tests.log`. Initial lint found test complexity/formatting and the deliberately unavailable compiler fixture's literal nil/nil return; fixtures were factored and formatting aligned without lint suppressions or relaxed checks. S09-01/05 now have complete parent implementation evidence assessments, subject to both final independent audits. Wider reconciliation, conditional dispositions and both final auditors remain outstanding; no external writes/release occurred.

## Previous checkpoint: nested output-schema traversal

Current OpenAI mapper inspection found schema children in tuple-form `items`, `additionalItems` and schema-form `dependencies` were skipped. The contract was updated before implementation. Deterministic traversal now visits those locations and preserves escaped original pointers; property-dependency name arrays and annotation data are not recursively treated as schemas. Explicit lowering still owns object-closing rather than silently normalizing.

New `adapter/openai/schema_children_test.go` compiles actual supported-dialect schemas: strict fails at the nested pointer, permitted lowering reports exactly one transformation, source bytes remain unchanged, and a value accepted by the original is rejected by the translated closed-object schema. A separate fixture retains property-dependency enforcement without transforming annotation objects. Compile fixtures are not a live endpoint capability claim. Other schema-location/profile coverage remains explicitly partial under S09-02.

Source and existing byte-split separator/lifecycle fixtures were re-read: leading/repeated/trailing/missing comma cannot be hidden by an already yielded provisional item; original handle fails and producer is released. S09-19 now has a complete parent evidence assessment, subject to both final independent audits.

Full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-schema-children-lint.log` and `/private/tmp/prompty-task37-schema-children-tests.log`. Remaining requirements, conditional decisions and both final auditors are outstanding. No release, message publication or issue closure occurred.

## Previous checkpoint: SDK host policy and capture reconciliation

Actual SDK host-redaction integration was missing from prior core policy evidence. New `capture_redaction_test.go` covers sync/buffered/native: Authorization exists on HTTP request, while every sink receives only an explicit two-byte host allowlist projection. Projector mutates its copied payload/usage adversarially; actual HTTP prompt, provider result/output/counts and source execution remain unchanged. Approved records have stable sequence/invocation, no endpoint or response/request ID, and redacted/incomplete markers. Completed execution has a closed but incomplete capture report with no invented capture failures. This is delivered payload boundedness, not a bound on SDK raw allocations or automatic secret detection.

Required unsupported capture fixture now covers both transport request and response stages with zero provider calls. Current CaptureSession/adapter hooks, metadata isolation, schema digest association, optional capture, cancellation, native ownership and report fixtures were re-read against remaining PRM-005 requirements. `capture-conformance.md` now references actual SDK host policy as well as core ports. Remaining S08 rows now have complete parent implementation evidence assessments: all PRM-005 rows have implementation evidence, not final independent acceptance or a live/all-profile guarantee. Broader profile-specific conformance is still separately tracked.

Full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-capture-policy-lint.log` and `/private/tmp/prompty-task37-capture-policy-tests.log`; `git diff --check` is clean. Wider schema/profile/cache/closeout reconciliation, conditional dispositions and both final independent auditors remain mandatory. No external write or release occurred.

## Previous checkpoint: capture durable acknowledgements and unknown outcome

Current PRM-005 source, CaptureSession admission/ack logic and actual OpenAI SDK sync/native/buffered capture fixtures were inspected. No production ack change was needed, but integration evidence was missing. New `capture_ack_test.go` exercises all three execution modes: undeclared durable capability makes zero sink writes/provider calls; false ack before dispatch makes zero calls; false post-dispatch ack retains known parsed result/accepted frame with one call; confirmed acknowledgements produce complete reports. Existing core fixtures also reject unadvertised durable ack claims.

New `capture_unknown_outcome_test.go` combines actual SDK HTTP 503 (client configured for retries) and failed terminal sink in all three modes. Per-call retries=0 produces exactly one request. Sync stays nil response; buffered/native failed status has no invented provider outcome/content. Both SDK and typed capture error remain available, CaptureError.AfterDispatch is true, there is exactly one terminal capture, and its static payload/category contains no SDK error body or auth value. This does not hide the SDK error from the caller; applications must still apply their logging policy to arbitrary errors. No automatic retry, not-executed claim or fake physical attempt ID is introduced.

`capture-conformance.md` inventories optional ports, stage/accuracy coverage, native ownership, observed metadata, known/unknown failure, durable ack, policy, mutation, bounded synchronous backpressure and crash limitations. Migration explains ack/unknown-outcome handling. Matrix S08-02/04/05/06/08/10/11/12/13 now has complete parent evidence assessments; other capture/profile rows and both final independent auditors remain required.

Full `make lint` and `make test` exited zero after the last test style correction, across all modules and generator fixtures including race checks. Logs: `/private/tmp/prompty-task37-capture-ack-outcome-final-lint.log` and `/private/tmp/prompty-task37-capture-ack-outcome-final-tests.log`; `git diff --check` is clean. Wider requirements and unresolved conditional scope remain active; no release or external write occurred.

## Previous checkpoint: descriptor/diagnostics isolation and host redaction conformance

Current clone implementations were inspected across RuntimeDescriptor/CapabilityProfile and ExecutionDiagnostics/CaptureReport. New permanent fixtures mutate all mutable descriptor collections (profiles, capability map, nested forbidden combinations, input/output MIME, schema bytes and reasoning settings) and diagnostics collections (checks/cache/schema reports/nested transformations/capture pointer and coverage/observed/failures) in both returned and source directions. Original and sibling views remain unchanged, including fresh RuntimeClient descriptor retrieval. No clone implementation or legacy bridge was needed.

`host_redaction_conformance_test.go` proves explicit application allowlist composition with logging and capture, not a universal secret detector: approved logging omits caller text/annotation/provider-settings credentials and raw state while separately approved persistence remains unchanged. Default state omission is explicitly shown not to sanitize arbitrary provider settings. Capture allowlist covers execution/translated/provider/terminal stages, removes credential/error/output/state payload and request/response identities, retains approved usage/outcome, and closes an incomplete/redacted report without fabricated byte replay. Payload/source metadata remain unchanged. `host-redaction.md`, migration and source task clarify intermediate projection versus host-approved output, SDK credential ownership, arbitrary string boundaries and explicit persistence consent.

Full `make lint` and `make test` both exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-isolation-redaction-lint.log` and `/private/tmp/prompty-task37-isolation-redaction-tests.log`; `git diff --check` is clean. S04-07 and S06-14 now have complete parent evidence assessments under their explicit host-policy boundary, not final independent acceptance. Wider capture/profile/closeout rows, conditional decisions and both final auditors remain required. No release or external write occurred.

## Previous checkpoint: credential-free endpoint identity

Current source reconciliation found populated endpoint identities were accepted on nonblank checks alone, allowing URL userinfo/query credentials into checked diagnostics and persisted scopes. Task/contracts first specified a stable credential-free identity, with malformed URI/userinfo/query/empty-query-marker/fragment rejection instead of silent sanitization or destination guessing. `ValidateEndpointIdentity` now gates capability profiles, descriptor resolution before diagnostics assignment, state scope associations, schema compilation before mapper invocation, and response diagnostics (including nested schema reports) on marshal/restore. RuntimeClient descriptor retrieval also rejects unsafe top-level/nested endpoints instead of returning their values. Executable wire identity schema rejects credential URL components; Go semantic checks additionally enforce URI parse validity. Core retains host-owned opaque references and safe paths, not a mandatory HTTP endpoint type or universal secrets scanner.

Permanent AAA fixtures cover userinfo with/without scheme, query/empty query, fragment/empty fragment, malformed escaped URI, safe opaque/URN/path references, execution override persistence, provider-state wire, response/nested-schema diagnostics wire and schema-level rejection, zero mapper calls, and sync/native/buffered zero Translate/dispatch. Direct Preflight returns empty diagnostics for invalid descriptor endpoint; RuntimeClient returns empty metadata, with static errors rather than echoed values. SDK credentials and request-specific URL params stay in host transport; migration requires explicit rebuilding of old scopes and prohibits silent auth stripping. Arbitrary path/prompt/payload credentials still require host policy, so S06-14 is not declared complete from this structural check.

Final full `make lint` and `make test` both exited zero after the last runtime getter fixture, across all modules and generator fixtures including race checks. Logs: `/private/tmp/prompty-task37-endpoint-identity-final2-lint.log` and `/private/tmp/prompty-task37-endpoint-identity-final2-tests.log`; `git diff --check` is clean. New source clarification is explicitly tracked as T37-S01-ENDPOINT rather than lost from requirement accounting. S04-07 still awaits full descriptor alias/redaction boundary reconciliation; S06-14 still requires host policy conformance. Wider PRM reconciliation, conditional decisions and both final audits remain outstanding.

## Previous checkpoint: provider outcome conformance and PRM-003 reconciliation

Issue #4 was re-read through GitHub read-only checks: still open, updatedAt unchanged at 2026-10-03T16:14:48Z. Source requirement scope was not narrowed. Current reasoning implementation/tests were inspected: effective-profile ModelOptionsSchema and explicit override gate settings before translation, mode/budget/visibility/combinations fail closed, actual Anthropic SDK preserves adaptive/display/effort, Gemini budget and level remain separate, and OpenAI does not use model-prefix option removal. Current positive signed round-trip fixtures prove alternating parts/citation/partial usage through response wire, history/Normalize, execution wire and next native translation/request.

New cross-provider outcome conformance uses actual SDK response parsers, then the universal structured helper: OpenAI/Anthropic/Gemini refusal and incomplete, plus Ollama incomplete, all with valid JSON and known usage. Each returns a typed OutcomeError, no structured value or repairable ValidationError, one invocation and unchanged response wire. This does not fabricate a provider schema/native capability; Ollama has no mapped refusal discriminator. Actual OpenAI native SDK schema fixtures now include length/incomplete alongside refusal: valid JSON, one failed terminal, retained provider finish/outcome/trailing usage and one dispatch. Migration explicitly separates provider outcome failure from repairable JSON failure.

Full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-outcome-conformance-lint.log` and `/private/tmp/prompty-task37-outcome-conformance-tests.log`; `git diff --check` is clean. Matrix S06-10/11/15/17 now has complete parent implementation evidence assessments; these are not final independent audit verdicts. Other PRM rows, conditional scope dispositions and both final auditors remain required. No external write, release or issue closure was performed.

## Previous checkpoint: OpenAI counter range and complete usage mapping inventory

The remaining OpenAI mapper converted SDK int64 counters directly to canonical int. It now validates every primary/cache/reasoning value before conversion, retaining observed representable siblings and returning ErrUsage for invalid fields. Sync parsing returns the known response alongside that error. Actual SDK native streaming accepts known content/valid counters in the current frame, then fails and closes transport without consuming later counters or replaying. Capture metadata exposes only representable observations; raw SDK JSON remains distinct, with canonical terminal reporting parse failure. Contract design was updated before implementation; migration documents handling response plus error.

`usage_range_test.go` checks negative/zero/platform maximum and a value that wraps to positive on 32-bit platforms for every mapped field; it adapts expected results to runtime width. The current host is arm64 and no executed 32-bit run is claimed. Actual SDK-native fixtures on both declared bindings prove single dispatch/body close, failed terminal, retained current-frame content/completion and no later total. A sync SDK-JSON fixture proves known-zero total and representable completion remain known while invalid prompt is omitted, including capture metadata. Gemini has a full primary/cache/reasoning composition fixture proving the provider total remains authoritative.

`usage-mappings.md` inventories all four actual mappings, presence sources, total composition and limitations, cross-referenced to core/adapter/provider evidence. S06-03 and S06-16 now have complete parent implementation evidence assessments, subject to both final independent audits. Full `make lint` and `make test` exited zero across all modules and generator fixtures, including race checks; logs: `/private/tmp/prompty-task37-openai-usage-range-final-lint.log` and `/private/tmp/prompty-task37-openai-usage-range-final-tests.log`. Initial lint flagged integer test-name formatting only; it was fixed without suppression. `git diff --check` is clean. Remaining requirements, conditional scope dispositions and final audits are still mandatory; no release/publication/issue closure occurred.

## Previous checkpoint: checked provider-derived usage arithmetic

Source reconciliation found that Anthropic added three int64 input counters before narrowing, allowing positive wrap to be advertised as a valid known prompt/total. Ollama also added canonical prompt/completion counters without a range check. Source task/contracts first specified the fail-closed arithmetic and known-result preservation contract. `adapter.SumUsageCounters` now checks nonnegative/platform range before arithmetic; Anthropic retains individually representable counters but leaves unrepresentable derived values unknown and returns ErrUsage. Ollama preserves primary counters/content alongside total overflow failure.

The generic checked parser-error path previously dropped any response returned alongside an error. It now preserves that known response, stamps effective-profile diagnostics and refreshes capture reports after sync closeout; buffered closeout remains owned by the original handle. Usage stream capture failures have the safe parse category rather than being misclassified as transport errors. No retry, SDK dependency in core, or provider-wide support claim was introduced.

AAA fixtures cover exact platform boundary, zero, negative and positive-wrap sums; actual Anthropic SDK JSON presence with positive-wrap/input/total failure; Ollama known response retention; generic sync/buffered parser partial result, one dispatch, and exactly one failed capture terminal with known provider outcome/usage and completed report. Targeted shared/Anthropic/Ollama tests pass. Initial full lint exposed unchanged execute cognitive-complexity limit; parse failure closeout was extracted without suppressions. Final full `make lint` and `make test` both exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-usage-overflow-final-lint.log` and `/private/tmp/prompty-task37-usage-overflow-final-tests.log`; `git diff --check` is clean. S06-16 remains partial: OpenAI platform narrowing and complete counter mapping inventory still need reconciliation. Final independent audits and conditional scope dispositions remain outstanding.

## Previous checkpoint: explicit profile-bound ID-less tool correlation

The source task/contracts first specified the missing ID-less admission rule. `CapabilityToolNameCorrelation` is explicit caller intent plus effective-profile knowledge, using the existing portable requirements/checks/wire representation. Checked Prepare additionally requires the optional actual ToolNameCorrelationAdapter port; an override cannot invent name-preserving translation. Gemini exposes its existing FunctionResponse name mapping, without claiming this for other modules or introducing SDK types into core.

The one-shot helper accepts ID-less calls only with nonblank checked destination/provenance/snapshot, exactly one supported tools and correlation check, matching effective model and valid supporting override, complete args and a unique name across the entire batch. An ID-bearing sibling of the same name is also ambiguous. Unknown/unconfirmed correlation fails with ErrToolScope before any handler; no synthetic provider IDs or agent loop are added. Custom invoker diagnostics remain a trusted host contract, not cryptographic attestation or IAM. All nonempty IDs retain the full-response uniqueness gate.

Permanent core fixtures cover positive unique-name batches/valid override and negative missing/unknown/unsupported/duplicate/foreign diagnostics, duplicate ID-less/mixed names and deltas via ExecuteWithToolValidation. Shared adapter fixtures prove supported knowledge/override cannot bypass an absent mapper port (zero Translate/Execute). A Gemini fixture uses actual Translate/Parse and checked client with intercepted SDK Execute: ExecuteWithTypedTools admits a unique ID-less call, subsequent SDK continuation preserves both call/result name without IDs; duplicate/no-intent batches invoke no handlers and unknown profile knowledge makes zero provider calls. An unknown tool scope regression was added too. The expanded diagnostics check hit the unchanged cognitive complexity bound; supported-check counting was extracted without suppression.

Full `make test` and `make lint` both exited zero on the final helper/fixtures across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-idless-final2-tests.log` and `/private/tmp/prompty-task37-idless-final2-lint.log`; `git diff --check` is clean. AUD-01's two rows now have complete parent evidence assessments, so the five original audit areas have implementation/regression evidence, not independent final acceptance. Wider PRM requirements, conditional dispositions and both final independent auditors remain outstanding. Migration and the unpublished author closeout draft include the new requirement. No release, commit, external comment or issue closure occurred.

## Previous checkpoint: original audit regression reconciliation and duplicate-ID gate

Current tool-helper review found that `validateResponseToolScope` checked repeated provider IDs only when Args was nonempty. Identity-only/delta full-response parts could therefore bypass the full-response uniqueness gate before later assembly. Every nonempty ID is now checked regardless of argument representation; permanent scope fixtures prove identity-only and split-delta repeated IDs fail with `ErrToolScope` before any handler. Existing allowlist/ForcedTool/original-schema/domain whole-batch gates remain intact. AUD-01 stays partial: explicit profile-bound correlation for calls without provider IDs is still missing and is not hidden by this fix.

Reviewed structured source and added `TestStructuredControlAndTerminalConformance`: completion, empty document, missing finish, truncated JSON and pre-canceled source cases verify provisional item counts, trailing usage, expected failed/canceled/completed status, single observation and zero source dispatch after pre-cancel. Existing consumer-break/source-unwind and byte-separated array separator fixtures cover cancellation, provisional items, typed causes and one lifecycle. Reviewed mediafetch's copied client, HTTPS-first redirect wrapper, host limits/denial, bounded body/probe and TLS fixtures for every-hop downgrade rejection, cancellation, concurrent policy isolation and exact/default/MaxInt64 limits.

AUD-02, AUD-04 and AUD-05 implementation/AAA rows now have complete parent evidence assessments, subject to both final independent audits. Both literal test commands from the source task also exited zero on current code: `go test ./...` and the four-provider `go test` command. Logs: `/private/tmp/prompty-task37-audit-reconciliation-final-root.log` and `/private/tmp/prompty-task37-audit-reconciliation-final-providers.log`. Full `make test` and `make lint` exited zero after the final structured fixture across every module and generator fixtures, including race checks; logs `/private/tmp/prompty-task37-audit-reconciliation-final-tests.log` and `/private/tmp/prompty-task37-audit-reconciliation-final-lint.log`. No final completeness/correctness verdict, release, commit, external comment or issue closure occurred. Conditional decisions, idless tool correlation, wider PRM reconciliation and both final auditors remain outstanding.

## Previous checkpoint: core state history/wire isolation and projection reconciliation

Current source review confirmed that AssistantMessage, execution constructors/history helpers, clone paths and wire marshal/restore deep-clone opaque payloads, annotations and optional expiry. Normalize checks both adjacent messages for state, annotations or an unavailable-continuation marker before merging. Explicit state projection is mandatory; logging omits opaque bytes and marks omitted required continuation unavailable, while preservation retains state without changing the source. Response/execution format gates and JSON Schema reject legacy/foreign formats rather than decoding them permissively. Structured validation scratchpads use an isolated AssistantMessage and retain its state/annotations.

New permanent AAA fixtures verify all six history ports (NewExecution/AddMessage/WithHistory/WithMessages/Clone/Normalize), mutation isolation for payload/annotation bytes and expiry, every association kind on either side of a Normalize boundary, pairwise expiry ownership across seven response/wire/history/runtime layers, and schema-decoded logging execution that cannot continue while independent explicit persistence still preserves the original state. The initial pairwise expiry test accidentally pointed its comparison baseline at the deliberately mutated source expiry; the fixture storage was separated from the immutable baseline. No production defect or workaround was needed.

Full `make test` and `make lint` exited zero on the final fixtures across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-state-isolation-tests.log` and `/private/tmp/prompty-task37-state-isolation-lint.log`; `git diff --check` is clean. S06-07/08/09/12 now have complete parent evidence assessments, not final audit approval. Arbitrary prompt/annotation credential filtering is not inferred from opaque-state omission; S06-14 and other outstanding requirements remain separate. Conditional decisions and both independent final audits still prevent overall acceptance. No release, commit, external comment or issue closure occurred.

## Previous checkpoint: exact Gemini persisted-codec reconstruction

`decodeGeminiState` previously relied on a JSON struct decoder, which accepts duplicate keys and case aliases while discarding some explicit null/zero fields. The source task and contract first specified that library-produced signed payloads must be unambiguous and reconstruct the same JSON keys/values. The adapter now rejects duplicate keys recursively (including nested argument objects/arrays), validates full JSON and required signature, decodes with exact JSON numbers, and compares source semantics against SDK re-encoding. Unknown fields, case aliases, discarded null/zero data and changed signature encoding fail with safe `ErrProviderState` before continuation dispatch. Whitespace/key order may differ, and arbitrary caller argument keys remain untouched. This is a local persisted codec gate, not a schema engine or promise to reconstruct upstream HTTP data already lost by an SDK.

Eighteen permanent AAA negative fixtures verify zero checked-client dispatch and unchanged saved payload for duplicate/escaped-duplicate keys, case aliases, unknown fields, null/zero discards, signature normalization, missing/null signatures and invalid document/root shape. The positive fixture preserves distinct caller keys, Unicode, arrays/nulls and exact large/decimal/exponent JSON numbers, including values not representable by float64. An initial test-only extra decoder used default float conversion for that positive fixture; it was corrected to UseNumber. A shadowed helper error was renamed for the existing lint policy. No production precision compromise or suppression was added.

After these corrections, full `make test` and `make lint` both exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-gemini-codec-tests.log` and `/private/tmp/prompty-task37-gemini-codec-lint.log`; `git diff --check` is clean. S06-05 now has a complete parent evidence assessment for SDK ordered parts/state/annotation handling, subject to both independent final audits. Other PRM-003 rows, conditional decisions and final audits remain outstanding; no final percentage, release, commit, external comment or issue closure is claimed.

## Previous checkpoint: Gemini mixed-part fidelity and required signed state

Source reconciliation found that `canonicalGeminiPart` chose the first matching switch branch when a single SDK part contained several content variants, silently dropping the others. Unsupported code/function-response payloads, media resolution/video metadata/display names and incremental function-call metadata could likewise disappear behind a supported variant. The source task and contract design first fixed the intended fail-closed mapper boundary; the adapter now rejects mixed supported variants with `ErrInvalidResponse` and unmapped fields with `ErrUnsupportedContentType`, with static diagnostics rather than caller content. These are declared mapper limitations, not provider-wide feature claims or silent normalization.

Signed Gemini continuation envelopes must also remain required. The previous validator accepted `Required=false`, allowing a signed envelope's preservation obligation to be weakened; it now rejects that persisted shape before dispatch. Existing ordered parse/wire/continuation behavior remains unchanged for supported parts. No provider SDK type or required infrastructure was added to core.

Permanent AAA fixtures cover 14 mixed/unsupported SDK part shapes, each with and without a signature, and reject corresponding saved signed payloads through the checked client with zero dispatch. A separate fixture rejects optional signed state. A positive mixed-history fixture preserves text, inline image bytes, signed reasoning, file audio, signature-only state and later text across response wire, restore, AddMessage/Clone/Normalize, execution wire and SDK translation, checking exact order/signatures and independent media/signature storage. Existing signed-call/citation/usage fixtures still pass.

After the last positive fixture, full `make test` and `make lint` exited zero across all modules and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-gemini-part-fidelity-tests.log` and `/private/tmp/prompty-task37-gemini-part-fidelity-lint.log`; `git diff --check` is clean. S06-05 remains partial with a concrete next check: persisted signed-codec duplicate keys, case aliases and exact shape are not proved by JSON struct decoding. Wider PRM-003 reconciliation, conditional decisions and both independent final audits remain required. No release, commit, external comment or issue closure occurred.

## Previous checkpoint: extended native conformance across declared fixture bindings

`forNativeFixtureModels` now runs the existing SDK protocol/tool/identity helpers on both the default-model and `fixture` binding: interleaved same-function calls, same-client parallel streams with a barrier after initial identity, reused provider ID across indices, missing/null/negative indices, ambiguous choice, conflicting identities/names, malformed/incomplete arguments, unknown finish outcome and segmentation-independent text/refusal references. Existing helper assertions and AAA semantics are preserved; constructors explicitly bind both model and profile. No new live model or provider support was inferred.

The synthetic `known` profile now has its own checked-client baseline suite: completion/trailing usage, two-delta read failure, finish-less EOF, consumer break, cancel and Close. It verifies zero translation/dispatch at handle creation, one translation/source dispatch, source release, single terminal except after consumer break, effective profile and retained content/logical refs. Provider-specific SDK indices are intentionally tested at the provider implementation, not fabricated for a generic in-memory source. The new synthetic suite initially exceeded the unchanged cognitive-complexity lint bound; its consumption phase was factored into a helper, without suppressions or policy changes.

`native-conformance.md` and S05-20 now record a complete parent implementation assessment for the currently declared native fixture profiles, subject to both final independent audits. S12-07 remains partial: broader state/schema/cache/capture profile reconciliation is not proved by the native inventory. Additional native-adapter demand remains unresolved separately. No final percentage is approved.

Full `make test` and `make lint` exited zero after the final test refactor across every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-native-profile-expanded-tests.log` and `/private/tmp/prompty-task37-native-profile-expanded-lint.log`. Module-specific OpenAI and shared adapter race suites passed too. No release, commit, external comment or issue closure occurred; the full goal remains active.

## Previous checkpoint: native fixture profile inventory and repeated baseline conformance

`native-conformance.md` now inventories every current adapter native-profile declaration: the default-model and `fixture` OpenAI bindings, native-only vs structured-output declarations, and the synthetic `known` preflight fixture. There is no built-in supported-model catalogue; other provider modules expose no native transport port. The inventory maps actual source declarations to core/common, SDK transport/protocol and feature-specific fixtures without treating local endpoints or SDK fields as live model support.

`cleanupFixtureClientModel` makes the effective model binding explicit while preserving existing default-model fixtures. Two permanent parameterized AAA suites run on both OpenAI model bindings: role/content/finish/trailing-usage completion, source read failure after two deltas, early EOF, and break/cancel/Close. Assertions prove lazy single dispatch, effective model/endpoint diagnostics, body closure, retained content/logical identity, expected failed/canceled/completed state and trailing usage. Initial execution found a test-only numeric assertion mismatch (`int64` expected vs canonical `int`); the expectation was corrected, not production code or contract. The entire OpenAI race suite and module lint then passed.

After the final diagnostics assertions, full `make test` and `make lint` exited zero across every module and generator fixtures, including race checks. Logs: `/private/tmp/prompty-task37-native-profile-tests.log` and `/private/tmp/prompty-task37-native-profile-lint.log`; `git diff --check` is clean. S05-20 remains partial with specific remaining supplemental tool/correlation/parallel per-binding reconciliation and synthetic fixture scope, replacing the now-completed inventory TODO. Conditional native-adapter demand, wider source reconciliation and both final independent audits remain outstanding. No final percentage, release, commit, external comment or issue closure is claimed.

## Previous checkpoint: logical stream identity, same-handle observation and canonical capture

StreamIdentity is now bound to ResponseChunk and StreamStatus: NewStream generates a local message ID at lazy handle creation, sources declare aligned logical PartIDs, and core validates nonblank/unique frame refs plus stable part-type/tool-ID binding before committing content/usage/refs. A provider call ID cannot be associated with two logical references. Frames/status/callback inputs clone references; source-owned frames remain unchanged. Terminal tool assembly remaps IDs with the same source association as annotations. Text fragments remain separate for exact state bindings, so retained/terminal references can repeat a logical text ID. Invalid/foreign/duplicate/missing/nil/kind/tool associations leave the previously accepted snapshot unchanged with a safe typed protocol error.

OpenAI declares distinct content/refusal logical references and stable tool-index references. Buffered results declare distinct complete-part references. Canonical frame capture now runs through WithStreamFrameObserver after identity validation/accepted accumulation; required observation failure retains/yields the parsed frame, cancels only the source before unwind and never replays. WithStreamCaptureObserver now receives StreamStatus, not only Response. Native and buffered canonical frame/terminal payloads carry stream_identity; buffered execution no longer writes an independent sync canonical terminal before its handle has identity. Normal sync capture remains unchanged and does not invent stream identity. Existing native original-schema validation and structured parser/capture error precedence remain intact.

Integration exposed two tracing defects: its second handle reconstructed already-normalized events, conflicting with message identity and discarding authoritative original terminal/capture status; sync error handling also discarded a known response. ObserveLifecycle now attaches optional lazy middleware to the same handle, preserving source/finalizer/capture and original identity. Start contexts add values but must retain the same cancellation channel; nil/detached bindings and post-consumption attachment fail explicitly. Nested closeout sees cloned final/canceled status without holding the mutex. Tracing uses this generic port with no tracing-specific core dependency, and retains known sync responses alongside errors.

Permanent AAA/race fixtures cover atomic reference rejection, nil/typed-nil parts, tool rebinding/aliasing, source/frame/terminal-observer/consumer/status mutation isolation, interleaved assembly, canceled readable refs, observation failure and source unwind, lazy/nested lifecycle contexts/finalizers/cancellation and tracing's original pointer/identity/finalizer/error/usage. Actual SDK/body fixtures prove segmentation-independent text refs and separate refusal refs. Native capture checks identical message/part references at canonical frame and terminal, including incomplete tool prefixes followed by assembled calls. Buffered capture checks the same identity, one closeout, zero dispatch on pre-dispatch sink failure and retained known content/usage/refs for post-dispatch sink failures. Same-client parallel SDK fixture checks distinct message IDs despite equal tool IDs/indices. Source fixtures/examples were updated explicitly; no implicit reference guessing or compatibility bridge was added.

Original S05-02 and two stable logical-identity/lifecycle clarification rows now have complete parent evidence assessments. Source task, contracts, migration, README/godoc and author closeout draft describe the clear break and runtime/capture-only identity. Per-profile conformance inventory and additional-native demand remain open under their existing rows; wider PRM requirements, conditional decisions and both independent final audits are still required. No final completeness percentage is approved.

Final complete make test and make lint both exited zero on the last SDK tool-capture fixture across all modules and generator fixtures, including race checks. Logs: /private/tmp/prompty-task37-stream-identity-final-tests.log and /private/tmp/prompty-task37-stream-identity-final-lint.log. An initial whole-workspace lint run found six unused suppressions caused by the broad formatter changing tag spacing in two otherwise unrelated manifest files. The exact formatting-only diff was inspected and restored through apply_patch; those files have no remaining diff and no suppressions/policy were changed. Earlier targeted lint requests were addressed by factoring identity/context validation, using slices.Backward and strings.Builder, and removing an error shadow—not by disabling checks. git diff --check is clean. No release, external comment, issue closure or commit occurred; the full goal remains active.

## Previous checkpoint: native correlation presence and same-client parallel conformance

Native OpenAI now requires actual SDK JSON presence for choice/tool indices: omitted/null choice or tool index no longer becomes zero; negative tool index and nonzero/multiple canonical choices fail with typed invalid-response diagnostics. Continuations still omit already-bound ID/name safely. Real SDK/body fixtures cover missing/null/negative indices, choice ambiguity, conflicting names/IDs, uncorrelated continuation, malformed/incomplete args and finish-less EOF. Protocol/materialization failures retain accepted partial content and usage, close the actual SDK-owned body and dispatch once despite globally enabled SDK retries.

A permanent same-client local SSE/race fixture runs two consumers concurrently, blocks both after their initial identity frames and then releases continuation. Both requests use tool index zero and the same provider ID but distinct function names/arguments; results remain independently correct. Test goroutines use WaitGroup.Go under the existing Go minimum. Initial lint requested only golines formatting and that modernize substitution; no suppression or policy change was added. AUD-03's two original rows now have complete parent evidence assessments, subject to both final independent auditors. A new stable index-presence clarification row is also assessed complete. The native-profile row still names real unfinished logical-reference/capture conformance rather than the now-implemented negative/parallel tests.

Logical message/part identity remains unfinished. The Spec-First design section now distinguishes a local handle-owned message identity, source-declared logical part references, transport fragment offsets and provider call IDs; it requires clone/early-stop/terminal remapping and canonical capture preservation, with no guessed adjacency or provider/physical identity claims. Current ResponseChunk/status/capture paths were inspected: identity must be established before capture as well as consumer delivery, and state/annotation offset bindings must remain exact. This design section is explicitly not implementation or S05-02 completion evidence. No new unwired API or compatibility bridge was introduced.

After the final production and SDK fixture changes, make test and make lint both completed with exit status zero across every module and generator fixtures, including race checks. Logs: /private/tmp/prompty-task37-stream-correlation-tests.log and /private/tmp/prompty-task37-stream-correlation-lint.log. Existing exhaustruct deprecation warning remains non-failing. git diff --check is clean. Goal is still active: broader requirement reconciliation, logical identity, conditional decisions and both final independent audits remain. No release, external comment, issue closure or commit occurred.

## Previous checkpoint: SDK stream cleanup, argument identity and outcome reconciliation

Real OpenAI SDK/body fixtures now prove break/cancel/Close release a body containing unread deltas, retain canceled partial content, never yield again after consumer stop and dispatch once despite globally configured SDK retries. A typed source read failure after exactly two accepted deltas retains onetwo, failed status, one terminal and the original cause, without replay. Existing module-wide goleak checks run unchanged. The new buffered fixture proves lazy translation/dispatch, single use, explicit buffered mode and preserved finish/outcome/known usage. An initial test panic was caused by using a schema-only fixture without ResponseFormat; it was corrected with a plain checked metadata fixture, not a production workaround.

Source reconciliation found and fixed three real contract defects. Provider ID reused at another tool index could silently collapse two calls into one valid object; OpenAI now enforces the inverse ID-to-index binding too. Conflicting tool names/snapshots now preserve ErrIncompleteToolCallArgs without exposing caller values, and Translate rejects Args plus ArgsChunk even when equal. Across separate frames the assembler now retains the original argument representation: mixing a snapshot and a delta fails in either direction even if the resulting JSON would be valid; identity-only frames choose no mode and equal snapshots remain idempotent. Finally, OpenAI unknown/missing finish reason no longer defaults to completed: it remains incomplete, preserving content/reason and trailing usage. A native SDK/body fixture explicitly distinguishes completed transport from that incomplete provider outcome; the known-stop empty-response regression remains intact.

The source task, contracts and migration document these clear breaks. Seventeen existing streaming rows plus two stable clarification rows now have complete parent evidence assessments, not independent final approval. S05-02 remains partial because ResponseChunk still lacks stable logical message/part identity across text deltas; fragment offsets are not a substitute. Additional native-adapter demand, declared-profile negative/parallel conformance and other task requirements remain open. No requirement is excluded and no final completeness percentage is approved.

After the last production/fixture changes, complete make lint and make test both finished with exit status zero across every module and generated fixtures, including race checks. Logs: /private/tmp/prompty-task37-stream-lint-final.log and /private/tmp/prompty-task37-stream-tests-final.log. The linter's existing exhaustruct deprecation warning is not a failure; no policy/suppression changed. git diff --check is clean. These are checkpoint gates, not completion or a substitute for the two independent final audits. No release, external comment, issue closure or commit occurred.

## Previous checkpoint: explicit injected-client endpoint binding

All four provider descriptors now return an empty endpoint for an injected SDK client unless the caller supplies WithProfiles endpoint binding. They do not introspect SDK-private options, reconfigure transport or probe the network. A nominal default remains only for descriptor inspection without an executable client. Checked Prepare already rejects empty/blank endpoint, so sync/native/buffered paths fail before Translate/dispatch; execution knowledge override cannot manufacture the missing transport identity. Explicit zero-profile binding remains usable for plain requests without inventing capabilities, independently of option order.

The permanent cross-provider AAA fixture uses actual descriptors/Translate with SDK dispatch intercepted, covering absent and whitespace binding, zero calls/translations across modes, override non-bypass, both option orders and nominal no-client inspection tests. Initial module race fixtures passed. Full make lint and make test completed with exit status zero on the final override/blank fixture across all modules/generator fixtures with race checks, after correcting the fixture's embedded-field spacing without suppression. git diff --check is clean. Three executable examples and the README SDK URL/binding now use one caller-owned endpoint value; migration/provider READMEs/godoc and author closeout instructions state the clear break. This does not establish capability knowledge for example tools/output without caller profiles; wider example/requirement reconciliation remains open.

The matching original profile-destination row and one stable clarification row now have complete parent evidence assessments, still subject to both final independent audits. Scope/credential edges, custom Invoker guarantee contract, other requirement reconciliation and conditional demand decisions remain open. No release/comment/closure was performed.

## Previous checkpoint: parent requirement reconciliation, ownership and preflight

The first ownership/boundary requirements were reconciled against current source rather than old checkpoint notes. Core imports were inspected with go list; provider SDK requirements remain in four separate adapter modules. Caller DTOs, validators, capture ports, media transport and wire policy are injected, generated recipes remain optional, and inspected core execution/stream/tool/capture code does not add a scheduler, agent loop, retry coordinator or storage backend. Eight S01 rows now carry complete parent assessments with concrete implementation evidence. Credentials/scope/conditional hosted-tool edge conformance remains partial instead of being inferred from constructor injection alone.

Seven selected PRM-001 profile/preflight acceptance rows now carry complete parent assessments after reading their implementation and rerunning the actual race fixtures. The matrix explicitly distinguishes these current implementation assessments from final independent approval: either auditor can revoke a row, and no final completeness percentage is approved while conditional decisions or audit results are missing. A row is no longer kept partial only because an audit has not run; real incomplete scope is named instead.

The reconciliation exposed a concrete remaining destination issue: all four provider descriptors currently use their default endpoint when an opaque SDK client is injected without explicit WithProfiles binding. This can produce profile/state identity unrelated to the actual injected transport. Caller-owned endpoint binding must be explicit rather than inferred from SDK defaults; this remains named under S04-06 for implementation. Shared checked adapter paths already integrate schema/cache/state/capture; the old matrix claim that this integration is absent was removed. The remaining S04-04 question is the guarantee contract of custom caller-supplied core Invoker helpers, not missing adapter integration.

Two targeted race commands completed with exit status zero for core/shared adapter boundary, capture optional/required, tool scope, explicit schema resources, native mode/unknown model, profile shape/ambiguity/override/wire/clone acceptance fixtures. Source and matrix scope were preserved. No production code changed in this reconciliation; prior full make lint/test results remain the preceding code checkpoint, not final acceptance. Independent final audits and all conditional demand decisions remain outstanding.

## Previous checkpoint: selected profile shape and deterministic resolution

Selected capability knowledge now passes the shared ValidateCapabilityProfile shape validator: nonblank destination/provenance, recognized support states with caller-owned extension keys, nonempty unique-member forbidden combinations, nonblank unique MIME entries, and object/boolean model-options schema shape. Core typed execution-wire paths and the executable common wire schema enforce matching override invariants. Errors expose static fields/categories rather than caller values. Truth of upstream support and model-options keyword semantics still require the actual profile validator/conformance; shape validation does not invent support.

Descriptor selection no longer stops at the first matching destination. Ambiguity is rejected before inspecting candidate knowledge, making even malformed/valid candidate pairs order independent. A valid matching explicit override replaces descriptor knowledge directly. Unrelated profiles cannot establish or invalidate an unknown effective destination. Profile clone preserves an absent combinations collection rather than turning it into an empty collection. No provider/model catalog or transport implementation is inferred.

Permanent AAA fixtures cover runtime/typed-wire/JSON wire-schema rejection, extension-capability and boolean-schema round-trip/isolation, malformed unrequested feature data across sync/native/buffered paths with zero translation/provider calls, explicit replacement of ambiguous descriptor knowledge, order-independent safe diagnostics and unknown-model rejection despite a real native port. A full mutable-field clone fixture covers map, nested combination, MIME collections and model-options bytes, including absent-collection shape. Initial new fixtures exposed an omitted explicit persistence policy and a clone nil/empty inconsistency; these were fixed without a compatibility path. The nested mode fixture was factored into an AAA helper to satisfy cognitive-complexity lint without suppression. Final independent audits, conditional decisions and wider evidence reconciliation remain open.

The final clone-fixture make test completed successfully across all modules/generator fixtures. During the concurrent lint gate the externally supplied local Go/linter changed; the current linter reported six modernize requirements in existing executor/schema/template/domain reflection code. Equivalent errors.AsType and reflect.TypeAssert substitutions use APIs available under the existing module Go minimum; no module minimum, dependency, lint policy or behavior was relaxed. Both full gates then completed with exit status zero after these mechanical changes, across every module and generator fixtures with race checks. The linter emits an exhaustruct deprecation warning, not a failing diagnostic; no checks were disabled. git diff --check is clean. A transient formatter PATH warning was rechecked: Go was available at the configured path, formatting reran successfully, and no tool installation or shell configuration change was made. These are checkpoint gates, not final acceptance.

## Previous checkpoint: structured parser/document errors in the stream lifecycle

Incremental structured parsing now attaches to the same lazy handle before consumption, after preserving each received chunk and before provisional item delivery. It composes document completion/original validation with the existing prepared finalizer instead of canceling the handle as a consumer on parse error. Parser failure cancels only the source before iterator unwind, preserves partial results and reaches failed terminal observation; voluntary consumer break remains canceled without another yield. ErrStructuredStream distinguishes invalid incremental JSON/separators/shape from refusal and cancellation, and capture classifies these errors safely as parse failures.

Permanent AAA lifecycle fixtures cover empty/single/multiple valid arrays, byte-separated whitespace, leading/repeated/trailing/missing commas and truncated arrays, retained earlier provisional items, one final observation/one invocation, original finalizer composition, producer cleanup and ordinary consumer break. Known non-completed outcome in a content frame now suppresses new items; the old mock expectation was updated instead of keeping a compatibility behavior. OpenAI exposes known refusal on deltas, so the helper can retain trailing usage without attempting to decode refusal as JSON. A real SDK/local-SSE fixture checks one provider call, failed refusal status, known total 9 and one correctly classified captured terminal.

Incremental decoder failures now preserve their typed underlying cause beneath ErrStructuredStream. Caller-owned Validate failures instead use the existing ValidationError, preserve errors.Is/As identity, and do not become parse errors or cancellation. Permanent fixtures distinguish balanced-but-invalid JSON from valid JSON rejected by business validation, assert failed status/one observation/no finalizer, and verify safe capture categories without exposing the caller error text. Contract and migration guidance describe the absent full assistant message for provisional stream-domain failures. Shared safe category constants satisfy lint without disabling checks.

Core/shared/OpenAI race checks passed. Full make lint and make test both completed with exit status zero after the safe-category constant cleanup, across all modules and generator fixtures with race checks. git diff --check is clean. Remaining independent audits/conditional scope and broader task evidence reconciliation remain required; no final acceptance is claimed.

## Previous checkpoint: media redirect/body-limit conformance

AUD-04 now has permanent AAA fixtures for a multi-hop HTTPS chain ending in an attempted HTTP downgrade (zero unsafe calls), default ten-request vs caller-owned three/twelve-hop limits, cancellation between redirects (zero next-hop calls), concurrent independent allow/deny fetcher policies, and exact/over/default/int64-boundary body limits. The host's redirect/egress policy remains independent; no private-address ban or universal redirect ceiling is imposed over an injected CheckRedirect.

The boundary fixture exposed a further confirmed implementation defect: MaxBodySize=MaxInt64 overflowed maxBytes+1 and returned an empty successful download. readBoundedBody now reads up to the caller limit and probes one extra byte separately, avoiding addition overflow and accepting exact limits. A further fixture preserves read/probe failures and cancellation instead of converting them to empty success or a fabricated size violation. The original task includes this implementation-time body-limit finding and a stable additional matrix requirement; no earlier requirement was removed. Contract/migration guidance reflects the corrected semantics.

Complete make lint and make test both finished with exit status zero after the final read/probe regression across every module and generator fixtures with race checks; git diff --check is clean. Remaining original scope, conditional dispositions, release/closeout final evidence and both independent audits remain open. This is checkpoint evidence, not final acceptance.

## Previous checkpoint: public documentation and release/closeout preparation

README and provider READMEs/godoc now describe full Response metadata, checked endpoint/model profiles, actual StreamerAdapter.StreamEvents, explicit lazy/single-use native vs buffered lifecycle and no silent polyfill. Chunk parsers/stubs no longer advertise native transport. Provider mapping descriptions are separated from model/endpoint conformance; conditional remote/resource features are not claimed delivered. Generated recipes are explicitly optional alongside caller-built executions, preserving BYOT rather than imposing an application workflow. The quick-start SDK pointer constructor and basic-chat outcome check are corrected. Media fetch documentation no longer claims arbitrary document downloading or lack of HTTP dependency.

An executable OpenAI SDK/local-SSE example proves profile binding, trailing usage, sole completed lifecycle, explicit response wire/restore and AssistantMessage continuation. It preserves partial usage after persistence, without a paid/live request. Its module race test passes.

`release-closeout.md` prepares full acceptance/audit gates, release-break vs patch guidance, actual script side effects/scope cautions, unresolved conditional decisions and an author-facing migration draft with concrete API changes. It explicitly forbids posting/closing/publishing until accepted and separately authorized; actual final evidence links remain placeholders, not invented results. Three omitted normative release/closeout paragraphs now have stable matrix IDs, and migration/closeout rows carry provisional evidence rather than false completion credit.

Complete make lint and make test both finished with exit status zero after the final documentation/example changes across every module and generator fixtures with race checks; git diff --check is clean. The basic-chat outcome message is constant rather than logging a provider-tainted value, satisfying the existing log-injection check without suppressions. Full task scope, conditional decisions, remaining conformance edges and both final independent audits remain outstanding. No release/comment/closure was performed.

## Previous checkpoint: semantic capture report validation

CaptureReport.Validate rejects blank invocation identity, duplicate/unrecognized coverage/accuracy, duplicate/uncovered observations, Closed inconsistent with observed terminal, Complete without Closed or with failures, and malformed failure stage/classification. Unsupported-stage failures are valid without inventing observed coverage. Runtime canonical metadata validation and both typed response-wire directions enforce these invariants; JSON decoding applies structural checks first and semantic checks on restore. Core invalid hook stage/classification input is rejected without corrupting the report.

Stream capture observers validate and clone their reports before installing status evidence. Invalid reports produce ErrCaptureReport while retaining source/capture errors and known partial results; existing observer fixtures were migrated to internally consistent reports, without a compatibility bypass. AAA tests cover thirteen contradictory reports through marshal/typed restore/JSON decoding, valid complete/redacted/pending/failed/crash snapshots, observer error precedence/content preservation and invalid hook zero writes. Public contract/migration/evidence are updated.

Missing required stages now publish Complete=false under the same mutex as Closed=true, before recording the safe failure classification. A concurrent report read cannot see the earlier transient complete terminal. The existing missing-stage regression now uses a complete semantic projection (not default redaction that masked the risk) and asserts the valid failed report/classification.

Complete make lint and make test finished with exit status zero after the final atomic-completeness adjustment across every module and generator fixtures with race checks; git diff --check is clean. The remaining full scope, conditional dispositions, release/closeout and both independent audits stay open. These checkpoint gates are not a final completeness verdict.

## Previous checkpoint: host-projected capture metadata

CaptureProjection now carries CaptureRecordMetadata: separately observed provider response identity/request identity, optional HTTP status, usage presence/outcome and role/name/source/translated schema digests. It deliberately contains no schema profile endpoint/provenance/raw values. Metadata is cloned before projector and sink callbacks; the default projection excludes response/request IDs and retains safe isolated usage/outcome/digests. Projected malformed usage/outcome/status fails before delivery. No status/request ID is inferred from SDK JSON, and no transport/attempt coverage is added.

The generic checked path attaches compilation digests before dispatch, at provider response and on canonical result snapshots, including nil-response failure terminals without inventing usage/outcome. OpenAI adds SDK-observed response ID and usage before sync/native normalization. AAA tests cover callback mutation isolation, default privacy/presence, invalid metadata zero writes, source-versus-translated digest associations, failed terminal digest retention, sync response ID/usage and native trailing usage/terminal outcome. Two normative source paragraphs (Stages/Record) previously absent from the extracted matrix now have stable IDs; they remain partial, not excluded or claimed complete. Broader record/report semantic validation remains open.

Complete make lint and make test both finished with exit status zero after the final nil-response digest regression and lint-required enum-map fixture construction correction, across every module and generator fixtures with race checks. git diff --check is clean. Remaining full scope, conditional decisions and both final audits stay open; these checkpoint gates do not establish final completeness.

## Previous checkpoint: exact thinking-envelope shape

Anthropic thinking restore no longer relies only on the permissive struct decoder. The codec checks case-sensitive keys, duplicate keys (including Unicode-escaped equivalents) and exact per-variant membership before decoding string values. Cross-variant null fields, case aliases and duplicate signed fields now fail with ErrProviderState instead of being silently omitted by SDK reconstruction. Valid empty exposed thinking, reordered fields and escaped string values remain accepted. The public contract and migration guidance were updated before implementation.

Permanent AAA coverage in `adapter/anthropic/state_test.go` exercises codec shape, retained raw SDK response rejection and persisted continuation rejection with zero HTTP calls. The Anthropic module race suite passes. Targeted lint reports zero issues; its default cache emitted filesystem-permission warnings, so the full gate used a fresh task-owned lint cache. Complete `make lint` and `make test` both finished with exit status zero across every module and generator fixtures; `git diff --check` is clean. Capture metadata and the remaining full scope/final audits remain open.

## Previous checkpoint: runtime/wire usage invariants

`Usage.Validate` now rejects negative counters, invalid/duplicate Known entries and unrecognized modes. Runtime accumulation preserves provider-defined totals, keeps cumulative/delta semantics separate, checks integer overflow and commits a usage/content update atomically. Invalid updates fail with ErrStreamProtocol and ErrUsage without altering earlier accepted partial results. MarshalResponse and typed restore reject the same malformed metadata. Checked sync preserves a known response with ErrUsage and one provider call; custom invokers cannot pass malformed metadata through structured/tool helpers into success, repair/retry or a handler. A prior mock with missing canonical outcome was migrated to explicit NewResponse rather than adding a zero-outcome compatibility fallback. `usage_test.go` and `adapter/usage_test.go` provide permanent AAA coverage.

Gemini zero-counter presence now requires a matching numeric field from a valid retained HTTP body. Null/string/boolean/object values, mismatched SDK/body values and malformed body cannot become known zero. Nonzero SDK counters retain their independent presence evidence. `adapter/gemini/usage_test.go` covers these cases; module race tests pass.

Complete `make lint` and `make test` both finished with exit status zero across every module, examples, Git registry and generator fixtures after the final Gemini presence guard. `git diff --check` is clean. The goal remains active. Capture record metadata (known status/request identity/usage/schema digests), envelope/capture/wire edge validation, complete docs/release/closeout, conditional remote/resource decisions and both independent audits remain outstanding; these are not excluded from the full task scope.

## Latest checkpoint: profile-scoped reasoning configuration

`CapabilityProfile.ModelOptionsSchema` now constrains the existing complete ModelOptions object for the effective destination. RuntimeDescriptor declares reasoning-setting keys and clones this metadata; profiles clone their schemas. Checked preflight rejects unknown reasoning knowledge, malformed/unresolved schemas, unsupported values/budgets/visibility and forbidden reasoning+tools combinations before Translate/dispatch. The wire schema preserves profile options contracts. `adapter/model_options_test.go` proves effective-model/override scope, original snapshot/wire isolation, invalid combinations, and zero translations/calls on failure. No parallel universal reasoning DTO, remote catalog or hidden reference loading is introduced.

OpenAI no longer uses model-name prefixes or effort presence to remove temperature. MaxTokens consistently maps to max_completion_tokens. Anthropic maps enabled/adaptive/disabled thinking independently of budget, visibility and output effort; an actual SDK/local HTTP fixture verifies adaptive/display/effort request bytes. Manual budget is an integer at least 1024 and below effective max_tokens. Gemini maps dynamic budget -1 or an explicit level, rejects mixing both, and retains exposed-summary visibility as a separate flag. Profiles—not SDK field existence—define valid model values/combinations. ProviderSettingsMap now preserves numbers as json.Number; internal conversion tests cover decimal/exponent integer semantics, exact integers above float precision, out-of-range values and the fixed rounded-float upper-bound wrap.

Complete `make lint` and `make test` both finished with exit status zero across every module, examples, Git registry and generated fixtures after these changes. This is an implementation checkpoint, not a final completeness percentage or correctness verdict. Remaining result/envelope/usage/capture/schema edge proof, complete migration/release/closeout coverage, conditional remote/resource decisions and both independent final audits remain outstanding. The original full task remains authoritative.

## Latest checkpoint: cache controls and stream associations

CachePolicy.Type is removed from runtime types, manifests, test fixtures and executable wire schemas. `cache.go` separates implicit-prefix/block-marker intent, advisory/required mode, provider-scoped JSON controls and safe disposition diagnostics. Prepared execution checks actual mapper support and effective profile knowledge, reports omitted advisory intent and fails required unknown/unsupported/foreign intent before translation/dispatch. Implicit caching does not fabricate request fields or observed hits. `adapter/anthropic/cache.go` maps TTL 5m/1h, validates actual SDK marker limits/order, and rejects unsupported controls. Other raw translators now reject required unmapped intent. Local SDK tests verify TTL reaches the provider request, absent TTL knowledge/bypass/unsupported values stop before dispatch, and known cache read/write counters are not added twice. Policy controls survive wire/clone; Normalize does not merge incompatible message-level policies.

Stream assembly now retains a source-to-materialized part-index map. Unrelated annotations and opaque state follow changed positions when tool deltas collapse. Changed state-bound content fails closed without replacing the known partial result. `stream_association_test.go` covers interleaved same-name calls followed by signed reasoning/citation, response wire round-trip, and rejection of changed state-bound delta. First-source ambiguous Args/ArgsChunk and incomplete Args snapshots are now rejected explicitly, and assembled cache controls are cloned.

OpenAI raw chunk capture errors are preserved together with early tool-identity/name/correlation errors. Disabled canonical chunk capture returns before constructing a response payload.

Targeted race tests and lint pass for core/shared adapter/Anthropic/OpenAI/tracing. Latest complete `make lint` and `make test` both finished with exit status zero across every module, examples, Git registry and generated fixtures after the cache/association/snapshot changes. `git diff --check` is clean. Final audits have not run. Reasoning configuration, remaining capture/result/schema edge proof, complete release/closeout documentation and conditional remote/resource decisions remain open. Cache controls do not implement optional resource lifecycle; unresolved triggers have not been excluded from completeness.

## Implemented paths and current evidence

- Shared capability preflight is now called by `adapter.NewClient` sync and native paths. `adapter.Prepare` clones the execution before translation, returns an opaque prepared payload and isolated diagnostics, and requires endpoint/model-bearing metadata. Required unknown/unsupported features fail before translation/dispatch. Provider adapters accept caller-owned `WithProfiles(endpoint, profiles...)`; no provider-wide capability catalog is inferred from names. Fixture coverage: `adapter/preflight_test.go` and `adapter/openai/stream_contract_test.go`.
- Explicit `Invoker.ExecuteStream(ctx, execution, mode) *Stream` replaces the old iterator API. Handles are lazy and single-use; `Events`, `Status`, `Close` preserve partial results and cancellation. Native does not fall back to sync; buffered cannot satisfy a native requirement. OpenAI SDK retries are disabled per invocation, trailing usage is requested, and multiple/nonzero choices are rejected rather than silently discarded. Lifecycle fixtures: `stream_lifecycle_test.go`.
- OpenAI tool continuation uses request-local index-to-identity correlation; `AssembleToolCallParts` replaces adjacency-based assembly. The real SDK/local SSE fixture covers interleaved same-name calls without repeated ID/name.
- Original JSON Schema validation uses the injected `SchemaValidator` port and an existing standard validator implementation. External references require explicitly supplied resources. Structured sync rejects refusal/incomplete before parsing; structured stream items are provisional and the complete document is validated at terminal exhaustion. Explicit stream schemas describe the complete document; inference accepts a single T object or an array of T objects. Fixtures: `schema_validation_test.go`.
- Local tool helper compiles execution parameter schemas before model invocation, validates all args against those original schemas before any handler, and checks whole-batch execution scope/forced tool/duplicate IDs. Missing parameter constraints mean a JSON object with no further restrictions. Refusal/incomplete never invokes tools. Scope fixtures: `task37_regression_test.go`; original-schema whole-batch and pre-invocation reference fixtures: `schema_validation_test.go`.
- Media fetching uses an instance client, preserves caller redirect restrictions and checks HTTPS on every redirect before following it. Fixtures: `mediafetch/fetch_test.go`.
- A protocol-error unwind deadlock was reproduced during testing: a source awaiting context cancellation after false yield could block iterator unwinding. Cancellation now occurs before returning to that source. A permanent `TestStreamFailureCancelsBeforeSourceUnwinds` covers protocol and transport errors.
- Provider envelopes now bind opaque payloads to exact part indices and scoped destinations. Response/history/clone/Normalize/scratchpad preserve those associations. Signed/annotated system messages are not merged. Legacy or corrupt format markers are rejected; persistence requires explicit `WirePolicy`, while logging and canonical JSON omit opaque state. Evidence: `provider_state_test.go`.
- Execution/response wire schemas are bundled explicit resources. Both JSON decoders and typed restore functions validate structural schemas and semantic associations, without hidden reference loading. Evidence: `wire_schema_test.go`; arbitrary schema/data fields remain caller-owned JSON rather than being interpreted as executable external references.
- Gemini parsing now preserves original part order, exposed reasoning summaries, signatures, candidate annotations, available usage and finish outcome. Request-scoped parsing binds envelopes to the effective destination. Gemini signed-part parse → response wire → history → execution wire → translate round-trip is covered by `adapter/gemini/state_test.go`. Unknown native streaming support is unchanged.
- Gemini/Ollama retain refusal/incomplete or finish/usage metadata rather than returning a content-only default result. SDK zero counters without presence remain unknown; retained Gemini HTTP body can establish known zero. Ollama total is derived only from known prompt and evaluation counters. Evidence: `adapter/ollama/metadata_test.go`, `adapter/gemini/state_test.go`. Complete remaining provider mappings are still pending.

## Verification status

- Gemini response-schema mapping now rejects unmapped keywords/shapes instead of silently dropping them. Typed `SchemaFidelityError` carries deterministic RFC 6901 pointers, including escaped property names. `adapter/gemini/schema_test.go` checks numeric enum/minimum/references and nested constraints fail before transport dispatch, without source mutation. The module race suite and root `go test ./...` pass. Explicit lowering, digests, general compiled schemas and tool-schema fidelity remain pending.

- `make test` completed successfully across every module and generator fixtures, including race checks, before the subsequent lifecycle refactoring. This is a checkpoint result, not a final gate for later edits.
- After correcting the reproduced unwind deadlock, `go test -timeout=60s ./... ./adapter/openai/... ./ext/otelprompty/...` passed. A subsequent `make test` also completed successfully across all modules and generator fixtures with race checks, including the new lifecycle and tool-schema regression tests.
- `make lint` subsequently completed successfully across all modules with zero issues. The Makefile now uses the documented serial-runner option to wait safely for concurrent external lint locks; no checks were disabled and no other task's lock/process was modified.
- After the state/wire implementation, the full `make test` completed again across all modules and generator fixtures with race checks. The one stale invalid-union assertion was changed to assert the executable schema error and its precise JSON path. A subsequent complete `make lint` finished with zero issues. These are checkpoint gates, not completion of the full task or substitute for the final independent audits.
- After the native Anthropic output change, module `go test -race ./...` passes and module `golangci-lint run --allow-serial-runners ./...` reports zero issues. Whole-workspace final gates and independent audits remain outstanding.
- After strict Gemini mapping, complete `make lint` and `make test` both exit successfully across all modules and generator fixtures, including race checks. These are current checkpoint results, not evidence that lowering/capture/cache/conditional scope or final audits are complete.

GitHub issue #4 was re-read through the authenticated CLI: it remains open, last updated `2026-10-03T16:14:48Z`. Its conditional lifecycle scope remains intact. The task's explicit clear-break decision supersedes the issue's historical additive/compatibility suggestions.

## Outstanding scope

The universal `schema_compile.go` contract now defines strict/lowering policy, a provider mapper port, immutable compiled source/translated documents, exact-byte SHA-256 reports and mandatory original validation. `schema_compile_test.go` covers forbidden/unreported transformations, source/policy/report isolation, large integer precision, unresolved references before mapping, and optional non-null → required nullable semantics. Its isolated-cache targeted test run passes. Portable policy is now in `Requirements.Schema`, cloned through execution/history/Normalize and included in the strict execution wire schema; `schema_policy_wire_test.go` adds round-trip and invalid-policy fixtures. Those new fixtures and current lint are still being checked. Checked adapter binding, provider transformation implementations and finalization integration remain required. Shared-cache test/lint setup failed with missing module/build files before compilation; no shared cache is cleaned or modified to work around this.

Core race tests and core lint subsequently passed after portable policy wiring. `ValidateSchemaPolicy` now protects shared adapter validation and both execution-wire directions from malformed intent (including whitespace-only transformation IDs). New final regression tests are being rerun; this is policy validation, not provider-fidelity enforcement. Valid lowering is not yet a delivered checked adapter feature.

Checked schema binding now runs before Translate in `adapter.Prepare`. It requires an actual mapper port, compiles original output/tool schemas, maps only the working copy, associates role/name/profile and digests in diagnostics, and validates sync results with original validators. A fake lowering mapper proves minimum violations are rejected after one dispatch while preserving response/diagnostics; strict rejects that transformation before Translate. Missing mapper cannot be filled by a knowledge override. Anthropic native object-schema passthrough is the first actual provider binding. Schema decoding uses `json.Number` to preserve large numeric constraints in SDK serialization. Core/adapter/Anthropic race tests and lint passed before the last two additional regression fixtures; their final rerun is pending. Other provider mappers, native-stream finalization and complete provider fidelity coverage remain outstanding.

The SDK numeric-serialization regression exposed that Anthropic encodes `json.Number` as a string. Its schema mapping now converts those numeric literals to `json.RawMessage` recursively, preserving numeric type and exact digits in SDK JSON. A permanent large-integer regression passes. Gemini's strict subset mapper is also bound to checked compilation; its `$ref` rejection fixture supplies an explicit source resource so source validation and provider mapping are tested independently. Unprovided references remain rejected before mapper invocation in core tests. Compilation reports survive response wire and are isolated from mutation. OpenAI/Ollama schema ports, actual provider lowering transformations and native-stream final validation are still required.

Final checks for this checkpoint pass with isolated caches: `go test -race . ./adapter ./adapter/anthropic ./adapter/gemini` and `golangci-lint run --allow-serial-runners . ./adapter ./adapter/anthropic ./adapter/gemini`. This is the changed-slice verification, not final whole-workspace gates or independent audit. Source-validator error pointers still need complete coverage separate from provider-mapper pointers.

OpenAI silent normalization is removed. Its mapper now reports and requires explicit `openai.close-object` / `openai.required-nullable` permissions, recurses through actual schema child keywords (not arbitrary enum/default data), and preserves source pointers before adding nullable wrappers. A real SDK/local HTTP fixture proves strict zero-dispatch rejection, permitted lowering, original minimum/non-null rejection after one provider call, preserved known response/diagnostics and exact SDK request semantics. Large numeric constraints remain numeric and exact through SDK JSON. The OpenAI module race suite passes; final current lint/provider checkpoint runs are pending. Ollama tool SDK mapping remains unbound and can lose unsupported keywords; no fidelity support is claimed for it yet. Native-stream original-schema finalization also remains required.

The ensuing isolated-cache `go test -race . ./adapter ./adapter/anthropic ./adapter/gemini ./adapter/openai ./adapter/ollama` passes. Lint over the same packages reports zero issues after formatting fixes. These remain checkpoint gates, not whole-workspace completion or substitute for the two final audits.

Ollama is now bound to checked tool-schema compilation. Raw translation also checks actual SDK decode/serialization rather than silently dropping fields. Regression fixtures cover root additionalProperties, nested minimum/required and large enum numeric rounding, with exact source pointers, unchanged source and zero provider calls. A representable schema has an exact SDK JSON round-trip. The module race suite passes; final provider-wide lint/checkpoint runs are pending. Output schemas and lossy lowering remain unsupported, not falsely declared delivered. Native-stream original-schema finalization remains next required work.

The subsequent core/shared-adapter/all-four-provider `go test -race` checkpoint passes, and lint over those packages reports zero issues. Final whole-workspace gates and independent audits still require completion after remaining work.

Native-stream original validation is now bound through `WithStreamFinalizer`: EOF/usage/tool assembly precedes the validator, and only its success permits completed status. Original mismatch retains known content/usage/profile/schema diagnostics; buffered validation failure likewise preserves the known response. Core fixtures prove finalizer snapshot isolation, exact single terminal, assembly ordering and cancellation without a held status lock. Shared adapter fixtures prove native original-minimum rejection and buffered known-response retention. A real OpenAI SDK/local SSE regression checks original non-null rejection after trailing usage with one dispatch and one failed terminal. Current wider checks are in progress; no final whole-workspace or independent-audit claim is made.

The finalization checkpoint race suite passes for core, shared adapter, OpenAI and tracing middleware. Regression checks also exposed and fixed two boundary errors during this change: cleanup cancellation must not override an existing transport/protocol error, and EOF materialization must not rewrite a canceled partial snapshot. Permanent fixtures cover both error precedence (existing lifecycle regressions) and canceled result stability. Whole-workspace final gates, remaining provider metadata/reasoning/capture/cache scope and both independent audits remain outstanding.

- Remaining provider state/outcome/usage mappings, reasoning configuration/profile checks, strict envelope/usage edge cases and complete conformance for the new wire contracts. Base envelope/response wire/continuation/logging paths and one Gemini round-trip are implemented but not independently audited.
- Provider schema fidelity compiler, explicit lowering diagnostics/digests and complete provider combinations. Anthropic native `OutputConfig.Format` mapping now coexists with real tools; the synthetic tool path and magic-name parser are removed. `adapter/anthropic/native_output_test.go` exercises checked profile execution through the real SDK against a local server and source-schema isolation. Module `go test -race ./...` passes; this does not establish live model support or completed fidelity compilation.
- Optional capture protocol and OpenAI sync/native boundary hooks; cache controls/disposition and TTL.
- Trigger decisions for remote operations/background/hosted tools, offline batch and explicit cache resources remain pending user input. No silent deferral or denominator exclusion is authorized.
- Complete documentation/examples/release and issue-closeout draft, final lint/test gates and both independent audits. `migration.md` documents implemented changes only; it is not a final release guide.

No release has been published and no issue has been closed or commented on.

Native OpenAI capture is now bound before SDK chunk normalization, with pure stream request preparation before translated capture. Generic native capture preserves canonical wire state associations/partial usage and yields parsed chunks before required sink failure. Optional capture observation runs after assembly/original validation and before the sole terminal. Source cleanup now cancels a dedicated source child context, not the invocation context needed by terminal observation; caller Close still cancels both. Core fixtures cover producer cleanup without deadlock, observer snapshot isolation, final-validation ordering and cancellation without a held status mutex. Real SDK/SSE fixtures cover raw extension fields, actual include-usage request option, stable sequence, single read/dispatch/terminal, pre-/post-dispatch failures and captured original-validation failure after trailing usage. The checkpoint core/shared/OpenAI/tracing race suite and lint passed before the final safe-category/stream-kind additions; those final changes are being verified. Transport bytes/physical attempts remain explicitly unsupported; full capture edge-case coverage and independent audits remain required.

Sync capture checkpoint lint reports zero issues for core/shared adapter/OpenAI, and the cancellation-guard race suite passes for the same scope. The wider four-provider/tracing race suite passed before that final shared guard. Whole-workspace final gates will be rerun after remaining implementation, not reused as final acceptance. Next required capture slice is native pre-normalization SDK chunk observation plus terminal/failure/cancellation lifecycle integration; native capture is not counted as implemented.

Checked synchronous capture is now bound to the existing generic adapter executor. Portable capture intent is part of strict execution wire, while runtime sink/projector binding is cloned but omitted from persistence. Capture preflight runs before Translate; canonical execution snapshots original schemas before lowering. SDK RawJSON preserves exposed provider extension fields; canonical response and terminal record original validation outcome separately. Real OpenAI SDK/local HTTP tests prove stage separation, stable identity/sequence, zero calls on pre-dispatch sink failure, one call and retained known result/outcome/usage on post-dispatch failure, and zero dispatch for unsupported transport/physical/byte/durable coverage. Runtime ports must be rebound after wire restore. Generic capture tests prove original-vs-lowered schema separation and no provider call after cancellation by the sink. Capture diagnostics include conservative stage/accuracy coverage and survive response wire/clone. Wider core/provider/tracing race checkpoint passes; final targeted lint/tests are being rerun after the latest cancellation guard. Native raw chunk/terminal hooks remain mandatory unfinished work and required native capture currently fails closed before dispatch.

Capture core ports now exist in `capture.go`: optional disabled sessions, generated invocation identity, stable serialized sequence, explicit coverage/accuracy negotiation, synchronous bounded projection, caller sink/projector ports, verified durable acknowledgements, secret-safe typed pre/post-dispatch error classification and cloned reports. Default projection omits payload; record metadata always omits endpoint URLs. No backend or queue is introduced. `capture_test.go` covers unsupported required coverage/physical attempts, default omission, advisory failure, payload/report isolation, callback report access, concurrent emissions, false acknowledgements, size/accuracy failures and missing required stages at terminal. Core lint reports zero issues; core/shared-adapter/all-four-provider race tests pass after the final fixes. This is foundation only: generic sync/native adapter hooks, real OpenAI SDK raw chunk capture, execution/wire requirements and report binding remain mandatory, not deferred.

After the Anthropic/OpenAI metadata changes, both complete `make lint` and `make test` commands finished successfully with isolated caches. This includes all modules, examples, the Git registry and generated testdata race tests. This is a verified implementation checkpoint, not final acceptance: remaining requirements and both independent audits are still required.

Anthropic metadata/state checkpoint: ordered thinking/redacted-thinking parts now carry required destination-scoped envelopes. Actual SDK/local HTTP tests cover effective-model parse → response wire → restore → normalized history → execution wire → native request with unchanged signature/data. Canonical reasoning does not expose redacted data. Unknown/malformed payloads, edited signed text, foreign endpoint, expiry, missing state, optionalized state and cache controls fail before dispatch; logging omits opaque values and cannot continue. Text citation JSON retains SDK-unknown fields. Stop reasons now distinguish tool calls/refusal/incomplete; unknown reasons are incomplete. Usage tracks known-zero vs missing/null and derives prompt/total only from all required provider counters. Anthropic module race tests pass. Initial lint found only long fixture lines; formatting corrected. Whole-workspace make lint/test remain running; lint has passed generator, core, tracing and all four provider modules, with examples still in progress. The final OpenAI guard also passes this lint checkpoint. Final audits, reasoning configuration, capture/cache and conditional remote/resource decisions remain outstanding.

OpenAI Chat Completions metadata checkpoint: sync parse preserves refusal (including explicitly present empty string), incomplete empty responses and known-zero usage. Full citation JSON, including fields unknown to the SDK, survives response wire and clone isolation. Single-choice/index-zero enforcement and annotation association rejection prevent silent loss. Native refusal deltas retain exact part annotations and drive refusal outcome even with a separate stop finish and trailing usage. Checked native fixture proves valid JSON + refusal yields `ErrResponseOutcome`, not schema mismatch, retains usage, emits one failed terminal and dispatches once. `go test -race -timeout=60s . ./adapter ./adapter/openai ./ext/otelprompty` and combined lint pass with isolated caches. The subsequent empty tool-call finish regression also passes provider race tests; lint must be rerun after that final guard/test change. This is partial PRM-003 progress, not final completeness credit; Anthropic reasoning/state/outcomes and other mandatory contracts remain.
