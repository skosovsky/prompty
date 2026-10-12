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
Known SDK resource name/count survive downstream representation/expiry validation failures. Unknown handles can persist without expiry for explicit Get/Delete reconciliation; generation reuse remains rejected. Successful resource HTTP status is not exposed by these SDK DTOs. Resource TokenCount is unknown for scalar zero without SDK presence evidence and is not billed
generation usage. Generation cached usage still uses normal presence semantics.
Lifecycle capture uses the common CaptureSession. Create takes execution capture
intent/binding; Get/UpdateExpiry/Delete accept optional CacheResourceOperationOptions.
Default projection omits payload; an explicit host projector owns redaction.
Canonical input/SDK parameters/SDK representation/result/terminal are separate
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
- **Native stream:** caller profiles must declare endpoint/model native support. `StreamEventsForProfile` keeps checked state scope. The official SDK owns SSE framing and decoding. `WithResponsePayloadLimit` bounds the serialized SDK representation after decoding, not wire bytes or SDK allocation. Complete calls use `Args` snapshots; late usage/metadata remain available. Missing finish and SDK-reported read errors fail with retained partial content; the SDK also logs scanner failures (SDK-GENAI-LOG). Break/cancel stops SDK consumption without redispatch. Run `go test -run ExampleAdapter_StreamEvents` for the offline example.
- **Signed continuation:** `gemini.partial_parts.json` and `gemini.source_part` retain ordered SDK part representations, never an attestation of original provider bytes. Explicit state, scope, matching content and complete provenance remain required. Finite numeric SDK arguments pass checked mapping and ordinary persistence/continuation; caller/persisted exact signed numbers that the SDK would round still fail before dispatch as ErrProviderState. Do not normalize saved protected state; obtain fresh state using SDK-representable domain values. Use string schemas for exact IDs/decimals; post-decode precision cannot be recovered. Fixture signatures prove transfer, not cryptographic or live-provider acceptance. Empty fields, duplicate keys, case aliases and unknown fields can already have been normalized by the SDK. No raw-body recovery is performed.
- **Candidate metadata:** message-level citation/grounding annotations retain ordered SDK DTO metadata. REST citationSources is mapped by the SDK to citations; omitted fields and float32 confidence precision cannot be recovered. Grounding references are validated without inventing canonical text bindings. Host owns rendering and redaction.
- **Incremental arguments:** require `gemini.CapabilityIncrementalToolArguments`, tools/native support and Vertex. MLDev and sync/buffered selection fail before I/O. Path/value annotations are provisional; numeric PartialArg.NumberValue is provisional SDK float64 representation, including explicit pointer zero; it cannot establish original source fidelity. String/bool/enum-null progress and interleaved provider IDs retain their mapping. Complete snapshots alone enter normal tool/schema validation; no handler runs during parsing. Run `go test -run ExampleAdapter_StreamEvents_incrementalArguments` for the offline opt-in.
- **Host ownership and privacy:** `WithClient` borrows the supplied client directly. Per-operation public RetryOptions requests one attempt without mutating host configuration. SDK-returned errors are projected to static adapter errors while cancellation remains identifiable. SDK converter panics propagate; no blanket recovery is installed. The pinned SDK logs scanner errors through the process logger, which can include arbitrary host error text; this is an accepted dependency limitation, not a privacy guarantee. See the [versioned limitation register](../../docs/official-sdk-migration.md).
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
again after capture. The official SDK converter owns serialization; no request rewrite, handler or implicit retry executes.

