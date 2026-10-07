# Anthropic adapter for prompty

`pause_turn` maps to `prompty.OutcomePaused`, which differs from incomplete,
refusal and completed model output. A valid pause can complete the HTTP/stream
operation while retaining signed state for an explicit caller continuation.
The adapter does not resume automatically. Output schema success is not claimed
for a paused result; structured-output helpers reject it as non-completed output.
The one-shot typed-tool executor also refuses paused results and does not run
local handlers. Persist the response/state and send its unchanged assistant
message only when the caller chooses to continue. Hosted search/fetch projection
has passed task43 independent offline conformance acceptance.
See the provider's [pause contract](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool#pause_turn-stop-reason).

The implemented sync/history mapping uses versioned HostedWebSearch and
HostedWebFetch declarations in PromptExecution.HostedTools. Hosted call/results
are required scoped MessageAnnotations, with source order and private-state
digests; they never become local ToolCallPart. Keep the markers through wire
restore. Unmarked legacy hosted history, edited/foreign source and unsupported
adapter mappings fail before IO. StateOmit revokes required history continuation.
The dedicated synthetic output profile has no hosted mapping and rejects this
intent before dispatch. Batch fixtures verify the same hosted parser and original execution binding, including pause and zero-IO forbidden combinations. Native SDK SSE fixtures cover mixed hosted results/errors, partial server arguments, stable source order under interleaved closes, premature EOF and wire restore followed by an explicit SDK continuation. Run `go test -run ExampleHostedItems` for an offline SDK fixture that persists state, resumes explicitly and rejects unknown hosted kinds before IO.

## Explicit Message Batches

NewMessageBatches(accountBinding, opts...) provides native SDK Submit/Get/Cancel/
Results. Declare independent batch_submit/batch_inspect/batch_cancel/batch_results
knowledge on the actual Messages endpoint/model profiles. Each item uses ordinary
checked Messages translation, schema fidelity, cache/state checks and original
output validation, without running handlers. Batch input capture is operation-
level; item capture requirements are rejected rather than silently discarded.

Supply unique caller IDs and nonblank caller-owned AttemptID; OriginalID defaults
to the item ID and ParentAttemptID records a prior attempt when selected by host.
Persist EncodeBatchHandle/DecodeBatchHandle output, rebind account/transport and
retain original executions/validators for Results. Their portable digests must
match the submit contract; different input/schema/model fails before download.
Multiple model profiles can share one destination. No scheduler, retry, dataset
runner or store is created.

Results preserve provider order and source line JSON, correlate by IDs and retain
per-item outcomes/partial content/usage. DownloadComplete and CorrelationComplete
are separate from the observed remote ended status and from success of all items.
Missing/duplicate/unexpected IDs and partial retrieval return ErrBatchResults with
the observed prefix. Body closure/read errors do not change known execution to
unknown. Results uses a bounded original-line reader over the SDK request rather
than the SDK JSONL scanner, whose Err hides scanner failures. MaxLineBytes/MaxBytes
are explicit per-call limits; zero selects 4 MiB/64 MiB, negative limits fail
before I/O. No source result URL is fetched outside the SDK endpoint.

SDK mutation retries are disabled. Known request rejection is ErrBatchRejected;
uncertain remote acceptance is ErrBatchUnknown, preserving a safe ID when observed.
Neither grants permission for automatic retry; Cancel may race ended processing.
Optional semantic capture uses common host redaction/acknowledgement policy, not
HTTP-byte or physical-attempt evidence. ExampleNewMessageBatches and local SDK
fault fixtures are runnable; live model/account support remains unverified.
See the current [matrix](../../docs/provider-capability-matrix.md) for separate OpenAI/Gemini batch ports and their fixtures; task37 reports remain historical evidence.

Checked `ForcedTool` needs `CapabilityForcedToolSelection` in the effective profile in addition to `CapabilityTools`; the adapter maps native named tool choice. This is separate from native output schemas and does not promise an infallible provider response.

