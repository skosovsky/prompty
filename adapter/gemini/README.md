# Gemini adapter for prompty

Maps prompty’s `PromptExecution` to the Google Gemini (genai) GenerateContent API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/gemini
```

## Configuration

- **API key / client:** use `google.golang.org/genai` to create a client and pass it to the Gemini API. This adapter returns `*gemini.Request` (Contents + Config); you call the genai client with that request. Model is set at the call site (on the client or in `Request.Config`), not via a default in the adapter.
- **Message metadata:** use `ChatMessage.Metadata` for provider-specific options (e.g. `gemini_search_grounding` for grounding). The adapter forwards metadata where supported by the genai API.

## Capabilities

- **Types:** `Translate` returns `*gemini.Request` (Contents and Config); `ParseResponse(ctx, raw)` expects `*genai.GenerateContentResponse`; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant; tools; images. **Images:** URL is accepted natively in `MediaPart`; no need to call `exec.ResolveMedia` for URLs.
- **Model config:** temperature, max_tokens, top_p, stop are mapped from `exec.ModelConfig` via `adapter.ExtractModelConfig` into the request Config.
- **Helpers:** `prompty.TextFromParts`, `adapter.ExtractModelConfig`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/gemini) for the full API.
