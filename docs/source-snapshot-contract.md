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

Evict(id) starts a new generation for that logical ID; EvictAll does so for every ID. A capture already joined before eviction may finish for its existing waiters, but cannot populate the new cache generation. Callers admitted after eviction returns must not join that old capture. Old completion must not remove or overwrite a newer in-flight capture/cache entry. TTL is freshness, not a byte bound. Material lookups and successful publication clean the expired publication prefix using an O(k) FIFO expiration index; idle caches retain entries until such an operation or explicit eviction. `WithCache` has no capacity limit; opt-in `WithCacheLimit(base, ttl, maxEntries)` bounds stored entry cardinality with FIFO eviction, including at TTL<=0. Held snapshots remain valid. See [retention contract](performance-memory-retention.md). Cache metadata/raw reads delegated to the base can be fresher than cached Plan and must not be combined into a claimed single snapshot.

Shared fetch work is registry-owned, independent of individual waiter deadlines and context values. Credentials, source configuration and trust belong to the configured instance, not the first caller's context. Waiter cancellation remains individually observable. Close forwards closure to the base; it is not a drain/join of captures and does not itself close the wrapper to future calls. The host owns base lifetime, must stop new admissions and finish/cancel active operations before closure, and must not close a shared base while another owner uses it.

HTTP logical IDs are decoded application path names, not pre-escaped URL syntax. Literal percent bytes must be escaped on the wire; a%2fb and a/b identify different sources. Logical nested path separators remain separators, and existing traversal validation still applies. Build Path/RawPath consistently without interpreting encoded dot/slash sequences from an ID. The configured base URL's query is configuration, not syntax supplied by the ID. Query/fragment injection through an ID is forbidden. HTTP's 1 MiB body limit accepts exactly the limit only with observed EOF, rejects an extra byte, and preserves read errors and cancellation at the boundary. Empty progress reads do not prove EOF; response bodies are closed on every path.

Mutable Git refresh errors fail closed by default, including missing remote, auth and unavailable refresh. There is no warning-only stale success. Cancellation/deadline wins at admission, after lock acquisition/refresh and before returning local or immutable-revision bytes; canceled operations return no successful bytes. This applies to Fetch and the corresponding ListIDs/Stat/revision paths. Stale availability, if ever added, requires explicit host policy and visible freshness/error evidence; task45 chooses no stale mode. No refresh retry, watcher or background renewal is introduced.

For mutable Git, `file://` intentionally selects a frozen local checkout (even if that checkout has an origin); other configured URLs refresh on each mutable operation. Binding a commit selects immutable blob reads. Opening a persistent mutable checkout requires successful refresh; a fresh clone does not perform a redundant pull. All source metadata/list operations preserve cancellation as cancellation, including file/embed Stat.

Git WithDir accepts the repository root (empty or dot) or an unambiguous relative
slash path with no empty/parent segments, backslashes or absolute prefix. This is
configuration admission, not symlink confinement of trusted working trees.
WithCloneDir never silently adopts a foreign persistent checkout: before refresh,
its current branch must match WithBranch and its sole origin URL must equal the
configured URL exactly. A frozen file:// URL pointing to that same local checkout
needs no origin, but still checks its branch. Detached/mismatched checkouts reject
without sending configured credentials. Hosts reconcile/reset their trusted clone
explicitly; the fetcher does not rewrite remotes or switch branches.

See [ownership and concurrency](README.md#ownership-and-concurrency) for supported clone-domain and borrow-until-plan rules. Captured snapshots/JSON defaults own their supported material; mutable recipe/checkpoint DTOs are not sealed arbitrary-object snapshots. Registry instances carry source/credential trust; per-caller context values are not forwarded into shared capture.

File/embed List is a base-ID catalog across stored environments, not a promise that every listed ID resolves in the currently configured environment. Plan/Stat environment resolution remains strict. File Stat Version is advisory RFC3339 metadata (subseconds omitted), not content identity; captured descriptors supply identity. HTTP/Git candidates include .yaml, .yml and .json.

File partial globs are relative to each main manifest directory, but List excludes
only the configured partial directory relative to the registry root. A nested
`feature/_partials/example.yaml` can therefore appear as a catalog ID. Prefer
non-manifest extensions for partial text, or explicitly filter that catalog in
the host; List does not inspect YAML semantics or certify standalone prompts.

CachedRegistry has a fixed wrapper method set. For a base lacking List, Stat,
DescribePrompt, ResolveManifest or captured materialization, those operations return
ErrRegistryCapabilityUnavailable; unsupported enumeration is not an empty catalog,
and unsupported metadata is not a missing ID. Raw bytes retain their dedicated
ErrManifestBytesUnavailable sentinel. Capability errors do not certify support
from a Go type assertion alone; inspect the operation result. Cancellation at
admission remains cancellation even when the base lacks that capability.

Built-in file/Git sources read trusted files/blobs without configurable per-source
or aggregate capture byte caps. HTTP/media limits do not extend to them. Hosts
bound trusted tree/blob size and admission, or supply a bounded Fetcher before
capture/parsing. Deadline checks do not interrupt os.ReadFile or schema compilation
and are not universal allocation limits. No unbounded source is called sandboxed.

File cold captures serialize through the registry lock; immutable cached captures
are retained until Reload. Embedded construction eagerly validates/captures the
available selected catalog, then captures reuse those immutable snapshots. This
keeps parser publication simple and safe; no per-ID worker/dedupe layer is added
without measured need. Local eight-ID capture evidence is in task45-performance.md.

Constructor policy is intentional: missing required structural dependencies
(remote Fetcher/cache base) are programmer misuse and panic; invalid source/parser
configuration returns errors. Pass non-nil concrete dependencies; typed-nil ports
are caller misuse unless a particular constructor explicitly checks them. The
checked nil-template constructor and convenience nil-template panic follow the
same distinction, without a universal reflective dependency validator.

Portable RemoteResponseHandle/CacheResource wire validation admits canonical
shape, scope and supported format. It does not authenticate identities or attest
adapter support for arbitrary HostedTools/status combinations. A receiving adapter
performs its capability/state gates and the host authorizes the destination before
operations. Nested maps/slices are cloned at designated handle boundaries; wire
restore does not turn mutable DTOs into permanently sealed snapshots.

The [custom file-port example](../adapter/gemini/batch_file_example_test.go) supplies
BatchFileTransport directly: bounded single upload, explicit destination binding
and a transferred download ReadCloser closed once by the operation. It contains
no file store or renewal worker. Binding is not an accuracy declaration: required
original-source capture additionally needs an honest BatchFileCaptureDescriber.
Upload unknown acceptance remains host reconciliation, never an implicit retry.
