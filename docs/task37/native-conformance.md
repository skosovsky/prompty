# Native profile conformance inventory

This is a source/evidence inventory, not final acceptance or a live provider capability catalogue. Caller-owned profiles describe knowledge for the actual endpoint/model; the host is responsible for that knowledge. No adapter installs built-in supported native profiles. All four initial native transports, signed/unsigned Gemini incremental arguments and ordered message-level candidate metadata now exist. Expanded conformance and IA10 independent acceptance remain pending. A chunk parser is not a native transport implementation.

## Concurrent cancellation and knowledge gates

The shared `internal/streamtest/isolation.go` harness is used by the Anthropic,
Gemini and Ollama `stream_isolation_test.go` fixtures. Two requests overlap on one
client with equal native IDs/indices but distinct arguments. Both publish before
completion; canceling the left releases its HTTP request and preserves partial
content while the right completes with late known usage. Canonical message IDs,
text and tool data stay request-local; each stream has one terminal, no replay.
No observed provider finish means unknown provider outcome after cancellation,
not an invented finish. Unfinished arguments remain partial rather than assembled.
Unknown model and unknown tools requirements fail before HTTP in each fixture.
This is actual local protocol/race evidence, not live endpoint/model verification
or complete schema/state/capture conformance.

## Gemini native implementation checkpoint

`partial_value.go`, `partial_path.go` and `partial_arguments.go` provide the protocol accumulator, now connected through `partial_stream.go`. Source values use Number and explicit presence; strings append only on an open path. Singular member/nonnegative-index paths build an object tree, with sparse arrays allocated by actual entries and completion requiring no gaps/open values. Updates are transactional; unknown/case-aliased/duplicate fields, conflicting variants/shapes, overwrite of a closed path and unbound queries fail. Depth beyond installed encoding/json validation is rejected before recursion. Unit tests cover numeric/presence/path/rollback/index/depth behavior. `partial_stream_test.go` supplies actual SDK/local SSE evidence for provider-ID interleaving, sequential same-name ID-less calls, exact source progress, complete snapshots, bad/ambiguous continuation and retained EOF. Exact path/value progress is a `gemini.partial_function_call` annotation on a provisional empty-Args call, not invented JSON text fragments. Common checked logical correlation materializes final calls/annotation bindings without fabricated provider IDs.

`partial_control.go` exports `CapabilityIncrementalToolArguments` (`gemini.partial_args`). Explicit endpoint/model opt-in plus tools/native knowledge enables `streamFunctionCallArguments` in the installed Vertex request mapper. `partial_control_test.go` proves actual SDK JSON on local HTTP, unknown/unsupported knowledge with zero I/O, no fabricated MLDev mapper, sync rejection and unchanged forced selection. `partial_example_test.go` is runnable offline. Fixture API version is v1beta1, SDK genai v1.48.0, model fixture, not live support evidence. Signed incremental binding uses the original ordered partial-parts codec/source ordinals with actual Vertex SDK continuation evidence described below; expanded conformance and final independent acceptance remain required.

