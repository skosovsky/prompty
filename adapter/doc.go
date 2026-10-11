// Package adapter binds caller-owned executions to provider SDK request/response
// types. Provider modules keep SDK dependencies outside the core contract.
//
// # Checked execution
//
// NewClient and NewRuntimeClient check the effective endpoint/model profile,
// requirements, combinations, schemas, cache intent and continuation state before
// dispatch. Inject caller-owned transports and provenance-bearing profiles.
// Unknown required knowledge fails closed. Raw Translate/Execute/ParseResponse
// are low-level SDK primitives, not a substitute for checked execution.
// WithClient does not reveal SDK endpoint configuration. Bind WithProfiles to
// the actual caller-owned endpoint even when no capability profiles are supplied;
// missing binding is a preflight error, never a default-endpoint assumption.
//
// Execute can return a known Response together with a post-dispatch validation or
// capture error. Preserve that result; such an error is not permission to replay.
// Inspect outcome before treating text as success and usage presence before
// treating missing counters as zero. Preserve state through explicit wire policy
// and AssistantMessage, not by copying Content alone.
//
// # Streaming
//
// Invoker.ExecuteStream requires explicit native or buffered mode and returns a
// single-use handle with local message identity and source-declared logical part
// references. Native sources must supply ResponseChunk.Identity.PartIDs aligned
// with Content; canonical capture runs after identity validation. Middleware
// attaches to the original handle through ObserveLifecycle rather than consuming
// its events into another stream. Consume Events, close on early stop and inspect Status
// for the known partial result/error. StreamerAdapter.StreamEvents is the optional
// native source port. Native never falls back to synchronous execution; a chunk
// parser alone does not implement native transport. Sources honor cancellation,
// stop yielding after false and emit finish/control markers, not a lifecycle
// terminal. The shared handle assembles tools, finalizes original schemas and
// emits the sole terminal. Routing/retry coordinators remain outside this package.
//
// # Custom adapters
//
// ProviderAdapter supplies Translate, Execute and ParseResponse. Checked clients
// additionally require RuntimeDescriber with effective endpoint/default model;
// optional ports declare actual schema mapping, native source, capture hooks and
// provider-state validation/parsing. Overrides add knowledge, never missing code.
// ParseResponse must preserve ordered parts, available counters, outcome and
// supported annotations/state; empty content need not mean absent metadata.
// Schema validators and domain DTO business validation are separate ports.
// Remote handle operations use PreflightOperation: capability/profile/binding
// knowledge is checked without applying generation-only model options to an
// accepted job. The full effective profile still defines the diagnostic snapshot.
// Generation Create/Submit retain ordinary Preflight and original schemas are
// rebound for local result validation; operation knowledge does not create IO.
// Resource content creation uses PreflightInputOperation to retain inferred
// tools/state/MIME checks while rejecting generation settings or output intent.
//
// Media resolution is caller-owned. Use an injected fetcher only for shapes that
// require inline bytes; image/PDF URL mapping availability is adapter-specific.
// No scheduler, persistence backend, authorization service or agent loop is added.
package adapter
