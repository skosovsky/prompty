# Clear-break migration: tasks38–43

These contracts replace previous behaviour. Regenerate and validate persisted data
at the application boundary; no compatibility loader, wrapper or instruction to
ignore admission errors is supplied. The [capability matrix](provider-capability-matrix.md)
is the current implementation map.

| Change | Required consumer action | Executable contract / evidence |
| --- | --- | --- |
| 38: ExtractSchema now returns schema and error; malformed explicit providers do not fall back to reflection | Handle the error; provide explicit schema for recursive/custom codecs and unsupported inference | [schema contract](schema-data-contract.md), [reflection](../schema_reflect.go) |
| 38: dynamic numbers are exact json.Number; composition rejects conflicting property/root constraints | Remove float64 assumptions and local schema override expectations; preserve original validation separately from domain Validate | [JSON document](../jsondoc.go), [composition](../manifest/compose.go) |
| 38: generated DTOs validate original schema and track presence, not nonzero | Regenerate recipes/DTOs. Required zero/false/empty can be valid; optional nullable pointer layers distinguish missing/null/value. Generated numeric fields use json.Number | [generator](../cmd/prompty-gen/gen/schema.go), [schema contract](schema-data-contract.md) |
| 39: explicit empty ToolScope.Allowed denies all tools; nil scope differs | Use WithToolScope; supply required definitions and exact schemas/dialects/capabilities. Do not mutate a retained scope or use a broader registry as admission | [tool contract](tool-execution-contract.md), [scope](../tool_contract.go) |
| 39: handlers and tool invocation receive context.Context | Change constructor callbacks, TypedTool.Invoke and ToolInvoker.InvokeTool signatures/calls. Propagate context to IO; handle retained result + error/cancellation | [binding](../tool_binding.go), [orchestration](../tool_orchestration.go) |
| 39: execution wire is prompty.execution.v2 with explicit scope | Persist/restore with designated MarshalExecution/UnmarshalExecution, not arbitrary json.Marshal of execution. Old wire markers are rejected; runtime recipe scope is explicitly rebound | [execution wire](../wire.go), [tool contract](tool-execution-contract.md) |
| 40: captured immutable source generation replaces Verify then separate Plan | Use CaptureManifest/VerifiedManifestPlan and the capture port for custom verified registries; retain the captured snapshot under host restore policy. Ordinary BYOT Registry remains a low-level unverified path | [source contract](source-snapshot-contract.md), [snapshot](../manifest_snapshot.go) |
| 40: source descriptors and compiled wire version3 replace old digest/compiled constructors | Regenerate checkpoints/constructors; create compiled checkpoints FromRenderPlan(ctx, capturedPlan), not by rereading registry. Bind immutable remote revisions explicitly; mutable capture is not atomic remote revision | [source contract](source-snapshot-contract.md), [compiled checkpoint](../internal/compiled) |
| 42: Gemini source arguments and native result IDs preserved; unverifiable SDK floats fail | The official SDK migration supersedes observed IO/encoding paths: use SDK representations with the documented numeric/source limitations. Preserve native IDs; declare tool_name_correlation for ID-less histories, never invent IDs from names | [exchange contract](provider-exchange-contract.md), [Gemini source](../adapter/gemini/response_source.go) |
| 42: IsError is preserved only with a native mapping; Ollama reasoning interleaving limited | Handle explicit unsupported errors on Chat/Responses/Ollama error results; use Gemini output/error or Anthropic is_error when appropriate. Keep Ollama reasoning as a contiguous prefix | [exchange contract](provider-exchange-contract.md), [Ollama continuation fixtures](../adapter/ollama/reasoning_continuation_test.go) |
| 41: DropOldest/DropOldestStrategy/TruncateWithStrategy and message Strategy.Truncate removed | Use execution-based Apply with explicit estimator, protection/removal policy and report. Default middleware blocks unsatisfied budgets; advisory is explicit. Message estimates do not guarantee whole-request limits | [budget migration](../ext/truncate/README.md), [middleware](../middleware/contextx/README.md) |
| 43: RemoteResponseRequest.HostedTools removed | Move declarations to PromptExecution.HostedTools; keep adapter-owned kinds/settings separate from local ToolScope. Clone/wire/batch identity preserve declarations | [hosted contract](../hosted_tools.go), [matrix](provider-capability-matrix.md) |
| 43: hosted history requires scoped Required MessageAnnotations and a verified source-order manifest | Retain complete AssistantMessage via StatePreserve. Unmarked/edited/foreign history is rejected. StateOmit revokes continuation when required metadata is omitted; private state stays in envelopes | [annotation scope](../provider_state.go), [Anthropic history](../adapter/anthropic/hosted_history.go), [Responses history](../adapter/openai/responses_hosted_history.go) |
| 43: Anthropic pause_turn is OutcomePaused | Distinguish pause from completed/incomplete/refused. Persist known response and explicitly choose a new request; typed output/tool helpers do not accept pause as success or handler permission | [outcomes](../response.go), [pause fixture](../adapter/anthropic/paused_test.go) |
| 43: Responses native stream/strict controls, Ollama Format/Think and Gemini image results require explicit profiles | Select existing ports and allowed settings/feature combinations. No native-to-buffered/synthetic fallback, model-name inference or hidden strict-schema rewriting. Close lazy streams and inspect partial Status/RemoteResult | [matrix](provider-capability-matrix.md), [Responses stream example](../adapter/openai/responses_stream_example_test.go), [Ollama example](../adapter/ollama/format_think_example_test.go), [Gemini example](../adapter/gemini/function_result_media_example_test.go) |