Persist the portable scoped handle and original executions. Supply those executions
as Results bindings; changed execution digests or profiles fail before retrieval.
Results prepares original local schema/tool-selection validation, not a new
generation request: original cache/state leases may have expired, and generation
capabilities need not remain available. Do not modify the original inputs to
remove these fields; digest matching remains exact. Submit still enforces live
leases and all generation constraints. Result schema reports describe local
original validation, not a new provider lowering/native constraint guarantee.
Results uses one explicit SDK Get and its returned DTOs to preserve per-item data,
refusal/incomplete/error/usage and metadata-key correlation, including out-of-order,
duplicate, missing and unexpected keys. Download completeness is independent of
correlation completeness and individual success. A malformed mapped item keeps earlier returned SDK items. If the SDK converter fails or panics before returning the job, no source-prefix recovery is available.

Cancel acknowledgement is not terminal cancellation; inspect again explicitly.
Unknown create/cancel never authorizes automatic retry. A scoped handle contains
no credentials and is not a retention/durability guarantee. The host owns polling,
storage, cleanup, egress and access decisions. Semantic operation capture uses
common redaction and coverage; transport bytes/physical attempts are not promised.

The inline input bound is 20 MB; serialized SDK response limit defaults to 4 MiB
and Results allows explicit MaxBytes/MaxLineBytes. Local protocol/error/schema/cancel fixtures cover the implemented ports. Vertex batches, embeddings and file results are not
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

The [unified capability matrix](../../docs/provider-capability-matrix.md) records pinned SDKs, operation/backend limits, checked/raw guarantees and linked offline evidence. The [clear-break migration](../../docs/migration.md) covers accepted tasks38–43.

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

The admitted subset contains schema objects with `type` (object, array, string,
number, integer, boolean, null; or a nullable pair of one concrete type and null),
`title`/`description`, `properties`, `required`, `items`, `minimum`/`maximum`,
`additionalProperties` and nonempty string/numeric `enum`. Native mode also
preserves `$defs` and acyclic local fragment JSON Pointer `$ref`, nonempty `anyOf`,
`prefixItems` (ordered schema array), `minItems`/`maxItems` (nonnegative integral
JSON numbers), `format` (date, date-time, time) and `propertyOrdering` (string array).
Tool roots require explicit `type: object`; a root reference cannot bypass this.
Output and tool parameter schemas have separate real-SDK HTTP fixtures.

References are decoded once as URI fragments and resolved only to validated schema
positions with strict RFC6901 escapes. Shared targets, escaped names and percent
encoding are preserved; nothing is inlined or fetched. Cycles (including optional
and nullable recursion), external/root/anchor references, missing or non-schema
targets, semantic reference siblings, resource identities and unknown keywords
fail before dispatch. `$anchor` has no required consumer fixture and remains
outside this subset. `oneOf` remains rejected because the provider interprets it
as `anyOf`. Unknown semantics are never stripped or sent through typed fallback.

The consumer matrix covers Kosmify shared/escaped/encoded references, properties
named `$ref`, recursion rejection and exact-constraint/unmapped-semantics fixtures.
Additional output/tool wire fixtures cover branches, tuple order, array bounds,
nullable types, documented formats and property order. This is offline schema
transport parity, not live certification of a deployment or full JSON Schema.
The host continues to own execution budgets, timeout and telemetry.

Admission errors use `SchemaFidelityError` with deterministic traversal and escaped
JSON pointers; errors omit schema values. Checked core compiles the original schema
first, so malformed source schemas and unresolved references can instead return
its existing schema compilation error, still before I/O. Direct translation and
`MapSchema` share the same admission and cycle checks.

Output sends `application/json` and only `ResponseJsonSchema`; tools send only
`ParametersJsonSchema`. Public native schema ports and structural admission remain supported. The SDK converts dynamic schema numbers through float64: large integers and precise decimals can round on HTTP serialization (SDK-GENAI-NUM). No extras hook restores those values. Final checked validation still uses the original exact schema, so request constraints and original validation can differ for affected numbers. Existing per-request options remain copied; the SDK client configuration copy can discard function-valued ExtrasRequestProvider hooks (SDK-GENAI-HOOK). Native mode never promises stronger numeric constraints than the pinned SDK exposes.