Maps prompty's `PromptExecution` to Anthropic Messages and returns a full `prompty.Response`, including usage presence, outcome, citations and supported signed/redacted thinking state.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/anthropic
```

## Configuration

- **API key:** configure the Anthropic SDK and inject it with `WithClient`. Checked `adapter.NewClient` performs preflight, translation, SDK execution and parsing. Direct SDK primitives do not supply checked guarantees.
- **Default model:** `New()` uses a default Claude model. Override with `WithModel(anthropic.Model(...))`, or set per execution via `exec.ModelOptions.Model`.
- **Prompt caching:** use `ChatMessage.CachePolicy` and optional part-level `CachePolicy`. Message-level cache is used by default; part-level cache overrides it. Anthropic currently supports `mechanism: block_marker`. Example in a manifest:

```yaml
messages:
  - role: system
    cache_policy:
      mechanism: block_marker
    content: "You are a helpful assistant..."
```

## Implemented mappings

Bind caller-owned `WithProfiles(endpoint, profiles...)` to the injected transport/effective model. SDK field mapping is not universal model support. Required cache controls, native output and reasoning settings need explicit profile knowledge. See the [migration guide](../../docs/migration.md).

`WithClient` alone no longer implies the default endpoint. Checked execution requires `WithProfiles(actualEndpoint)` even without capability profiles; configure the SDK URL from the same caller-owned value. Missing/blank binding fails before translation or dispatch. An execution override cannot replace this binding, and the adapter does not inspect SDK-private configuration.

- **Types:** `Translate` returns `*anthropic.MessageNewParams`; `ParseResponse(raw)` expects `*anthropic.Message`. `StreamEvents` supplies native Messages SSE with retries disabled. Bind `CapabilityNativeStream` to a known endpoint/model profile and use checked `ExecuteStream(..., StreamNative)`. `StreamBuffered` is separate and never a fallback. The old unusable `ParseStreamChunk` stub is removed.
- **Messages:** system, user, assistant; tools and tool use. **Media:** `image/*` maps to image blocks (base64 or URL), `application/pdf` maps to PDF document blocks (base64 or URL), and `text/plain` maps to plain-text document blocks (base64 only). `MediaPart.MIMEType` is required for media translation; unsupported or missing MIME types return `adapter.ErrUnsupportedContentType`.
- **Tool results:** multimodal `ToolResultPart.Content` supports text and media blocks.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Output:** native `OutputConfig.Format` coexists with real tools when the effective profile permits the combination. Native `New()` does not inject synthetic tools or intercept magic names; final output/tool args are validated against their original schemas.
- **Explicit synthetic output:** use `NewSyntheticOutput`, `Requirements.OutputMode = prompty.OutputSyntheticTool`, a valid caller `ResponseFormat.Name`, and independent synthetic-output/tools/forced-selection profile support. One matching output tool becomes JSON text and is validated against the original schema by the checked client. Real tools/caller ForcedTool and manual enabled thinking conflict; no handler or silent fallback is used. This is received-result validation, not constrained decoding. Sync/buffered works; native streaming is not implemented for this separate adapter and fails before dispatch. See executable `ExampleNewSyntheticOutput`; its local SDK fixture is not live model conformance.
- **Synthetic capture:** five shared semantic stages retain SDK build identity; default projection excludes payload. Opt-in source capture distinguishes private tool-use JSON from canonical validated text, preserving original unknown fields and numbers. Required sink errors retain known post-dispatch output/usage without replay. No native-stream, wire-byte or physical-attempt coverage is promised by this profile. `synthetic_capture_test.go` covers sync/buffered stage failures and semantic opt-in.
- **Continuation:** use checked parsing, explicit response wire and `AssistantMessage`. Signed text/data associations are preserved; corrupt/missing/foreign state fails before dispatch. Logging omits required opaque state and cannot be reused as continuation.
- **Reasoning/cache:** explicit mode/budget/visibility/effort settings and marker TTL have separate profile-scoped checks. Disposition is not a cache hit, and controls do not implement cache resource lifecycle.
- **Helpers:** `prompty.StrictTextFromParts`, `prompty.JoinAdapterTextParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/anthropic) for the full API.

The [unified capability matrix](../../docs/provider-capability-matrix.md) records pinned SDKs, operation/backend limits, checked/raw guarantees and linked offline evidence. The [clear-break migration](../../docs/migration.md) covers accepted tasks38–43; historical task37 evidence is not rewritten as current certification.
