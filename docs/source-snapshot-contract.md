# Captured source materialization

`ManifestDescriptor.Digest` is `source-bundle.v1:sha256:<64 lowercase hex digits>`. The identity covers canonical JSON containing the logical prompt ID, named source bytes, and declarative resolution settings. Map traversal order does not affect the identity. Main manifest bytes, every transitive import (including inactive conditional imports), and every configured partial are captured once and included. Cycles and missing declared imports fail capture. Runtime compose values select branches inside that captured closure.

`Registry` remains the ordinary BYOT planning port. Verified materialization accepts the small `ManifestSnapshotRegistry.CaptureManifest(ctx, id)` port by itself. `ManifestCheckpointRegistry` embeds Registry and that port, with descriptor recommendation and verification; raw-byte inspection is an independent optional port. A decorator over an incomplete custom registry returns an error when capture is requested.

`NewManifestSnapshot` seals caller-owned bytes and configuration. `SnapshotMaterializer` receives isolated copies and must build only from those sources, without reopening storage. Mutating the supplied source contract is rejected. Go implementations of parsers and materializers are trusted host dependencies; the library cannot prove that arbitrary host code obeys its interface contract.

Typed recipe Plan/Execute and raw `BindRuntime` use `VerifiedManifestPlan`: capture one snapshot, compare its identity with the checkpoint, then construct the plan from that snapshot. A separate Verify/Plan pair is not a verified execution protocol. Recommendation can be followed by a later capture, but any intervening generation change must then produce a mismatch.

`RenderPlan.SourceDescriptor` retains the captured identity through late input, response-format overrides and tool-scope binding. `internal/compiled.FromRenderPlan(ctx, plan)` takes that identity directly from the plan and performs no registry reads. Uncaptured plans are rejected. Raw-byte and canonical-template constructors that assigned a digest to an already rendered execution were removed; caller-built executions can use the public execution wire directly.

## Capture and restore policy

File registries cache the complete immutable source bundle and its parsed manifest closure until `Reload`. Recommendation, verification and Plan select that same cached generation. Editing a file does not silently refresh its identity. Template metadata no longer derives an implicit version from a separate Stat read; declare version in the manifest or bind host metadata separately. Reload starts a new capture generation; previously returned snapshots and plans retain their old identity. A recipe with a new descriptor used against an old cache fails with `ErrManifestDigestMismatch`. Execute can succeed under the old identity if cache policy deliberately retains that old snapshot.

Embed registries copy source bytes at construction. Changes to caller-owned `fs.FS` data afterwards cannot replace that generation. Templates are validated conservatively at construction; runtime conditions are evaluated strictly during Plan.

Uncached remote registries capture a new bundle per operation. `WithCache` retains complete snapshots, including composed manifests, until TTL expiry or eviction. Cache hits do not perform fresh compose probes or source reads. Concurrent callers share a capture; each caller retains its own cancellation, and the shared fetch is canceled when all waiters leave. Cache keys are logical IDs within one configured registry; hosts must scope registry instances to their source configuration and trust domain.

A host may retain a returned snapshot and call its Plan after the source disappears. Durable storage, materializer rebinding, revision history, retention and restore policy remain host-owned. The library does not search historical versions or automatically replace a mismatching checkpoint with current source.

## Source revision boundaries

A bundle captured through several mutable file/remote reads records the bytes actually observed. It does not claim an atomic filesystem transaction or a common remote revision. Its plan exclusively uses those captured bytes, so later changes cannot relabel it.

`remoteregistry.WithRevision(revision)` requires a `RevisionFetcher.BindRevision` implementation returning an immutable source for the whole capture. Unsupported or unavailable bindings fail; mutable fallback is forbidden. The Git fetcher accepts a full commit hash, reads regular blobs from that commit tree, and rejects branch/tag names and unavailable commits. It never uses moving checkout bytes for a bound capture. A shallow clone may not contain the requested commit; the host must provide appropriate clone depth or source availability. Git default behavior without explicit binding remains mutable bundle capture.

