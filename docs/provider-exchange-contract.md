# Existing provider exchange fidelity

## Gemini arguments and continuation

The checked generation paths use google.golang.org/genai v1.48.0. Its `deepMarshal` converts interface JSON numbers to float64. The adapter observes the original HTTP response and native SSE event source; batch inline/JSONL parsing likewise uses original JSON with `UseNumber`. Signed and unsigned complete calls share the exact source restoration and SDK-shape checks. Source ordinals, part count, signature bytes, scope, partial-state completion and canonical binding remain enforced. Unknown or incomplete precision fails explicitly; incremental args remain provisional until completion.

Before the final SDK JSON encoder, existing sync, buffered/native streaming, inline/file batch submission and cache creation restore checked native contents through the SDK `ExtrasRequestProvider` hook. This preserves nested integer/decimal/exponent values and explicit empty argument objects. These encoding guarantees apply to adapter-owned dispatch. A host calling the raw SDK directly with translated DTOs owns its subsequent serialization pipeline. The native SDK must still be able to perform its intermediate conversion: values outside its finite numeric range (for example `1e1000`) are rejected rather than rounded, repaired or retried. Exact content is derived from the translated request; an arbitrary host callback cannot replace it later in the hook chain.

For direct `ParseResponse` on a caller-supplied SDK DTO without original source, string/bool/null, Go integer values and `json.Number` retain their declared JSON representation. Float32/float64 anywhere in maps, arrays/slices, exported struct fields or pointers cannot attest original provider precision and are rejected. Unserializable/cyclic JSON arguments fail explicitly; recursive numeric inspection tracks visited containers without an arbitrary nesting policy. This limitation does not replace the exact observed IO path. Runtime Go callbacks and supplied DTO provenance remain host-owned. With an original source, SDK rounding is accepted only at corresponding float leaves; every exact integer and all other native fields must agree independently. Custom JSON projections whose float leaves cannot be bound are rejected when the exact projection differs.

Gemini WithResponsePayloadLimit bounds retained original-source observation, not
the sync SDK body read: the wrapped reader still returns all bytes to the SDK.
Native observation instead limits each SSE line including framing, without a
whole-response byte bound. Oversize/unavailable exact source fails fidelity checks;
these limits do not promise protection against all SDK allocation. Exact original
parts, numeric leaves, signatures and scoped state/ExtrasRequestProvider restoration
remain deliberately separate guards, not a generic repair layer.

Gemini typed schema mapping accepts only type/properties/required/items/description
and string enum under the existing shape constraints. Other keywords fail closed
with a JSON pointer, rather than disappearing. Extend this subset only with a
concrete pinned SDK representation and original-contract conformance fixtures.
Managed cache Create/Get/UpdateExpiry/Delete remain explicit scoped provider
operations; renewal workers, recaching, memory storage and retry policy belong to
the host.

Anthropic explicit block-marker mapping retains at most four markers and requires
1h markers before 5m markers, without sorting/repairing caller intent. This agrees
with the [provider caching contract](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)
checked on 2026-10-07. Unsupported required controls fail before dispatch; advisory
controls can be removed with diagnostics. This rule does not imply a cache hit,
host renewal policy or support for all newer provider caching modes.

Ollama ParseStreamChunk is a raw parts-only callback decoder. It does not admit
finish/correlation/lifecycle semantics; use StreamEvents or checked ExecuteStream
for authoritative outcomes and lifecycle ownership.

When a later Gemini/Ollama part fails decoding, already accepted canonical parts remain available together with the error. The partial response has unknown outcome/finish and unavailable continuation; it cannot be treated as successful completion. No automatic retry occurs. Failed or canceled checked streams conservatively mark a retained exchange unavailable for continuation, including malformed whole frames, transport errors, missing finish, conflicting call assembly and consumer stop. Observed finish metadata, when already known, remains evidence rather than a success guarantee. Source/SDK errors before any accepted canonical part produce no invented partial response. The Ollama HTTP decoder preserves ordinary reasoning/text and preceding valid calls on an argument failure; sync and buffered/native parsing publish this prefix without a successful finish. Duplicate argument keys, including nested or escaped equivalents, are rejected before decoding into maps.

