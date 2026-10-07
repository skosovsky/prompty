# Provider capability mappings

Profiles are caller-owned knowledge for the exact provider, endpoint and effective
model. The following table describes implemented mappings, not live account/model
certification. Offline SDK fixtures exercise request serialization and response
protocols. A field in an SDK or a familiar model name does not establish support.

| Endpoint and operation | Mapping | Required knowledge and limits |
| --- | --- | --- |
| OpenAI Responses foreground Create | `reasoning_effort`, `reasoning_summary` | `reasoning` and the profile's ModelOptionsSchema. Effort: none/minimal/low/medium/high/xhigh; summary: auto/concise/detailed. The profile may narrow them. Summary does not request encrypted state. |
| OpenAI Responses foreground Create | `RemoteResponseRequest.StrictTools` | Explicit `strict_tools`, local tools, original schemas compatible with the adapter's strict subset. Closed objects and all properties required; no implicit schema rewriting. Returned arguments still validate against the original schema. |
| OpenAI Responses foreground CreateStream | Native SDK SSE | `response_create`, `native_stream`, plus actual features and combinations. Text, summary/encrypted state, function arguments, supported hosted search/citations and late usage; partial output and observed handle retained on failure. No Get polling, reconnect or automatic resume. |
| OpenAI Responses Get/Cancel/background | Explicit existing remote operations | Independent operation knowledge; background is selected explicitly. These operations do not acquire native streaming or strict generation requirements automatically. |
| Ollama Chat and native stream | Native Format from original output schema | `structured_output`; completed output validates against original schema. Nonterminal JSON is provisional. No prompt/synthetic fallback. |
| Ollama Chat and native stream | Top-level Think | `reasoning` and ModelOptionsSchema; bool/null/low/medium/high are mapped where explicitly supported. No model-name inference. |
| Gemini GenerateContent sync/stream/batch | Multimodal function results | `gemini.function_result_media`, tool knowledge and InputMIME. Text plus PNG/JPEG/WebP, preserving ToolCallID and order. Developer backend: inline bytes; Vertex: inline bytes or valid `gs://bucket/object`. No implicit upload/download. |
| Anthropic Messages sync/native stream/batch | Versioned hosted declarations and source history | `hosted_web_search` / `hosted_web_fetch`, actual operation combinations, and `provider_state` for required continuation metadata. Search `web_search_20250305`; fetch `web_fetch_20250910`. Server calls/results/errors remain hosted records. |
| Anthropic synthetic output | Separate checked synthetic mapping | Hosted declarations/history have no implemented port and fail before dispatch. Native streaming is unsupported for this separate adapter. |

## Portable continuation and migration

Declare hosted tools in `PromptExecution.HostedTools`; the former Responses-only
request field has been removed. Each kind has an adapter-owned closed settings
contract. Unknown kinds, settings, shapes and profile combinations fail before IO.

Hosted history records use required scoped `MessageAnnotations` and a verified
source-order manifest. Preserve them with response/execution wire `StatePreserve`
and pass the unchanged `AssistantMessage` into an explicit caller request. Core
checks scope and required mappings without decoding provider DTOs. Missing legacy
markers, foreign destinations and edited source fail before dispatch. `StateOmit`
removes scoped annotations and revokes continuation when required metadata is lost.
Private signed state remains in provider envelopes, not duplicated into manifests.

Anthropic `pause_turn` maps to `OutcomePaused`. It completes the current transport
operation but does not claim completed structured output or authorize execution
of local handlers. Callers choose when and whether to issue another request. The
library supplies no resume loop, browser executor, sandbox or session service.

`RemoteResponseStreamer` is an optional native port. Canonical stream part
finalization seals observed fragments against complete provider output; unknown
mandatory events, malformed consumed source fields and inconsistent terminal output produce explicit errors. SDK coercion does not establish source fidelity.
Consumers should use canonical stream/result APIs instead of treating semantic
capture as HTTP bytes. Cancellation and premature EOF retain available partial
output and usage while revoking unfinished continuation.

## Verification boundary

Task43 passed independent completeness and error acceptance using offline SDK conformance checks. Local protocol
fixtures are runnable offline; live support requires separate caller verification
for the configured account, endpoint, model and selected feature combinations.
See adapter READMEs for concrete examples, settings and rejected forms.