Environment, partial pattern and its resolution mode, explicitly configured version, and immutable revision binding are included when applicable. Default settings have the same representation across registries. File partial patterns resolve relative to the main manifest directory; embed partial patterns resolve relative to the configured root. Named partial bytes preserve lexical parse order and implicit basename template names.

## Guarantee and migration

Source identity does not attest Go funcmap code, parser implementation, external services, clock/randomHex, model responses, or durable agent state. Runtime functions can produce different text from the same source bundle. File and Git registries are not sandboxes for untrusted source trees. Source hashes are integrity tokens, not signatures over stored rendered executions; host storage trust and authentication remain external.

Legacy single-file and compose-only digests are rejected by descriptor decoding and verified recipe constructors/binding. Regenerate constructors and checkpoints from captured sources; there is no lossy format migration. Compiled prompt wire is version 3 and uses the core execution wire with explicit `StatePreserve`, including tool scope and provider continuation state. Older compiled versions are rejected.

Custom registries implement the capture port using `NewManifestSnapshot` and a source-only materializer. Plain Registry implementations remain usable for ordinary Plan, without verified source claims. Do not implement the stronger port by reading a digest after constructing a plan or by calling Verify and an unrelated Plan.

Parser input is read-only and output belongs to the caller. Capture passes a private byte copy, rejects synchronous input mutation, and deep-copies parsed manifests before caching. Retaining the parser output cannot alter the captured generation. Compose values and nested Lookup results are owned copies. Codegen rejects collisions between logical IDs and file-path aliases instead of binding schema and identity to different sources.

## Eviction, fetch errors and lifecycle (task45)

Evict(id) starts a new generation for that logical ID; EvictAll does so for every ID. A capture already joined before eviction may finish for its existing waiters, but cannot populate the new cache generation. Callers admitted after eviction returns must not join that old capture. Old completion must not remove or overwrite a newer in-flight capture/cache entry. TTL is freshness, not a bound on memory retention; expired entries need not be deleted until revisit/explicit eviction. Cache metadata/raw reads delegated to the base can be fresher than cached Plan and must not be combined into a claimed single snapshot.

Shared fetch work is registry-owned, independent of individual waiter deadlines and context values. Credentials, source configuration and trust belong to the configured instance, not the first caller's context. Waiter cancellation remains individually observable. Close forwards closure to the base; it is not a drain/join of captures and does not itself close the wrapper to future calls. The host owns base lifetime, must stop new admissions and finish/cancel active operations before closure, and must not close a shared base while another owner uses it.

HTTP logical IDs are decoded application path names, not pre-escaped URL syntax. Literal percent bytes must be escaped on the wire; a%2fb and a/b identify different sources. Logical nested path separators remain separators, and existing traversal validation still applies. Build Path/RawPath consistently without interpreting encoded dot/slash sequences from an ID. The configured base URL's query is configuration, not syntax supplied by the ID. Query/fragment injection through an ID is forbidden. HTTP's 1 MiB body limit accepts exactly the limit only with observed EOF, rejects an extra byte, and preserves read errors and cancellation at the boundary. Empty progress reads do not prove EOF; response bodies are closed on every path.

Mutable Git refresh errors fail closed by default, including missing remote, auth and unavailable refresh. There is no warning-only stale success. Cancellation/deadline wins at admission, after lock acquisition/refresh and before returning local or immutable-revision bytes; canceled operations return no successful bytes. This applies to Fetch and the corresponding ListIDs/Stat/revision paths. Stale availability, if ever added, requires explicit host policy and visible freshness/error evidence; task45 chooses no stale mode. No refresh retry, watcher or background renewal is introduced.

For mutable Git, `file://` intentionally selects a frozen local checkout (even if that checkout has an origin); other configured URLs refresh on each mutable operation. Binding a commit selects immutable blob reads. Opening a persistent mutable checkout requires successful refresh; a fresh clone does not perform a redundant pull. All source metadata/list operations preserve cancellation as cancellation, including file/embed Stat.
