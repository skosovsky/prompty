# Current contracts

Use the root contracts below as the current public specification. `docs/task37/*` preserves historical audits and acceptance evidence; it is superseded by these contracts and the clear-break migration guide. Historical test counts and API snippets do not establish current conformance.

| Contract | Boundary |
| --- | --- |
| [Schema/data](schema-data-contract.md) | BYOT, presence, numbers, pure schema ports and supported projection |
| [Host tool dispatch](host-tool-dispatch.md) | Pure admission, caller correlation, context mapping and cleanup recipes |
| [Tool execution](tool-execution-contract.md) | Scope, one-shot batch preflight, cancellation and side effects |
| [Provider exchange](provider-exchange-contract.md) | Checked/raw execution, frame shape, outcomes, state, capture and lifecycle |
| [Source snapshots](source-snapshot-contract.md) | Materialization ownership, verified generation, cache/HTTP/Git cancellation |
| [Provider matrix](provider-capability-matrix.md) | Implemented adapter operations and offline evidence; not live model certification |
| [Migration](migration.md) | Breaking DTO/handler/schema/wire/source changes |
| [Request budgeting](../ext/truncate/README.md) | Remove-only execution groups, explicit estimator/provenance |
| [Budget middleware](../middleware/contextx/README.md) | Request-local predispatch budgeting; no conversation memory or scheduler |

Task45 [acceptance](task45-acceptance.md) and [decision register](task45-design-decisions.md) are audit evidence. Pending rows are incomplete work, not extra normative rules.

## Ownership and concurrency

| Surface | Ownership and permitted use |
| --- | --- |
| ManifestSnapshot / captured generation | Immutable owned source/material; each Plan receives independent materialization. Retain under explicit host restore policy. |
| RenderPlan | Constructor receives a valid non-nil template and owns its supported material; With methods return new plans. Caller input must satisfy the schema/data admissibility rules below. |
| RegistryPlanInput | Opaque mutable-input token. Maps/basic slices may be borrowed until plan construction; history is copied. Keep input stable until materialization. |
| PromptExecution / Response / recipe and wire checkpoints | Mutable DTOs. Clone before sharing or concurrently mutating; a generic checkpoint value is not a sealed snapshot of arbitrary Go objects. |
| NewStructuredExecutor closure | One serial attempt chain. Private scratchpad grows across explicit caller invocations; host bounds history and owns retry/backoff. |
| Stream | Lazy single consumer. Concurrent Close requests cancellation; it does not join. Final status follows iterator unwind. |
| Observer/capture callbacks | Synchronous serialized backpressure, no re-entry or mutation of retained error values. Honor context; callback latency affects the caller. |
| TypedToolRegistry | Constructor-only configuration, finish registration before concurrent calls. Registration and invocation are not concurrent operations. |
| File/embed/remote registries | Operations own captures; host controls source/credential trust and lifetime. File/embed cold captures can serialize. |
| CachedRegistry | Concurrent Plan/capture and eviction select generations. Close forwards base closure and does not drain or disable the wrapper; host stops admissions/active work first. |
| SDK configuration / transports | Adapter snapshots stated config fields; transport/client references remain host-owned. Rebind profiles after transport reconfiguration. |

The reflective clone domain is acyclic maps/slices/arrays/pointers of supported scalar or immutable values. It is not arbitrary Go-object graph copying: mutable reference fields inside struct-valued nodes and cyclic graphs are outside its snapshot guarantee. BYOT roots remain caller-defined; use explicit JSON documents or read-only values at materialization boundaries when ownership matters. JSON PartialVariables and captured source bundles have the stronger explicit owned-copy contract.

Requested model selectors/profile identity are host bindings; response model strings are observed evidence. Native, buffered, checked and raw paths have different guarantees as specified by the exchange contract. Current Responses handle binding requires an exact selected model; aliases resolving to another snapshot are not silently admitted. Endpoint knowledge is not proof of SDK transport routing.

Cancellation does not roll back side effects or prove remote rejection. Preserve unknown remote outcomes for host reconciliation before retries. Unknown usage is not zero. Heuristic token estimates do not certify a model window. Capture payload/observation retention limits do not cap SDK allocations or network bytes. Error strings can retain sensitive decoder/preview data even when capture projections are redacted. Egress, authorization and trusted file/Git trees remain host-owned; no filesystem or network sandbox is promised.
