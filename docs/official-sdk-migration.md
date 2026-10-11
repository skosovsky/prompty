# Official SDK migration contract

Status: implementation complete; verification/review evidence tracked below. Baseline: 1538c45414c31a1e1283907a1295327d6f22b6f2.

The optional adapter owns canonical projection; the official SDK owns requests,
provider decoding and streams. Core contracts, BYOT, checked admission, original
schema validation, host tool dispatch and host lifecycle remain independent.
No alternate provider transport/parser, request rewrite, fork or private SDK API
is permitted. A public RawJSON/field metadata projection is a domain mapping
extension point, not an independent response acquisition path.

## Operation inventory and target

Every row includes sync and explicit buffered mode where applicable. Native mode
is a separate SDK iterator/callback; adapters never replace it with buffering.
SDK pins selected and retained after fixture evaluation: Ollama v0.40.0, genai v1.72.0,
OpenAI v3.73.0, Anthropic v1.78.0. Published version inventory checked 2026-10-11
with go list -m -versions; newer available releases are v0.40.3, v1.73.0,
v3.76.0 and v1.80.0 respectively. Pins are deliberate, not a latest claim.

| Owner / operation | Before / helper classification | Target public SDK API | Availability / migration consequence | Evidence gate |
| --- | --- | --- | --- | --- |
| Ollama Chat sync/native | HTTPChatClient + bounded NDJSON: alternate transport/parser | api.Client.Chat | Available; remove HTTPChatClient; SDK numeric/scanner semantics accepted explicitly | real SDK sync/native/error/cancel fixtures |
| Ollama history/tools/schema | exact argument constructor, canonical mapper: domain mapping | api.NewToolCallFunctionArguments, ChatRequest | Keep outbound canonical JSON numbers; no inbound lossless promise | continuation/schema fixtures |
| Gemini GenerateContent MLDev/Vertex sync/native | responseSource/streamSource: alternate parser; isolated client replaces transport | Models.GenerateContent/GenerateContentStream | Available; SDK representation only; no raw signed-source attestation or repaired numeric precision | multimodal/state/partial stream fixtures |
| Gemini native schema MLDev/Vertex | preserveNativeSchemaNumbers/exactGenerationRequest: request rewrite | ResponseJsonSchema / FunctionDeclaration.ParametersJsonSchema / Contents | Preserve public native schema port; accept converter representation limits explicitly | serialization characterization |
| Gemini batches inline/file create/get/cancel/results | batchSource: alternate body parsing/identity recovery | Batches.Create/Get/Cancel, BatchJob.Dest | Available subject to backend; SDK returned identity only; no raw-body recovery | batch outcome/resource fixtures |
| Gemini file batch upload/download | host BatchFileTransport + SDK local request encoder + JSONL item mapping: host port/public injection/domain mapping | Batches.Create with BatchJobSource.FileName; host-owned file port (no newly invented Files service) | Existing host file operations retained; output JSONL is resource content mapping, never alternate GenerateContent acquisition | batch_file_test.go / ExampleNewFileBatches |
| Gemini managed cache create/get/update/delete | cache capture + response observer | Caches.Create/Get/Update/Delete | Keep public SDK calls; SDK representation capture only | resource/cancellation fixtures |
| OpenAI Chat sync/native | SDK Chat + RawJSON canonical projection: public extension point | Chat.Completions.New/NewStreaming | Keep available blocks, ordering, arguments/refusal/usage | chat fixtures |
| OpenAI Responses create/get/cancel/previous/native | responses_source body/identity parser: alternate parser; SSE status observer: generic telemetry | Responses.New/Get/Cancel/NewStreaming + RawJSON | Remove body reparsing; retain public event unions/raw metadata; decode failure may have unknown ID | response/event/unknown outcome fixtures |
| OpenAI batch submit/get/cancel/results/files | SDK batch/file calls, JSONL item projection: domain mapping | Batches.New/Get/Cancel, Files.New/Get/Content | Preserve Chat batch scope; no new Responses batch | batch/file fixtures |
| Anthropic Messages sync/native + hosted | response_source/native_stream_source/hosted_source: inspect classifications per function | Messages.New/NewStreaming + event/block RawJSON/JSON metadata | Keep public field validation and canonical hosted projection; no independent Messages/SSE acquisition | ordered hosted/thinking/tool fixtures |
| Anthropic batch submit/get/cancel/results | batchSource body identity recovery: alternate parser | Messages.Batches.New/Get/Cancel/ResultsStreaming.Next/Current/Err | Remove identity recovery; SDK returned identity only | batch acceptance/results fixtures |
| Shared numeric_source | exact canonical numeric validation: domain mapping | independent of SDK | Remove provider acquisition callers only; retain canonical schema/tool validation | root numeric regression |

