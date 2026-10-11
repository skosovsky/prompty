# Ollama adapter for prompty

Maps prompty's `PromptExecution` to Ollama Chat and returns a full `prompty.Response`, including available usage, finish outcome and exposed reasoning text.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/ollama
```

## Configuration

- **Endpoint:** inject the official `api.NewClient(endpointURL, hostHTTPClient)` with `WithClient`. Host owns authentication, transport, redirects, timeout and lifecycle.
- **Default model:** `New()` uses `llama3.2` as an example convenience, not capability evidence. Select your deployed model explicitly with WithModel. Override with `WithModel(modelName)`, or set per execution via `exec.ModelOptions.Model`.

## Implemented mappings

Chat uses the official `api.Client.Chat` for synchronous and native callback execution. `ChatClient` is an optional provider boundary for injection, not a core dependency. Inbound SDK float arguments fail with ErrMalformedArgs because their original numeric value cannot be attested. Caller-constructed exact arguments can use json.Number. There is no replacement NDJSON parser or frame-limit constructor. SDK frame/decode behavior and numeric limitations are recorded in the [migration contract](../../docs/official-sdk-migration.md).

Ordinary tools are mapped, but native named-tool selection is not implemented. ForcedTool now fails with `adapter.ErrUnsupportedCapability`, including raw Translate, instead of adding a system instruction. A profile override cannot create that mapping; use ordinary tools without the forced guarantee or a supported adapter.

Bind caller-owned `WithProfiles(endpoint, profiles...)` to the actual injected client/model. Required unknown support fails before dispatch. SDK representability is not model capability evidence. See the [migration guide](../../docs/migration.md).

`WithClient` alone no longer implies the default endpoint. Checked execution requires `WithProfiles(actualEndpoint)` even without capability profiles; configure the SDK URL from the same caller-owned value. Missing/blank binding fails before translation or dispatch. An execution override cannot replace this binding, and the adapter does not inspect SDK-private configuration.

- **Types:** `Translate` returns `*api.ChatRequest`; `ParseResponse(raw)` expects `*api.ChatResponse`. `StreamEvents` uses native Chat callbacks with stream=true, immediate consumer-stop propagation and no sync fallback. Use checked `ExecuteStream(..., StreamNative)` with an endpoint/model-specific native capability profile; `StreamBuffered` remains a separate choice. Incoming tool arguments are complete snapshots (`Args`), not `ArgsChunk` deltas. SDK indices have no presence metadata and do not establish correlation. IDs remain provider-owned; ID-less calls stay ID-less. One-shot tool helpers permit name correlation only with explicit checked `CapabilityToolNameCorrelation` and unique batch names; tool results preserve `tool_name`.

Run the offline example with `go test -run ExampleAdapter_StreamEvents`. Usage zeros remain unknown when SDK presence is unavailable. Semantic SDK callback capture is not original NDJSON-byte or physical-attempt capture. Protocol EOF/invalid frames retain partial output; caller requests are not modified by sync/native mode selection. Tool support is model-dependent, not universal Ollama support.
- **Messages:** system, user, assistant. **Tools:** native Ollama tool definitions and tool call/result format.
- **Media:** Ollama chat request supports only `images`; this adapter accepts only `image/*` user media. For image URLs call `exec.ResolvedMedia(ctx, fetcher)` before `Translate`; otherwise the adapter returns `adapter.ErrMediaNotResolved`. Tool results remain text-only in this adapter.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Schema/output:** tool schemas must survive actual SDK decode/serialization without field loss or numeric rounding; unsupported shapes fail with a source pointer before dispatch. Native `ResponseFormat` maps the original schema object to `ChatRequest.Format` with no prompt injection or lowering. Both checked sync and native stream validate the final output against the original schema; stream fragments remain provisional. Exposed reasoning is a separate channel and does not enter output JSON. Opaque provider-state translation is not implemented.
- **Thinking controls:** `ProviderSettings.think` maps to top-level `ChatRequest.Think`, never `Options`. The installed SDK v0.40.0 supports booleans, null and `low`/`medium`/`high`; other named levels fail explicitly. Even false/null require a caller-owned profile declaring `reasoning` and a `ModelOptionsSchema` constraining allowed values. Unsupported feature combinations fail before dispatch. No model-name inference or automatic `/api/show` call.

These new features also require profile knowledge for direct `Translate`. For native output declare `structured_output`; for native streaming additionally declare `native_stream`. Bind capabilities to your actual endpoint and model, including `ForbiddenCombinations` where needed. The [Ollama documentation](https://docs.ollama.com/capabilities/structured-outputs) currently excludes cloud structured outputs; fixtures prove serialization and local validation, not server/model support. Thinking controls are [model-specific](https://docs.ollama.com/capabilities/thinking); profiles must not permit controls merely because the SDK represents them. Run `go test -run ExampleAdapter_nativeFormatAndThink` for a complete offline SDK fixture and zero-IO negative example.
- **Helpers:** `prompty.StrictTextFromParts`, `prompty.JoinAdapterTextParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/ollama) for the full API.

The [unified capability matrix](../../docs/provider-capability-matrix.md) records pinned SDKs, operation/backend limits, checked/raw guarantees and linked offline evidence. The [clear-break migration](../../docs/migration.md) covers accepted tasks38–43; historical task37 evidence is not rewritten as current certification.

Sync Chat requires exactly one callback; native Chat owns correlated multi-frame lifecycle. Zero/multiple sync responses fail explicitly. Redirect/transport policy belongs to the host, so an explicit Chat call is not proof of exactly one physical HTTP request. EstimateTokens/EstimateTokensStrict use an approximate message counter, exclude tools/schema/native wrapping and do not certify a context window.
