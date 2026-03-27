# Ollama adapter for prompty

Maps prompty’s `PromptExecution` to the Ollama Chat API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/ollama
```

## Configuration

- **Endpoint:** use the `ollama/api` client (e.g. `api.NewClient()` or custom endpoint). This adapter produces `*api.ChatRequest`; you send it with the Ollama client. No API key; typically local or self-hosted.
- **Default model:** `New()` uses `llama3.2`. Override with `WithModel(modelName)`, or set per execution via `exec.ModelOptions.Model`.

## Capabilities

- **Types:** `Translate` returns `*api.ChatRequest`; `ParseResponse(raw)` expects the Ollama chat response type; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant. **Tools:** native Ollama tool definitions and tool call/result format.
- **Media:** Ollama chat request supports only `images`; this adapter accepts only `image/*` user media. For image URLs call `exec.ResolvedMedia(ctx, fetcher)` before `Translate`; otherwise the adapter returns `adapter.ErrMediaNotResolved`. Tool results remain text-only in this adapter.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Helpers:** `prompty.TextFromParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/ollama) for the full API.