## Semantic boundaries

Host owns borrowed SDK/HTTP clients, auth, endpoints, transport, timeout and
cancellation. No global client, host mutation, borrowed Close or blanket panic
recovery. Public SDK options remain usable. Adapter code never retries an unknown
accepted mutation and never invokes tools while parsing. Retry behavior is
verified through network attempt counters, not an inferred guarantee.

Positive representation-limit failure retains known usage as a failed usage-only partial result, without content/state. Known SDK cache names and valid counts survive unrelated expiry/model/projection failure; unknown handles may persist an unknown expiry for explicit inspection/deletion, while reuse still fails. Successful genai HTTP status remains unknown; rejection metadata comes only from public APIError.Code.

Canonical results retain SDK-available content in order, IDs, model, reasoning,
signatures, hosted outputs, finish/outcome and usage. Missing evidence remains
unknown. Partial results never become success; provisional tool arguments never
become executable completion. EOF without terminal is incomplete. Consumer abort
stops dispatch/consumption; it does not cancel an already accepted remote effect.
Unknown variants fail explicitly. Privacy-safe adapter errors must not leak
provider bodies or credentials; host transport panics are not swallowed.

Checked, raw SDK mapping and buffered/native paths keep distinct guarantees.
Unsupported backend fields fail before IO when identifiable. An unavailable
mandatory operation requires a user scope decision, never a fallback.

## Dependency limitation register and upgrade gate

All entries below have executable local reproduction on the pinned published version. No forks, private imports, patched vendor or copied SDK implementation are used. “Characterization passes” means the dependency behaves as recorded, never that it was fixed.

