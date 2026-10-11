# Gemini adapter for prompty

## Explicit cached-content lifecycle

`NewCacheResources(accountBinding, opts...)` supplies caller-driven Create/Get/
UpdateExpiry/Delete through the injected SDK. Declare independent operation
capabilities in the effective endpoint/model profile. Create reuses canonical
PromptExecution immutable inputs and requires exactly one explicit TTL/expiry;
generation-only options, output schemas and cache hints are not repurposed.

Persist `EncodeCacheResourceHandle` output and restore it with
`DecodeCacheResourceHandle`; host owns storage, access and cleanup. For generation,
set `execution.CacheResource`, bind `WithCacheAccountBinding`, and declare
CapabilityCacheReuse. Foreign/expired/deleted/unknown handles fail before dispatch
without upload/recache.
Expiry is checked again immediately before logical dispatch, after capture or
native consumer pauses. Create/update absolute expiry receives the same guard.
No remote lease is promised; expiration during an HTTP call remains a provider
outcome.
Base reuse rejects conflicting supplied system/tools; cached configuration is
not silently overwritten. Unknown mutation outcome is
`ErrCacheResourceUnknown`, retaining identity where available; never blindly retry.
Resource TokenCount distinguishes known zero from absent and is not billed
generation usage. Generation cached usage still uses normal presence semantics.
Lifecycle capture uses the common CaptureSession. Create takes execution capture
intent/binding; Get/UpdateExpiry/Delete accept optional CacheResourceOperationOptions.
Default projection omits payload; an explicit host projector owns redaction.
Canonical input/SDK parameters/original provider JSON/result/terminal are separate
semantic stages, not HTTP bytes or physical-attempt coverage. Required early
failure stops dispatch; post-dispatch failure retains known resource result and
does not authorize retry. Offline cache lifecycle/fault fixtures are linked in the current capability matrix.
Executable ExampleNewCacheResources
uses a local SDK fixture, not live model conformance or a cache-hit guarantee.

