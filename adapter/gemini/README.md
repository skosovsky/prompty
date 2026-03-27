# Gemini adapter for prompty

Maps prompty’s `PromptExecution` to the Google Gemini (genai) GenerateContent API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/gemini
```

## Configuration

- **API key / client:** use `google.golang.org/genai` to create a client and pass it to the Gemini API. This adapter returns `*gemini.Request` (Contents + Config + Model); you call the genai client with that request.
- **Default model:** `New()` uses `gemini-2.0-flash`. Override with `WithModel(...)`, or set per execution via `exec.ModelOptions.Model`.
- **Provider settings:** use `PromptExecution.ModelOptions.ProviderSettings` for provider-specific options (e.g. `gemini_search_grounding` for grounding).

## Capabilities

- **Types:** `Translate` returns `*gemini.Request` (Model, Contents and Config); `ParseResponse(raw)` expects `*genai.GenerateContentResponse`; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant; tools; media. URL and inline bytes are mapped through Gemini URI/inline parts; no need to call `exec.ResolvedMedia` for URL media.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Cache control:** `CacheControl` is accepted on messages/parts and ignored by this adapter in current Gemini APIs.
- **Helpers:** `prompty.TextFromParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/gemini) for the full API.
