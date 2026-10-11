# Task44 contracts before implementation

Base59402ca. Originaltask44scope retained; implementation/acceptance pending.

OTel extension remains independent module. Bind caller tracer/provider via options; global fallback is explicit existing default, not mandatory setup. One logical invocation span observes actual lazy Stream lifecycle; no producer loop, retry counter or physicalattempt claim. Default projection contains metadata categories only, never raw error text/prompt/tool/reasoning/state/URL/account/IDs. Host opt-in projection controls additions and retention.

Pin GenAI semantic conventions Development snapshot `4f85037ef86e92c510d2ef881a58f1076f6fc0e4` (official repo, checked2026-10-07). [Client inference](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md), [operation spans](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Use standard operation/provider/requestmodel/responsemodel/finishreasons and observed input/output/cache_read/cache_write/reasoning token attributes; total remains documented prompty extension. Only Usage.Known counters exported, including zero. Subsets never added twice. Fetch/cache/job resource observations are distinct from generation usage; no pricing/metrics backend/storage/dedupservice.

Core minimal contract changes: Response.Model/ResponseChunk.Model optional observed provider-reported model (not selectedprofile inference), retained through clone/wire/stream. StreamStatus.StopReason distinguishes explicitClose/consumerstop/cancellation without payload or policy. RemoteResponseStream exposes existing ObserveLifecycle method so extension wrappers can attach to original lazyhandle. CloneResponse provides owned metadata projection. No OTel dependency/vendorDTO in core.

Explicit extension wrappers own Responses Create/Get/Cancel/nativeCreateStream, BatchSubmit/Get/Cancel/Results and CacheCreate/Get/UpdateExpiry/Delete. They retain originalport results/errors and keep job/resource states separate from generation/item outcomes. Repeatedobservations must not export generation token counters again. Optionalcaller projection can inspect raw observation but has no default egress.

Full in-memory exporter fixtures will prove knownzero/unknown/effectivemodel/response+error/privacy/lifecycle/resource observations; unified capability matrix/migration must link actualcode+fixtures and caller modelknowledge. Historicaltask37evidence remains unchanged.

## R1 correction invariants

Observed model must come from a genuine raw string when SDK source JSON exists; SDK scalar coercion is not source evidence. Absent model never causes profile-based inference; source null follows the provider-specific optional/required parse contract and never becomes an observed model. Typed SDK string without raw source stays caller-owned low-level input. Malformed source returns a typed error and preserves independently verified partial facts without fabricated model or permission to replay that source-derived response/history. An independently verified remote handle retains its ID/status; explicit Previous-only remote continuation remains a caller-selected port operation, not permission to replay malformed source content.

Known transport/protocol error remains an error category even when partial response outcome is refusal/incomplete; outcome remains a separate attribute. Responses Get/Cancel preserve available observed model/outcome/finish on response+error, while generation token usage remains suppressed for resource retrieval.