Maps prompty's `PromptExecution` to Gemini GenerateContent and returns a full `prompty.Response` with ordered parts, thought signatures, annotations, usage presence and outcome.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/gemini
```

## Configuration

- **API key / client:** create and inject a `google.golang.org/genai` client with `WithClient`. Checked `adapter.NewClient` performs preflight, translation, SDK execution and parsing. Raw SDK primitives remain available without checked guarantees.
- **Default model:** `New()` uses `gemini-2.0-flash` as an example convenience, not capability evidence; select the deployed model explicitly with `WithModel`. Override with `WithModel(...)`, or set per execution via `exec.ModelOptions.Model`.
- **Provider settings:** use `PromptExecution.ModelOptions.ProviderSettings` for provider-specific options (e.g. `gemini_search_grounding` for grounding).

## Implemented mappings

ForcedTool maps to native ANY with a single allowed function name, never an injected system instruction. Checked use needs a caller-owned `CapabilityForcedToolSelection` profile in addition to `CapabilityTools`; request mapping is not live model certification. Ordinary tools and ID-less name correlation remain separate guarantees.

Use caller-owned `WithProfiles(endpoint, profiles...)` for the actual injected transport/effective model. Required unknown support fails before dispatch; SDK mappings are not a universal model support catalog. See the [migration guide](../../docs/migration.md).

`WithClient` alone no longer implies the default endpoint. Checked execution requires `WithProfiles(actualEndpoint)` even without capability profiles; configure the SDK URL from the same caller-owned value. Missing/blank binding fails before translation or dispatch. An execution override cannot replace this binding, and the adapter does not inspect SDK-private configuration.

- **Types:** `Translate` returns `*gemini.Request` (Model, Contents and Config); `ParseResponse(raw)` expects `*genai.GenerateContentResponse`. `StreamEvents` uses native GenerateContent SSE with no synchronous fallback; choose `StreamBuffered` explicitly for a synchronous call through the checked lifecycle. The old `ParseStreamChunk` API is removed: it lost ordered/signed parts and mislabeled complete tool snapshots as JSON fragments.
- **Native stream:** caller profiles must explicitly declare native support for the endpoint/model. `StreamEventsForProfile` preserves full checked state scope. Source JSON is bounded per SSE line (including framing) by `WithResponsePayloadLimit`; exact arguments use Number before SDK conversion. Complete calls use `Args` snapshots, not `ArgsChunk`. Late usage and message metadata after candidate finish are accepted; read failures and missing finish fail with retained partial content. Break/cancel closes the SDK body and does not repeat completion. Signed incremental calls preserve original parts through explicit state updates, source-order provenance and Vertex continuation, including interleaving. The offline executable example is in `stream_example_test.go`.

Signed partial calls retain unsigned and signed original fragments in the separate
`gemini.partial_parts.json` codec. Arguments are only a canonical projection; the
continuation request sends original parts, not a merged functionCall with a moved
signature. Completed matching content, required state and Vertex backend are
checked before dispatch. Canceled/incomplete sequences, edited content and SDK
numeric loss fail closed. Ordered message source is bounded to 4 MiB. Preserve
`gemini.source_part` annotations too: they carry ordinals and unsigned original
parts, never signature payloads. Restore validates complete unique source coverage
and canonical projections before sending interleaved original parts in ordinal
order. Missing/duplicate provenance fails, with no repair from canonical order.
Unsigned empty args may follow SDK omission semantics; signed source remains
strict. Local SDK fixtures do not certify live models; current implemented contracts and evidence are linked in the capability matrix.
- **Candidate metadata:** read `Response.MessageAnnotations` / `ResponseChunk.MessageAnnotations`, not a fabricated annotation on content part zero. `candidate_citation_metadata` and `candidate_grounding_metadata` preserve exact observed JSON (including REST `citationSources`), with ordered updates in streams. Grounding chunk indices reference accumulated provider chunks across updates; byte segments and provider part indices are not canonical content indices. The adapter validates chunk references and segment shape without inventing text bindings. History/wire/capture retain these values; the host owns redaction and rendering. SDK-only parsing without observed source preserves SDK DTO semantics, not unobserved source bytes.
- **Incremental arguments:** require `gemini.CapabilityIncrementalToolArguments` (`gemini.partial_args`) and supported endpoint/model profile knowledge, alongside tools/native support. This opt-in sends `streamFunctionCallArguments=true` through the Vertex SDK mapper. `stream_function_call_arguments` is an optional boolean provider control; true requires that explicit capability, and false cannot contradict it. The installed MLDev mapper cannot send this field and rejects before I/O; sync/buffered requests cannot select it. Provisional calls have empty `Args`, with exact path/value progress in `gemini.partial_function_call` annotations; these are not JSON `ArgsChunk` fragments. A provider close yields one complete snapshot for common correlation/schema validation. ID-less calls require a unique open call, while provider-ID calls may interleave. No handler is run by the streaming parser. See `partial_example_test.go` for a runnable offline opt-in.
- **Original signed source:** `WithClient` snapshots the public SDK configuration and copies its HTTP client to observe the original response before SDK float decoding, without mutating caller transport/policy. Observation is invocation-local and bounded (default 4 MiB); `WithResponsePayloadLimit(bytes)` selects a positive bound. Rebind after client reconfiguration. Signed parts use original JSON Number values; unavailable/truncated/ambiguous source fails closed. Raw signed float DTOs without original Body cannot establish fidelity. Outbound SDK representability remains a separate gate; no HTTP capture coverage or replay guarantee is implied.
- **Response-error privacy:** after completely observed HTTP 2xx EOF, SDK decode/conversion errors and malformed-shape conversion panics become static `ErrProviderState`, without raw body/signature in the error chain. Signature detection is not required for privacy. Cancellation remains identifiable separately; incomplete read and transport/host errors retain their causes. No replay or blanket recovery of caller transport panics before this response boundary.
- **Messages:** system, user, assistant; tools; media. URL and inline bytes are mapped through Gemini URI/inline parts; no need to call `exec.ResolvedMedia` for URL media.
- **Multimodal tool results:** declare `gemini.CapabilityFunctionResultMedia` for the actual endpoint/model and the nested image MIME types in `InputMIME`. Supported result images are PNG/JPEG/WebP. Inline caller bytes work in both GenerateContent backends; URI results require Vertex and a `gs://bucket/object` without query/fragment/userinfo. Gemini API accepts inline only; unsupported URI/MIME/media shapes fail before dispatch, with no fetch/upload. For mixed results, `response.output` (or `response.error`) is an ordered array of text strings and native `$ref` objects; nested images retain order, MIME and bytes/URI using matching unique `displayName`s. ToolCallID/name are unchanged; text-only results keep native output/error strings. Execution wire keeps the original canonical Content. This follows [Gemini API](https://ai.google.dev/gemini-api/docs/generate-content/function-calling?hl=en) and [Vertex](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/multimodal/function-calling) forms without inferring model support. Run `go test -run ExampleAdapter_multimodalResult` for an offline SDK fixture and rejected unsupported MIME.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Schema/state:** the strict typed-schema subset rejects unmapped fields instead of dropping them. Signed parts retain ordering and scoped continuation through explicit wire and `AssistantMessage`; default logging is not reusable continuation. Reasoning budget/level/visibility need a profile options schema; missing counters remain unknown rather than known zero.
- **Cache control:** checked execution reports block markers as unsupported and implicit-prefix knowledge from the effective caller-owned profile; it never fabricates a hit or request field. Raw Translate rejects required unmapped intent. Explicit cache resource lifecycle is separate and not supplied by request controls.
- **Helpers:** `prompty.StrictTextFromParts`, `prompty.JoinAdapterTextParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/gemini) for the full API.

## Native inline batch

`NewBatches(accountBinding, opts...)` implements `prompty.BatchManager` through
the actual Developer API SDK batch create/get/cancel operations. Declare the
independent batch capabilities in the host-owned endpoint/model profile. Native
inline requests must use one model and unique caller item keys; each item supplies
an attempt ID and may carry original/parent lineage. Preparation reuses generation
schema, state, cache and options checks before creation. Cache expiry is checked
again after capture. The SDK request hook restores exact checked content numbers
after normalization; no handler or implicit retry executes.

Persist the portable scoped handle and original executions. Supply those executions
as Results bindings; changed execution digests or profiles fail before retrieval.
Results prepares original local schema/tool-selection validation, not a new
generation request: original cache/state leases may have expired, and generation
capabilities need not remain available. Do not modify the original inputs to
remove these fields; digest matching remains exact. Submit still enforces live
leases and all generation constraints. Result schema reports describe local
original validation, not a new provider lowering/native constraint guarantee.
Results uses one explicit Get and original bounded JSON to preserve per-item source,
refusal/incomplete/error/usage and metadata-key correlation, including out-of-order,
duplicate, missing and unexpected keys. Download completeness is independent of
correlation completeness and individual success. A malformed item keeps earlier
source-proven items, even if SDK conversion failed on the entire job response.

Cancel acknowledgement is not terminal cancellation; inspect again explicitly.
Unknown create/cancel never authorizes automatic retry. A scoped handle contains
no credentials and is not a retention/durability guarantee. The host owns polling,
storage, cleanup, egress and access decisions. Semantic operation capture uses
common redaction and coverage; transport bytes/physical attempts are not promised.

The inline input bound is 20 MB; original response observation defaults to 4 MiB
and Results allows explicit MaxBytes/MaxLineBytes. Local protocol/error/schema/cancel fixtures cover the implemented ports; task37 audit checkpoints retain their original scope. Vertex batches, embeddings and file results are not
advertised by this inline path. `ExampleNewBatches` is an executable local SDK
fixture, not evidence of live account/model support. Protocol reference:
[Gemini Batch API](https://ai.google.dev/gemini-api/docs/batch-api).

For explicit file-backed submission use `NewFileBatches` with a host-owned
`prompty.BatchFileTransport` whose provider/endpoint/account binding matches the
adapter. All items are checked and SDK-encoded locally before one upload; encoding
does not send a generation request. Input JSONL has caller keys and native request
fields. The port owns upload/download authentication and cleanup, not prompty.
There is no inline fallback, file store or cleanup worker. Inspect `Resources`
and `SubmissionAttempted` on failure: a known input file is not a created job.

Inspect explicitly to obtain the output reference, persist the scoped handle,
then call Results with original input bindings. Downloads preserve original JSON
and correlate by key. HTTP/read/close/size failures keep accepted prefix and mark
download incomplete; missing/duplicate/unexpected keys are independently visible.
Bodies close once, even on failure. Bounds default to 64 MiB total and 4 MiB per
line, configurable explicitly; input cannot exceed the provider's file limit.
Required original-source capture needs a file-port accuracy declaration.
`ExampleNewFileBatches` is a runnable local fixture, not live conformance.

The [unified capability matrix](../../docs/provider-capability-matrix.md) records pinned SDKs, operation/backend limits, checked/raw guarantees and linked offline evidence. The [clear-break migration](../../docs/migration.md) covers accepted tasks38–43; historical task37 evidence is not rewritten as current certification.

## Opt-in native JSON Schema

`gemini.New(gemini.WithNativeJSONSchema(), gemini.WithClient(sdk),
gemini.WithModel(model), gemini.WithProfiles(endpoint, profile))` selects native
schema transport. The default remains the strict typed output schema mapper.
Use checked `adapter.NewClient` to validate results against the **original** schema.
Raw `Translate` enforces the same keyword admission, but does not validate responses.

The contract is GenerateContent on the Gemini Developer API (`v1beta`), for
models whose caller-owned endpoint/model profile explicitly supports structured
output and/or function calling (for example Gemini 2.5 Flash/Pro). The option is
not endpoint or model capability evidence. Bind the actual SDK endpoint with
`WithProfiles`; capabilities and combination restrictions still use the existing
preflight, including per-execution overrides. Vertex and OpenAI-compatible
endpoints are not certified by these offline fixtures.

Verified against `google.golang.org/genai v1.72.0`, its
`GenerateContentConfig.ResponseJsonSchema` / `FunctionDeclaration.ParametersJsonSchema`
contracts, and the official [GenerateContent API](https://ai.google.dev/api/generate-content)
and [structured output guide](https://ai.google.dev/gemini-api/docs/structured-output)
(2026-10-11). No live provider compatibility claim is made.

The initial, deliberately narrow subset is schema objects containing `type`
(single string: object, array, string, number, integer, boolean, null),
`title`/`description` (strings), `properties` (schema objects), `required` (string
array), `items` (schema object), `minimum`/`maximum` (JSON numbers),
`additionalProperties` (boolean or schema object), and `enum` (nonempty array of
strings/numbers). Tool roots must have `type: object`. All other keywords are
rejected, including **all references** (local, missing, cyclic, external), `$defs`,
resource identities, composition, format, ordering, and tuple keywords. This
subset covers the Kosmify exact-constraint consumer fixture; it is not full JSON
Schema support. `oneOf` is rejected because the provider interprets it as `anyOf`.
Unknown semantics are never stripped or sent through typed fallback.

Admission errors use `SchemaFidelityError` with sorted traversal and escaped JSON
pointers; errors omit schema values. Checked core compiles the original schema
first, so malformed source schemas and unresolved references can instead return
its existing schema compilation error, still before I/O. Direct translation and
`MapSchema` reject references with a fidelity error without resolving them.

Output sends `application/json` and only `ResponseJsonSchema`; tools send only
`ParametersJsonSchema`. Decimal and large integer lexemes survive SDK HTTP
serialization: a request-local extras hook restores admitted schema-bearing fields
after genai's float64 DTO conversion. Existing request options/extras are copied
and composed; retry, budgets, telemetry, names, and tool choice are not configured
by native mode. Response validation remains a checked-core responsibility.
