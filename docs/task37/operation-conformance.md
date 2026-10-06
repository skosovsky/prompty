# Provider operation conformance inventory

This inventories installed mappings and deterministic SDK/host-transport evidence,
not live account/model support or final task acceptance. Profiles bind the actual
provider, endpoint, model and caller account reference. No provider-wide supported
model catalogue or automatically inherited capability is installed. Core remains
SDK-independent; all SDK dependencies below are in separate adapter modules.

## Installed SDK and API fixture scope

| Adapter | Installed direct SDK dependency | Fixture endpoint/API scope | Working operation paths |
|---|---|---|---|
| OpenAI Chat / Responses / Batch | openai-go/v3 v3.24.0, adapter/openai/go.mod | Local/intercepted endpoint, fixture model; Chat /v1/chat/completions, Responses /v1/responses, Batch /v1/batches and /v1/files | Checked Chat sync/native/buffered; separate Responses Create/Get/Cancel, previous_response_id, hosted web_search/file_search, explicit background/store; file-backed Batch Submit/Get/Cancel/Results |
| Anthropic Messages / Message Batches | anthropic-sdk-go v1.26.0, adapter/anthropic/go.mod | Local/intercepted Messages /v1/messages and /v1/messages/batches, fixture model; SDK-owned protocol headers | Checked Messages sync/native/buffered; signed thinking, native output and explicitly selected synthetic adapter; native Message Batches Submit/Get/Cancel/Results |
| Gemini MLDev / Vertex | genai v1.48.0, adapter/gemini/go.mod | MLDev local v1beta, Vertex partial-args local v1beta1, fixture model; injected SDK public HTTP config | Checked GenerateContent sync/native/buffered; Vertex-only explicit incremental-args opt-in; original signed parts/order; inline and injected-file Batch; explicit context-cache lifecycle/reuse |
| Ollama Chat | ollama v0.17.4, adapter/ollama/go.mod | Host-injected HTTP endpoint /api/chat, fixture/default model declarations; positive frame bound | NewHTTPChatClient preserves original JSON numbers for sync/native/buffered. Unverified already-rounded SDK values reject; no fabricated forced-tool mapper |

API scope here records observed request paths/configuration in installed fixtures,
not a semantic API release guarantee. SDK versions are pinned by module files;
source/capture SDK-build identity is observational metadata, not proof of live
model support. Native fixture specifics are in native-conformance.md; schema,
state, capture, cache and tool constraints have their own inventories.

## Active Responses interruption and explicit reconciliation

`TestResponsesActiveInterruptionPreservesKnownIdentity` uses the actual SDK with
a host-injected HTTP transport. Create/Get/Cancel reach HTTP acceptance and block
on incomplete body JSON before cancellation or deadline. Every case observes one
dispatch and exactly one original body Close. Create retains unknown acceptance
without inventing an ID from partial JSON. Get preserves the known caller handle/
status with unavailable retrieval; Cancel preserves the ID with unknown mutation
outcome. The source handle remains unchanged. For a known ID, the test separately
invokes Get under a fresh host context to reconcile completion. No adapter replay
or automatic polling runs. An unknown Create without an authoritative identity
cannot be reconciled by inventing an ID or blindly submitting again.

Existing responses_test.go and response capture/state/hosted tests separately
cover complete source identity retention, queued/in_progress/completed/failed/
incomplete/cancelled, cancel race, unavailable resource, incompatible scope,
schema/tools rebind, explicit current instructions/tools/store and typed hosted
coverage. Interrupted partial JSON is not a usable partial model answer.

## Batch lifecycle and active retrieval interruption

`TestMessageBatchesActiveInterruptionAndUnknownCancel` and
`TestBatchesActiveInterruptionAndUnknownCancel` first establish a real SDK job
handle, then interrupt a separate Cancel or Results operation. Cancel receives
HTTP acceptance without complete mutation confirmation: one request, one Close,
known ID and unknown mutation outcome, no retry. Results accepts one complete
JSONL success followed by an incomplete record and blocked read. Cancellation or
deadline retains that exact line, its correlation/usage and ended provider status;
download/correlation completeness are false and two missing IDs are explicit.
OpenAI retains file resources and stops without downloading its second file.

`TestFileBatchActiveInterruptionRetainsPrefixAndRemoteStatus` supplies Gemini's
optional scoped host file port and a persisted output handle. It proves active
cancel/deadline, exact accepted prefix, missing IDs, one Download/Close and zero
Upload/job/generation calls. Download cancellation is not remote job cancellation.
The existing Gemini batch_operation_test.go unknown Cancel fixtures preserve the
ID and original native ProviderStatus, mark mutation outcome unknown and do not
retry. Empty native Cancel acknowledgement means canceling, not terminal canceled.
Inline Gemini results are one bounded JSON resource: interrupted transport cannot
certify individual nested records; only complete source records are materialized.

## Native per-item outcomes and usage

| Transport | Native generation truncation evidence | Item lifecycle outcomes and limits | Usage evidence |
|---|---|---|---|
| Anthropic JSONL | TestMessageBatchesNativeTruncationCancellationAndUnknownUsage: max_tokens -> incomplete with retained text, not a schema-error substitute | canceled and expired native result variants; errored/refusal separately covered in batch_test.go; missing/duplicate/download failures explicit | Same fixture has success with absent usage -> unknown, truncation with known counters; no response/zero-usage fabrication for canceled |
| OpenAI output/error files | TestBatchesNativeTruncationExpiryAndUnknownUsage: length -> incomplete with retained text | batch_expired -> expired; provider HTTP/error -> error; remote batch cancelled is a separate handle state. No special native per-item canceled mapping is claimed; unrecognized error codes remain error/source JSON | Same fixture has success without usage -> unknown, truncation known total; cached counters are subsets, not added twice |
| Gemini inline and host JSONL | TestBatchNativeTruncationUnknownUsageAndUnmappedItemCancellation: MAX_TOKENS -> incomplete in both transports | RPC error with code/status cancellation remains error with exact source, not a promised native per-item canceled/expired enum. Those normalized item lifecycle guarantees are unsupported in this delivery; remote job cancellation is separate. Refusal/provider error/partial correlation covered separately | Same fixture has absent usage -> unknown and known truncated counters; file interruption retains unknown presence rather than known zeros |

Results order is provider-defined, not input order. Batch completion is not success
of every item. `ExampleNewMessageBatches_failedOnlyRetry` implements an explicit
host policy after complete retrieval, excludes successful items from the second
HTTP payload and gives failed/expired attempts new IDs with original/parent
lineage. It is not a dataset/evaluation runner or retry coordinator. Unknown
submit/Cancel and incomplete download require host reconciliation, not blind replay.

## Boundaries and remaining verification

Fixtures use installed SDKs, fake/local HTTP protocols or explicitly injected host
file ports. They do not prove real credentials, deployment, account retention,
model availability, throughput, cost savings or unsupported variants. Optional
live checks require separate explicit authorization and credentials. Handles are
secret-free bindings, not IAM; host owns storage, polling, access, cleanup and
retry decisions. Cancellation is not rollback. Full all-module gates and two
independent audits of the eventual final snapshot remain required.