| ID | Version / operation and minimal input | Observed behavior / accepted semantics | Characterization evidence / upstream source / upgrade trigger |
| --- | --- | --- | --- |
| SDK-OLLAMA-NUM | ollama v0.40.0 Chat, tool args `{ "id":9007199254740993 }` | SDK argument map contains float64 rounded to 9007199254740992. Checked sync/native reject any unverified numeric arguments; do not dispatch a handler. Preserve nonzero usage and prior text/thinking. Caller-owned json.Number arguments/outbound canonical numbers remain separate. | [precision fixtures](../adapter/ollama/precision_test.go), [native fixture](../adapter/ollama/stream_test.go); [SDK args](https://github.com/ollama/ollama/blob/v0.40.0/api/types.go). Recheck decoding, exact tool admission and usage each upgrade. No matching upstream issue found. |
| SDK-OLLAMA-FRAME | ollama v0.40.0 Chat: NDJSON record >8 MiB, malformed line `private-provider-body`, truncated argument array | SDK scanner has 8 MiB limit and exposes scanner errors; malformed line error can contain body. Adapter sanitizes SDK errors, cannot salvage content inside an undecodable frame. Prior callbacks survive; missing terminal fails. No custom frame bound/decoder exists. | [frame/privacy characterization](../adapter/ollama/sdk_limitations_test.go), [partial fixtures](../adapter/ollama/multiframe_failure_test.go); [SDK stream](https://github.com/ollama/ollama/blob/v0.40.0/api/client.go). Recheck limits, errors, cancellation/consumer stop and body closure. No matching upstream issue found. |
| SDK-GENAI-NUM | genai v1.72.0 MLDev/Vertex generation/state, inline/file/cache contents and MLDev native schema; `9007199254740993`, `1.234567890123456789` | Dynamic numbers pass through float64; PartialArg.NumberValue is float64. Tool args cannot prove original fidelity and fail. Signed caller state that changes numerically fails before IO. Public native schema ports remain supported but numeric constraints can round: 9007199254740992 / 1.2345678901234567. Final validation still uses the original exact schema. No HTTP repair. | [native numeric serialization](../adapter/gemini/native_schema_test.go), [state admission](../adapter/gemini/state_sdk_precision_test.go), [ingress](../adapter/gemini/response_source_test.go), [inline encoding](../adapter/gemini/batch_test.go), [cache encoding](../adapter/gemini/cache_resources_test.go), [partial numeric usage](../adapter/gemini/usage_failure_test.go); [SDK converter](https://github.com/googleapis/go-genai/blob/v1.72.0/models.go), [types](https://github.com/googleapis/go-genai/blob/v1.72.0/types.go). Recheck both backends, content and schema serialization after every upgrade. No matching upstream issue found. |
| SDK-GENAI-SOURCE | genai v1.72.0 generation/cache: duplicate/case-aliased JSON keys, unknown field, scalar usage zero, zero confidence, empty args | Only normalized DTO data is available. Unknown fields/source spelling/duplicates and presence of scalar zero can be lost. Signed envelopes/provenance retain SDK part representations, not original-wire attestation. Omitted counters remain unknown; citationSources becomes citations, confidence has float32 precision. Caller-supplied HTTPResponse.Body is ignored. WithResponsePayloadLimit checks serialized DTO after SDK allocation, never wire/allocation bounds. | [normalization](../adapter/gemini/response_source_bounds_test.go), [part association](../adapter/gemini/response_source_policy_test.go), [metadata](../adapter/gemini/message_metadata_test.go), [usage](../adapter/gemini/usage_test.go), [cache presence](../adapter/gemini/cache_resources_test.go); [SDK decoder](https://github.com/googleapis/go-genai/blob/v1.72.0/api_client.go), [types](https://github.com/googleapis/go-genai/blob/v1.72.0/types.go). Recheck DTO presence/raw API changes each upgrade. No matching upstream issue found. |
| SDK-GENAI-STREAM | genai v1.72.0 malformed generation `candidates:"private"` / inline response converter shape | SDK conversion can panic before returning a DTO. SDK owns stream framing (256 MiB scanner cap); scanner errors are yielded, not hidden. Adapter has no blanket panic catch or alternate parser, so cannot recover an undecoded item prefix. No privacy guarantee for panic values is claimed. | [malformed native fixtures](../adapter/gemini/stream_test.go), [inline converter panic](../adapter/gemini/batch_test.go); [SDK API/converter](https://github.com/googleapis/go-genai/blob/v1.72.0/api_client.go). Recheck panics, EOF, late usage, scanner errors and cleanup. No matching upstream issue found. |
| SDK-GENAI-LOG | genai v1.72.0 stream: body read returns `private-host-read-error` | SDK both yields error and writes it to the process standard logger. Adapter sanitizes returned error; it neither reconfigures global logging nor promises prevention of SDK logs. Host must account for SDK logging policy. | [logger/error characterization](../adapter/gemini/sdk_limitations_test.go); [SDK iterator](https://github.com/googleapis/go-genai/blob/v1.72.0/api_client.go). Recheck logger removal/configurability after upgrades. No matching upstream issue found. |
| SDK-GENAI-BATCH-DEST | genai v1.72.0 MLDev Get `{name,metadata:{state},response:{responsesFile}}` | SDK drops REST response destination; supported `metadata.output.responsesFile` returns Dest.FileName. Retain identity/state, output unknown/unavailable if missing; no REST destination fallback. File/inline result operations remain exposed with this limitation. | [two-form characterization](../adapter/gemini/sdk_limitations_test.go), [file lifecycle](../adapter/gemini/batch_file_test.go); [SDK batches converter](https://github.com/googleapis/go-genai/blob/v1.72.0/batches.go). Recheck actual REST form each upgrade. No matching upstream issue found. |
| SDK-GENAI-HOOK | genai v1.72.0 ClientConfig/NewClient with HTTPOptions.ExtrasRequestProvider function | Public configuration copy can discard function-valued client hook. Adapter borrows host client directly, keeps per-call public options, and does not install a repair hook. Native schema numeric fidelity is not restored by such a hook. | [client hook characterization](../adapter/gemini/native_schema_test.go); [SDK config](https://github.com/googleapis/go-genai/blob/v1.72.0/client.go). Recheck client/per-call hook behavior each upgrade. No matching upstream issue found. |
| SDK-MUTATION-ID | openai-go v3.73.0 Responses/Batch, anthropic-sdk-go v1.78.0 Batch, genai v1.72.0 cache/create: incomplete JSON with valid ID prefix or cache counter 2147483648 | If SDK returns no DTO, ID recovery is unavailable and acceptance remains unknown. A returned SDK RawJSON/JSON-valid ID is still retained even if later domain validation fails. No automatic replay. | [OpenAI malformed response](../adapter/openai/responses_test.go), [Anthropic malformed batch](../adapter/anthropic/batch_test.go), [Gemini cache decode](../adapter/gemini/cache_resources_test.go), [known OpenAI ID](../adapter/openai/batch_test.go); [OpenAI public response](https://github.com/openai/openai-go/blob/v3.73.0/responses/response.go), [Anthropic batch](https://github.com/anthropics/anthropic-sdk-go/blob/v1.78.0/messagebatch.go), [genai cache](https://github.com/googleapis/go-genai/blob/v1.72.0/caches.go). Recheck returned identity and actual attempt counts each upgrade. No matching upstream issue found. |

Issue searches on 2026-10-11 included repository issues + scanner errors, numeric precision/9007199254740993, responsesFile, ExtrasRequestProvider and malformed identity. No matching version-specific issue/PR was established; the versioned implementation links above and local tests are the evidence. Older genai stream/timeout reports were not counted as current defects. No external issue was created.

The former Anthropic scanner-error workaround is **obsolete**: v1.78.0 public JSONL Next/Err exposes scanner failures and has a 32 MiB line buffer. [Large-line/read-error tests](../adapter/anthropic/batch_test.go) verify actual SDK behavior. MaxLineBytes applies after SDK record decoding; generic MaxBytes read policy and EOF/Close telemetry remain separate. [Official iterator source](https://github.com/anthropics/anthropic-sdk-go/blob/v1.78.0/packages/jsonl/jsonl.go). This working SDK behavior is not listed as an unfixed defect.

Upgrade procedure: select an official published version, replay characterization
and canonical regression fixtures, inspect changed public unions/fields and both
Gemini backends, verify serialization and actual retry/cancel attempts. If behavior
changes, revise the register and expected result; replace fixed characterizations
with corrected-behavior regressions. Run all modules, lint, offline integration/e2e
and GOWORK=off consumers. Never skip failing characterization indefinitely.

## Helper classification and cleanup evidence

| Helpers / paths | Classification / disposition |
| --- | --- |
| Ollama chat_http.go / chat_frame.go, NewHTTPChatClient | Alternate provider transport/parser and constructor: deleted; api.Client.Chat injected. |
| Gemini stream_source.go / responseSourceTransport / isolatedResponseClient | Alternate response/SSE acquisition: deleted. response_source.go now stores SDK DTO projection only; WithClient borrows the same host client. |
| Gemini exact_contents.go / preserveNativeSchemaNumbers / batchExactContents / cache exact options | Request rewrites: deleted; public SDK converters serialize requests unchanged. |
| Gemini native_schema.go / state codecs / partial_stream.go / message_metadata.go | Domain schema admission, scoped canonical state, incremental tool mapping and DTO metadata validation retained. No independent provider acquisition. |
| Gemini batch_source.go / cache_resources.go | Typed SDK job/cache projection; raw REST identity/destination recovery removed. |
| Gemini batch_file.go local batchRequestEncoder | Public HTTP-client injection records the official SDK's serialization in an isolated **no-egress** client for existing JSONL input files. It implements no API/framing/parser, never rewrites bytes, never replaces host transport for a provider call, and never sends a generation request. Host BatchFileTransport still owns upload/download. |
| OpenAI responses_source.go / batch helpers | Metadata-only public middleware + returned RawJSON projection; body reread/replacement and malformed-body identity scanner deleted. responses_sse_source.go retains generic status telemetry; events decoded by SDK. |
| Anthropic response_source.go / native_stream_source.go / hosted_source.go | Public RawJSON/JSON metadata domain validation for ordered content/usage/state/hosted unions retained. No Messages/SSE transport implementation. |
| Anthropic batch_source.go / batch_results.go | SDK-returned metadata/raw projection; alternate identity parser and JSONL line reader deleted. Public iterator owns records; generic byte/EOF/Close policy does not decode payloads. |
| OpenAI/Gemini batch output file JSONL projection | Resource-file domain mapping from SDK/host file body, necessary for canonical correlation/results. Does not replace SDK response/stream decoding. |
| adapter/numeric_source.go | SDK-independent recursive numeric evidence validation; retained callers in Ollama/Gemini mapping/state. No provider wire decoder. |

## Verification and reviews

All verification is offline. Runnable adapter examples and `scripts/verify-sdk-consumer.sh` use genuine official SDK clients against local fixtures. The independent consumer has its own temporary go.mod, GOWORK=off and GOPROXY=off; local replace directives select only the current optional prompty modules, never patched SDKs. Existing official dependency pins are preserved.

Review regression evidence includes [usage/rejection/cache fixtures](../adapter/gemini/sdk_review_regression_test.go) and [portable unknown-expiry core contract](../cache_resource_test.go). Known genai rejection uses public APIError.Code; successful job/cache HTTP status remains unknown.

Final commands, exit codes, versions, tests present/absent by profile, snapshot manifest and independent A/B verdicts are saved under `.cursor/tasks/evidence/official-provider-sdk-migration/`. Review verdicts apply only to the manifest snapshot; new substantive edits invalidate them. Paid live/account/model behavior, release/publish and neighbor apps are not verified or changed.