Gemini `FunctionResponse.ID` receives the canonical `ToolCallID`. Parallel repeated function names are correlated by distinct native IDs. Duplicate calls/results, changed names, unknown results within supplied call history and ambiguous ID-less/mixed correlation fail instead of guessing. A result-only low-level request can carry explicit native IDs without a preceding call in that request; its earlier provider history is host-owned. No ID is generated from a function name.

Checked execution derives `tool_name_correlation` from ID-less call/result history, even without an explicit requirement. The profile must confirm it and the adapter must implement the native mapping port. Low-level Translate/Execute expose native protocol mechanics and do not provide checked profile admission; hosts choosing them own capability confirmation. Gemini and Ollama retain their native name-preserving mapping ports.

## Reasoning and result semantics

Ollama v0.17.4 continuation maps a contiguous canonical reasoning prefix to `Message.Thinking`. Multiple adjacent reasoning parts concatenate in order. Ordinary text remains in `Content`; subsequent reasoning after another part is explicitly unsupported because one native field cannot preserve that interleaving. The ordinary sync/buffered/native response → wire → next HTTP request paths preserve thinking without automatically logging it. Explicit host capture policies remain independent.

| Existing result path | IsError | Content shapes |
|---|---|---|
| Gemini | Native `error` or `output` response key; native call ID retained | Existing text results; unsupported multimodal forms rejected |
| Anthropic | Existing native `is_error` retained | Existing text/image mappings; other shapes rejected |
| OpenAI Chat | Explicit unsupported error before dispatch | Existing text-only tool result mapping |
| OpenAI Responses | Existing explicit unsupported error | Existing checked result shapes |
| Ollama | Explicit unsupported error before dispatch | Existing text-only results; unsupported multimodal forms rejected |

An error result is never silently encoded as success. New multimodal or provider features belong to task43. Ollama has no batch path in this library; Gemini's existing inline/file batch parsing uses the same argument contract. No new provider number/state types enter core, and existing incompatible state/wire formats remain rejected.