`message_metadata.go` / `message_metadata_test.go` add exact candidate-level
citations/grounding in `MessageAnnotations`, with no invented part-zero binding.
Actual MLDev SDK/local SSE fixtures retain REST citationSources, empty license,
zero offsets and a long decimal confidence score. Ordered grounding updates can
reference previous/current provider chunks; malformed/null/negative/out-of-range
associations fail and retain accepted earlier metadata/text. Late metadata-only
events after finish add no content. Protocol guidance is [GenerateContent
metadata](https://ai.google.dev/api/generate-content). Fixtures use genai v1.48.0,
API v1beta, model fixture, not live certification. Core message_annotation_test.go
covers wire/history, exact JSON and clone/atomic failure; adapter/capture_test.go
covers full/provisional canonical capture. The native executable example now
demonstrates reading late candidate metadata.

Protocol references: [streaming argument semantics](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/multimodal/function-calling) and [REST PartialArg](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/Content?authuser=1). REST explicitly supplies nullValue as JSON null; source decoding distinguishes it from an absent value/closing marker. SDK-semantic enum spelling is separately recognizable. No live endpoint/model claim.

`stream.go` performs native GenerateContent streaming through the isolated injected genai client with full checked profile scope, exact complete tool snapshots and signed parts, semantic source capture and late presence-aware usage. No sync fallback or replay. The old flattening ParseStreamChunk API is removed. `stream_test.go` exercises actual SDK/local SSE ordered text/same-name calls/state wire, large argument numbers, refusal, safe malformed/duplicate/signed-alias/unsupported events, missing/duplicate finish, post-finish content and SDK-swallowed read failure. A blocked server proves early text and break/cancel body release without caller config mutation. `stream_example_test.go` is runnable offline. `partial_state.go/test.go` and `partial_order.go` now retain original signed parts/ordinals and reconstruct provider-global order through actual Vertex SDK stream/wire/continuation fixtures. Missing/edited/duplicate provenance and canceled provisional state fail before continuation dispatch. Message-level citation/grounding updates are described above; they are not incorrectly bound to a content part. Expanded conformance and final independent acceptance remain required.

`adapter/gemini/stream_source.go` observes one bounded SSE source line at a time through the isolated public SDK HTTP configuration. The real genai v1.48.0 scanner/converter rounds dynamic numbers; `stream_source_test.go` preserves source lexical integers at each callback without later-event read-ahead association. It verifies consumer break/closure/one dispatch and bounded/short-read/EOF/read-failure behavior. Source failures are retained independently because the SDK logs scanner errors without yielding them. Native source shape validation precedes SDK conversion; SDK response custom UnmarshalJSON is bypassed by a typed alias with UseNumber for canonical source mapping. Signed source codec checks prevent normalization. Source JSON is not an original HTTP-byte capture claim. No live endpoint/model conformance is claimed.

## Anthropic implementation checkpoint

`adapter/anthropic/stream.go` uses Messages.NewStreaming with SDK retries disabled and deferred transport Close. StreamEventsForProfile receives the full prepared effective profile, including model family, through the shared optional ProfileStreamerAdapter port. Protocol follows the [Messages streaming documentation](https://platform.claude.com/docs/en/build-with-claude/streaming); installed SDK identity is anthropic-sdk-go v1.26.0. No live endpoint/model evidence is claimed.

`stream_test.go` uses actual SDK/local SSE: early text, two same-name interleaved tool calls, late cumulative usage, refusal, completed signed thinking with wire restore/full scope, missing EOF/finish/index/block/signature, wrong delta kind, consumer break and instrumented transport Close/no replay. `stream_example_test.go` is a runnable offline checked native example. Thinking is retained until the block closes with signature; overlapping a still-open reasoning block with a later block fails explicitly, not reordered. Unsupported content variants/deltas fail rather than disappear. Provider capture observes SDK-yielded semantic events, not raw SSE framing, pings, SDK-filtered events or physical attempts. Model profiles remain caller declarations.

Further scoped capture/cancellation/concurrency/fault and schema combinations must be verified before full native requirement credit. Historical OpenAI evidence below is not that verification.

## Declared fixture profiles

Ollama checkpoint: native Chat callback transport, stream=true in translated capture, no sync fallback/replay; consumer break returns an internal stop error and the SDK closes its body. Snapshot Args stay complete, stable provider IDs retain logical refs; ID-less calls never receive fabricated IDs/indices. `adapter/ollama/stream_test.go` covers snapshots/repetition/conflict, late usage, EOF/error/malformed/after-finish frames, early text from a blocked server, body closure, request immutability and name-preserving tool results. `stream_example_test.go` is runnable offline. Protocol source: [Ollama Chat](https://docs.ollama.com/api/chat); installed SDK v0.17.4. No live model verification. SDK zero counters have no presence metadata and remain unknown. SDK callback capture is semantic, not original NDJSON bytes. IA10 parent correction adds ChatClient/NewHTTPChatClient with host-injected HTTP policy and positive frame bound. precision_test.go proves exact sync/native/buffered values through wire/next HTTP request, SDK floats fail closed before handlers, safe errors/bounds. Core Number-preserving assembler/DTO regressions pass. Full Ollama completion remains partial until full gates/expanded conformance and final independent audits.

| Declaration | Destination and claims | Evidence and scope |
|---|---|---|
| `adapter/openai/stream_cleanup_test.go`, `cleanupFixtureClientModel` | OpenAI, intercepted `https://fixture.invalid`, default model or `fixture`; native only | Real SDK with caller-owned HTTP transport/body. `TestNativeFixtureProfilesTerminalConformance` runs role → two text deltas → finish → trailing usage → EOF, read failure after two deltas, and missing-finish EOF for both models. `TestNativeFixtureProfilesStopConformance` runs break/cancel/close for both. Each checks lazy dispatch, one call, body closure, retained identity/content and effective profile binding. |
| `adapter/openai/stream_contract_test.go` | OpenAI, local HTTP server, default model and `fixture`; native only | The same interleaved and parallel conformance helpers run for both model bindings. Same-function tool deltas, identity only initially, arbitrary argument splits, distinct terminal references; parallel streams of the same client with an initial-frame barrier and equal call IDs/indices. |
| `adapter/openai/checked_example_test.go` | OpenAI, local HTTP server, `fixture`; native only | Executable lazy stream, completed result, partial known usage, response wire restore and next-request projection. |
| `adapter/openai/native_capture_test.go`, `nativeCaptureFixture` | OpenAI, local HTTP server, `fixture`; native + structured output | Provider-chunk capture before normalization; canonical frame/terminal identity; incomplete tool prefixes and final assembly; required sink failures retain accepted content; original-schema failure is visible in terminal capture. Actual SDK/local transport, not exact HTTP-byte/physical-attempt evidence. |
| `adapter/openai/schema_test.go`, `TestNativeSDKFinalizationValidatesOriginalAfterTrailingUsage` | OpenAI, local HTTP server, `fixture`; native + structured output | Lowered JSON accepted by provider but rejected against original schema after trailing usage; one failed terminal and one request, known usage retained. The other profiles in this file are sync-only and are not native declarations. |
| `adapter/openai/refusal_schema_test.go` | OpenAI, local HTTP server, `fixture`; native + structured output | Valid JSON plus refusal is not success/schema mismatch/retry; refusal annotations and trailing usage retained. |
| `adapter/openai/structured_lifecycle_test.go` | OpenAI, local HTTP server, `fixture`; native + structured output | Structured consumer observes the original handle; refusal emits no items, failed outcome is not a JSON parser error, usage and single terminal capture retained. |
| `adapter/preflight_test.go`, `fixtureDescriptor` | Synthetic provider `fixture`, `https://fixture.invalid`, model `known`; native + tools + structured output, forbidden tools/structured combination | Generic checked preflight/snapshot/override fixtures, not a provider implementation. `TestSyntheticNativeProfileLifecycleConformance` adds completed/trailing-usage, read-failure/EOF and break/cancel/Close cases through the checked client, proving lazy single translation/dispatch, one terminal except after consumer break, source release and retained content/identity/profile. `nativeLoweringFixture` in `adapter/schema_stream_test.go` separately proves original-schema failure with trailing usage. Required native cannot be replaced with sync/buffered or fabricated by an override without a native port. |

Model names above are fixture binding values, not live supported-model claims. Dynamically allocated local server URLs are exact caller bindings within each test, not a set of independently certified provider endpoints. No credentials or paid provider requests are required.

## Common contract and supplemental evidence

`stream_state_update.go` provides explicit latest-binding updates for logical
tool parts, independent of provider SDK types. `stream_state_update_test.go`
covers source-proven interleaving/ID-less pointer calls, atomic failure and
canceled partial state, unchanged-content invariants and wire retention.
`stream_state_update_example_test.go` is executable; adapter capture tests retain
state-update indices in source-frame records. This is the common foundation,
not by itself Gemini signed incremental protocol/continuation conformance. The
provider integration now has separate partial_state/partial_order SDK evidence
described above, subject to expanded conformance and final audits. Pointer/value tool representation
is not a signed field change; all actual fields must still match before assembly.

`stream_local_tool_test.go` extends common materialization to source-proven ID-less local references: two interleaved equal-name calls stay separate, large integers/order/annotation bindings survive terminal collapse, provider/local namespaces cannot alias, and incomplete/mixed/conflicting arguments still fail. It does not implement Gemini JSONPath partialArgs or change separate tool-handler scope admission. Standalone assembly still has no local proof and rejects ID-less deltas.

The common core suite is provider-independent: `stream_lifecycle_test.go`, `stream_identity_test.go`, `stream_association_test.go`, `stream_finalizer_test.go`, `stream_capture_test.go`, `stream_observer_test.go`, `tool_args_stream_test.go` and `structured_stream_lifecycle_test.go`. It covers atomic frame acceptance, single terminal, partial failure, no replay, cancellation/source unwind, clone isolation, stable logical references, tool assembly and structured parser/finalizer observation. It does not certify a remote model.

The OpenAI SDK suite supplements this with native protocol semantics: missing/null/negative indices, ambiguous choice, conflicting tool identities/names, duplicate provider IDs across indices, malformed/incomplete arguments, unknown finish outcomes, segmentation-independent text/refusal references, same-client parallel isolation and real SDK-body closure. `forNativeFixtureModels` runs these same protocol-negative, interleaved, parallel and identity helpers on both model bindings. Schema/refusal/capture-specific declarations additionally run their corresponding feature fixtures on `fixture`; a native-only declaration does not claim structured-output support.

The terminal/stop suite explicitly runs identical baseline cases on both OpenAI model bindings. The synthetic `known` fixture runs its common checked-lifecycle cases; provider-SSE choice/index semantics are not applicable to that in-memory fixture, and are tested on the actual SDK native implementation instead. Both override/no-native-port and forbidden capability combination cases remain in the shared preflight suite. S05-20 records the parent's complete assessment for those local fixture profiles, subject to both final independent audits. It does not certify arbitrary caller profiles or live models. Owner scope decisions now require Anthropic/Gemini/Ollama native adapters and expanded conformance; earlier baseline evidence does not certify that added scope.