Restore validators/transports/capture policy explicitly; runtime ports and credentials
are not persisted. Error results can retain known provider output/usage/identity;
inspect that evidence before host reconciliation. Library cancellation does not
roll back tool effects, cancel a remote job automatically, or authorize retry.

Historical independent acceptance and checks for tasks 38–43 are local-only
artifacts under the ignored `.cursor/` directory and are not distributed with
the repository. They attest their frozen snapshots; current source contracts,
regression tests and documentation provide the published migration evidence.

Task45 removes `ToolHandler`, `ExecuteWithToolValidation` and `ErrToolInvokerRequired`. Replace execution calls with `ExecuteWithTypedTools` and provide a `ToolInvoker`; pure `ToolValidator` ports remain separate. No legacy aliases remain.

Structured streams accept invocation-local StructuredStreamOptions with one
positive MaxObjectBytes; omitted options retain 10 MiB per object. Cache wrappers
return ErrRegistryCapabilityUnavailable for unsupported optional operations rather
than empty catalogs/missing IDs. Persistent Git checkouts must match configured
branch/origin before credential use; reconcile foreign clones explicitly.

`PlanInputWithComposeContext` now returns `(RegistryPlanInput, error)` and honors optional ValidateComposeContext before reading values. Handle its error before registry access. Use CheckedComposeValuesFromPairs for dynamic path/value admission; NewComposeValuesFromPairs panics on invalid paths, collisions or nonfinite numbers. CachePolicy.Mode uses CacheMode; explicitly convert runtime strings and validate as before, without changing wire values.

`NewRenderPlan(nil)` now panics immediately as programmer misuse; use the checked `NewRenderPlanFromPlanInput` for error-return admission. RNG failures in randomHex fail rendering instead of generating an empty delimiter. `NewTypedTool` requires an object-root argument schema; scalar JSON decoding remains available through DecodeToolArgs. TypedToolRegistry copies ready-JSON result buffers; direct typed handler values retain their generic ownership contract.

OpenAI Chat inline audio accepts explicit mp3/wav MIME families only; unsupported/empty/malformed MIME types are rejected rather than passed as arbitrary native format strings. Part state/annotation identifiers reject whitespace-only values consistently with message metadata. synchronous Chat requires exactly one callback. Synchronous OTel wrappers end spans on caller-port panic with a safe category and propagate the original panic without exporting its value.

## Official SDK migration

Remove `ollama.NewHTTPChatClient` calls and inject `api.NewClient(url, hostHTTPClient)` through WithClient. Gemini now borrows the supplied genai client directly, removes original-body observers and schema/content request rewrites, and captures SDK representations. Signed continuation validates that representation; finite numeric SDK arguments are accepted; exact caller/persisted signed state that SDK serialization changes fails with ErrProviderState and requires fresh state. Native schema dynamic numeric constraints retain SDK serialization limits. Accepted malformed responses have no recovered identity unless the SDK returns it. Anthropic batch Results uses the SDK iterator. Follow the [complete matrix/register](official-sdk-migration.md) and [upgrade checks](verification.md#sdk-upgrade-verification). These are explicit guarantee changes; upstream limitations are not fixed.


Ordinary numeric tool calling is supported by SDK representation, including nested
zero/one/fractions and string references. Schema checks remain on the available
value. OpenAI/Anthropic retain raw JSON precision; Gemini/Ollama may round large
integers or precise decimals. Scalar zero usage without public presence evidence
stays unknown, including trailing native usage; unknown does not establish zero
cost or budget safety. See [capability refinement](sdk-capability-contract.md) for
state migration and consumer expectation changes.