Protocol references checked against the pinned local SDKs: [Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling), [Ollama tool calling and follow-up fields](https://docs.ollama.com/capabilities/tool-calling), [Ollama thinking](https://docs.ollama.com/capabilities/thinking). Actual local HTTP/SSE/JSONL fixtures establish serialization conformance; they are not live model capability evidence.

Migration: declare name-correlation support in host profiles for ID-less history. Regenerate provider tests expecting silent IsError loss or SDK number normalization. Use canonical exact args and checked invocation for provider admission; remove legacy repair/fabricated-ID assumptions.

## Ownership and execution lifecycle (task45)

These are target contracts for task45; the remediation acceptance record tracks implementation. Immutable publication boundaries seal JSON-domain maps/slices/bytes and preserve json.Number. Cloning is not a universal Go object-graph copier: cycles and arbitrary mutable struct/port graphs require caller-owned types and ownership policy.

| Surface | Ownership and concurrency contract |
| --- | --- |
| ManifestSnapshot / captured RenderPlan | Captured sources are immutable. Template defaults and returned defensive template clones own independent nested JSON values; construction options reused across templates do not share mutable JSON. Parsed templates can be shared read-only. Concurrent execution requires read-only host funcs/ports. |
| RegistryPlanInput | Binding can borrow caller map/slice values until Plan takes its snapshot. Keep payload immutable from token creation through planning; do not mutate it concurrently. History/compose owned copies do not imply universal payload cloning. |
| PromptExecution, Response, typed recipes/checkpoints, RuntimeOptions | Mutable caller DTOs. Do not mutate during dispatch, cloning or binding; shallow generic Input/LateInput fields do not promise deep copies of arbitrary T. Explicit clone/wire/snapshot boundaries provide only their documented isolation. |
| NewStructuredExecutor | Retained optional serial helper, one logical attempt chain and one provider attempt per call. Private owned execution/scratchpad; ValidationError/ToolCallError may append feedback. Host bounds calls/history, decides retries/backoff, and discards the closure to reset. No concurrent calls or implicit loop/scheduler. |
| Stream | Lazy single-use, one Events consumer. Status returns owned snapshots and concurrent Close requests cancellation. Close is not a join; final status follows unwind. Close before consumption avoids dispatch. |
| Registry/cache and injected ports | Follow the source lifecycle contract. Shared ports must support concurrent invocations; tool registration finishes before invocation. Host callbacks honor cancellation and do not re-enter the same capture session. |

Cancellation is not rollback or proof of remote rejection. Known partial evidence remains available; unknown remote outcome needs host reconciliation before another mutation. Usage unknown differs from known zero. Observer callbacks impose synchronous backpressure; retention limits on observed payloads do not bound SDK/transport allocation or bytes transferred. Error strings can contain legacy structured-output previews/decoder values; capture redaction does not sanitize errors. Hosts must treat errors as sensitive until choosing an explicit redaction policy. Arbitrary host callback panics are not normal provider errors and have no blanket containment promise.

## Canonical source stream event shapes (task45)

A source emits the following event union. It cannot supply Result or StreamTerminal; the lifecycle alone constructs terminal Result. Identity.PartIDs aligns with frame Content, state/annotation indices refer to that frame, and source MessageID is empty. Finalizations refer to previously accepted logical parts. Every nonempty Outcome must be a valid enum on every kind; empty is unobserved except on Finish. Usage is independently validated on every kind. Diagnostics/model metadata remain observed evidence, never profile attestation. ContinuationUnavailable is a monotonic revocation flag: true on an accepted Delta, Finish or metadata Usage is retained; false never restores availability. It can accompany Start metadata too. Invalid event shape cannot install this flag.

| Kind | Before provider finish | After provider finish |
| --- | --- | --- |
| Start | Optional informational event; repetition allowed, not a required phase. Metadata/usage only, no content/state/finalization/part annotations. | Rejected. |
| Delta | Content, associated state/annotations/updates, finalizations and metadata/usage; finish fields can be provisional valid observations. | Rejected. |
| Usage | Metadata/usage and ordered MessageAnnotations; no Content, ProviderState, StateUpdates, Finalizations or Annotations (`[]PartAnnotation`). | Same shape; Outcome/FinishReason must be omitted or exactly repeat the accepted finish fields. |
| Finish | Exactly one, valid nonempty Outcome; may carry final content/state/finalizations/annotations/usage. Establishes immutable Outcome and FinishReason. | Another Finish is rejected. |
| Terminal | Source emission rejected. | Source emission rejected. |

Contradictory trailing finish fields fail atomically with ErrStreamProtocol before metadata/usage from that bad frame is installed. Prior accepted finish, usage and content remain partial evidence, with failed continuation unavailable; no replay and one terminal. Legitimate late MessageAnnotations and usage remain accepted in source order. EOF without Finish is a protocol error. Transport completion with incomplete/refusal/paused/tool_calls outcome does not become successful typed structured output.

A non-nil empty TextPart is a valid no-op fragment for structured parsing, including citation-bearing frames. Nil/non-text fragments remain invalid for a text document. Only finalization classifies the whole empty document as ErrEmptyTextResponse. Item parser limits do not bound total retained response: the stream retains O(total response) data; host cancellation/transport budgets remain explicit.

StreamStructuredOutput accepts at most one StructuredStreamOptions with a positive
MaxObjectBytes. Without options the per-object byte limit is 10 MiB. Explicit zero,
negative limits or multiple options reject before schema/provider work. The limit
is invocation-local; it bounds each parsed object, not total retained document,
whole stream bytes or upstream SDK allocation. No mutable package-wide limit exists.

## Raw and observed ingress boundaries (task45)

Public raw parsers reject untyped nil, typed nil and wrong SDK input types with an invalid-response typed/sentinel error rather than dereferencing them. Raw caller-constructed SDK DTOs have no source-presence attestation; their documented low-level behavior does not replace checked wire-backed validation. Observed consumed text/tool identity/usage fields are checked before trusting SDK coercion; unknown optional fields do not imply exhaustive top-level rejection. Foreign explicit Gemini/Ollama roles fail, omitted roles remain allowed by the agreed DTO contract, and unsupported Ollama images fail rather than disappearing. Sync, buffered and native modes follow the same admissible envelopes while preserving their different lifecycle guarantees.

OpenAI Chat preserves distinct system/developer roles and ordering through the native role mapping, with no model-name-based collapse. Inline PDF uses a filename and reversible application/pdf base64 data URI; unimplemented inline formats fail explicitly. Media fetching/uploading and endpoint capability knowledge remain host-owned. These are wire mapping requirements, not live model certification.

Chat inline audio accepts explicit mp3 (audio/mp3, audio/mpeg) and wav (audio/wav, audio/wave, audio/x-wav) MIME types only. MIME parameters do not change the format; empty, malformed or unsupported MIME types fail before dispatch. The adapter does not guess from bytes or claim codec validation. OpenAI Responses combines system/developer text into its single Instructions field in message order, separated by two newlines; this projection cannot preserve distinct native roles as Chat does.

Part and message metadata use the same nonblank identifier rule: Provider, Codec, annotation Kind and at least one of Model/ModelFamily must contain non-whitespace text. Identifiers are validated without trimming or rewriting their original values. PartIndex is a canonical index in the associated content frame, not a provider block ordinal: a provider text block at ordinal 7 can yield frame Content[0] and therefore PartIndex 0. Finalizations and StateUpdates retain their separate immutable identity/scope checks; they are not arbitrary patches.

Sync, buffered and native terminal capture share a cause classifier. Wrapped cancellation/deadline, schema, parse, capture and protocol causes retain their safe categories. An arbitrary executor error is `operation`; entry into Execute proves only dispatch possibility, not transport IO. Typed URL/network operation errors provide transport evidence. Categories contain no raw error text; original error identity remains available separately. `AfterDispatch` is independent of category and does not prove a physical attempt or remote rejection.

Structured parser diagnostic suffix remains byte-for-byte bounded to the last 1 KiB, including its legacy sensitive error preview. A fixed ring implements that suffix without copying it per input byte. Anthropic text history accumulates privately until block close, then materializes the complete source once; canonical deltas and citation order are unchanged, and incomplete/private state is never published as verified continuation.

Ollama synchronous Chat requires exactly one response callback with stream=false. Zero or multiple callbacks fail with ErrInvalidResponse rather than fabricating an empty result or replacing earlier evidence with the last frame. HTTP chat constructor failures use ErrInvalidHTTPConfig, before response IO exists. Host http.Client redirects/transports can cause multiple physical requests; one explicit Chat operation is not a physical-attempt guarantee.

## Capture callback policy

CaptureSession serializes Emit callbacks with a mutex. Waiting for that mutex is not cancellation-aware; a queued canceled caller can still enter projection before the pre-sink context check. Projectors and sinks honor their context, do not re-enter the session, and bound their own work. Serialization does not impose a memory or latency bound: concurrent callers can queue, and advisory Report.Failures grows per failure. Hosts bound stream duration/event count and discard completed sessions; required capture failures stop their caller. Failure identities remain per event for reconciliation rather than being silently aggregated.

CaptureBinding.MaxBytes validates the final approved payload. It cannot prevent allocation in source serialization, projection, digests or metadata, and does not bound record counts. For example, a host limits an HTTP response body before decoding, selects metadata-only projection, and limits session duration; setting MaxBytes alone is insufficient for those goals. Direct SDK response buffering can still precede observation limits, as documented for each adapter.

Injected capture ports are non-nil concrete implementations. A typed-nil interface is caller misuse; there is no general reflective nil validation or panic containment across these ports. The budgeting estimator has its own explicit invalid-config nil check, which does not expand other port guarantees. CaptureRecordMetadata.Validate checks known usage/outcome/status; SchemaDigests, response/request IDs and names are trusted opaque diagnostic strings. They are neither portable validated schema references nor authorization evidence. The host projector allowlists/redacts them before sink delivery.

Anthropic native thinking_delta and signature_delta are retained privately until
content_block_stop verifies and closes that block. Reasoning therefore appears at
block completion rather than token by token; this latency preserves signed
continuation and does not promise a streaming reasoning UI.

Gemini cache mutation errors deliberately retain only ErrCacheResourceUnknown and
identifiable cancellation, without SDK bodies or raw causes. Unknown means the
remote mutation may have occurred; inspect explicit resource identity rather than
retrying automatically. Safe structured status diagnostics remain a future API
change, not a reason to expose provider text. Usage presence is conservative:
Ollama scalar zeros remain unknown without SDK presence, whereas matching Gemini
original source metadata can establish known zero. Profiles remain caller-owned
knowledge bound to the actual endpoint/model; raw Translate representability is
separate from checked Preflight admission. Ollama IsError tool results are rejected
because this mapping has no faithful native error-result representation.
