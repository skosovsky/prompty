> Historical task37 evidence; superseded by [current contracts](../README.md) and [migration](../migration.md). Original audit content below is retained; old API snippets and test counts are not current acceptance.

# Task37 selected library scope

Source: [issue #4](https://github.com/skosovsky/prompty/issues/4), clarified by the [owner reply](https://github.com/skosovsky/prompty/issues/4#issuecomment-6008749968) on 6 October 2026. This supersedes unselected-roadmap/rejected-output dispositions. All five directions are required now; optional means consumer opt-in, not deferred implementation. Reference consumers suffice without production workload or credentials as a prerequisite.

| Required direction | Selected implementation | Current code gap |
| --- | --- | --- |
| Native streaming | Anthropic Messages, Gemini GenerateContent, Ollama chat alongside OpenAI; endpoint/model-specific profiles and protocol-fault fixtures | All four initial transports/fixtures/examples exist; Gemini partialArgs have explicit Vertex opt-in, progress and correlated completion. Signed partial state now retains original ordinals and source-part provenance; actual Vertex SDK fixtures preserve global interleaving/signature positions through wire/continuation and trailing usage. Candidate metadata uses exact ordered message-level annotations, including metadata-only events. Expanded conformance and final independent acceptance remain required, including IA10 verification. |
| OpenAI Responses | Separate adapter/profile; continuation; web_search/file_search; background submit/retrieve/status/cancel, optionally explicit bounded Wait | NewResponses implements actual SDK Create/Get/Cancel, explicit storage/continuation, scoped wire handle, original output schema binding and operation capture with fixtures/example. Hosted web_search/file_search mappings and typed results exist. Ordered text/media input has native SDK URL/bytes mapping with profile gates, zero local fetch/upload, negative fixtures and runnable example. Local tools/history and encrypted provider-state projection/continuation are implemented. Active cancel/deadline/close and explicit reconciliation have deterministic SDK evidence in operation-conformance.md; expanded full-scope verification and final paired audits remain required. Other-provider analogues not required. |
| Offline batch | OpenAI Batch, Anthropic Message Batches, Gemini Batch; correlation, partial results, per-item outcomes and optional injected file transport | OpenAI file-backed, Anthropic Message Batches and Gemini inline/file-backed SDK lifecycle/results implementations and local fixtures/examples exist. Shared result-only validation corrects IA13 without replaying generation or requiring an expired input lease. Gemini file fixtures now cover lifecycle/keys/unknown acceptance/limits/malformed/status/close failures. Expanded conformance and final paired audits remain required; these checkpoints are not complete scope acceptance. |
| Managed context cache | Gemini create/get/update expiry or TTL/delete/use handle in generation. Supported OpenAI/Anthropic/Gemini request controls remain distinct | Actual SDK lifecycle, scoped portable handles, checked reuse, semantic capture/default redaction and local fault fixtures/example exist. Expanded conformance and final paired audits remain required; resource CRUD does not replace request controls. |
| Synthetic-tool structured output | Explicit Anthropic profile for forced choice without native output; exactly one dedicated output call, original schema validation, no real tools in base profile | NewSyntheticOutput implements separate explicit sync/buffered extraction, original-schema validation and zero-I/O conflicts, with SDK fixtures/example. Expanded capture/reasoning/concurrency conformance and final paired audits remain required. New explicit contract, not legacy bridge. |

Reuse implemented stream/profile/wire/schema/state/usage/capture guarantees. Detailed contracts and acceptance are in [task37](../../.cursor/docs/task37.md); implementation gaps remain pending in the [matrix](requirements.md). Working adapters, offline protocol/fault fixtures and runnable examples are mandatory per selected provider. Ports/stubs or absent live credentials do not satisfy implementation acceptance.

## Library boundaries

IA16 operation-only preflight correction keeps accepted work inspectable without
fabricated generation options. Responses/batch/cache handle operations check
scoped capabilities/combinations and full profile snapshots; cache-content Create
also retains input tools/state/MIME/schema checks and forbids generation options.
Generation Create/Submit keep the original gate. Shared and actual SDK lifecycle
regressions pass; final paired verification and expanded conformance remain required.

Local Responses functions now have native declarations/selection, checked
original-schema results and exact scoped item envelopes. Portable schema bindings
include dialect/role/name/source digest; Get/Cancel rebind Tools without requiring
generation support. Completed function history now replays original scoped state;
text results correlate by call_id against history or Previous.ToolCalls and keep
native arrays, including checked image URL/bytes and document URL/inline PDF.
SDK/wire/negative fixtures and an offline mixed example exist. IsError has no
independent native flag and fails explicitly; no wrapper or silent flag loss.
Expanded full-scope verification remains required; this does
not introduce handlers or an agent loop.

Hosted stateless replay now preserves a scoped native-order manifest, original
public message/search JSON and citations. Private reasoning/function items are
index references only, with checked required envelopes. SDK/wire/continuation,
mixed privacy and zero-I/O invalid-history fixtures plus offline example exist.
Unsupported old metadata is rejected, not reordered or bridged; full Responses
conformance and final paired audits remain required.

Responses reasoning now has ordered summary projection, exact encrypted item
envelopes and checked stateless assistant replay. Actual SDK Create/Get/wire/next
Create fixtures and an offline example exist. This narrows, but does not remove,
the provider-state gap above: ProviderStateRequired now persists through handle
wire and requests Get encrypted-content includes with independent capability
checks. Cancel retains missing-state errors without inventing a native include;
new Create does not inherit the requirement. Hosted/local-tool history uses the
implemented mappings/manifest described above; expanded full-scope verification
and final paired audits remain required.

Core stays universal and independent with caller-owned domain types and injected transport/policy. Provider SDK types stay in adapters. Hosts own credentials, access/data/egress/persistence decisions and cleanup scheduling. Scope checks are not IAM.

No agent loop, routing, daemon, scheduler, queue, retry coordinator, dataset/experiment/grade runner, approval UI, billing, application cache or storage backend. Explicit caller-invoked bounded Wait observes one operation under context/deadline, not a background worker. File ports transport host data, not a file store. Hosted file_search consumes vector-store references without corpus CRUD, retrieval engine or local execution. No realtime voice/browser sessions, hidden recache/upload, automatic renewal or silent synthetic fallback. Cancellation is not rollback.

## Acceptance and delivery

Full selected scope is one implementation acceptance target; intermediate MVP does not close the issue. Historical audits certify only their snapshots/inspected scope. Both independent completeness and error audits must verify the same final snapshot after all-module make lint/test. Optional live tests require explicit credentials; absence means unverified live integration, not waived implementation or fake live success.

Use make release-break for full public-contract change; make release-patch only for separately scoped insignificant fixes. Publication, author messaging and issue closure require separate authorization. Closeout explains concrete caller changes for final APIs, handles/outcomes/behaviour; preparation is not release or closure.
