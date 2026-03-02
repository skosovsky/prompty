# Ollama adapter for prompty

Maps prompty’s `PromptExecution` to the Ollama Chat API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/ollama
```

## Configuration

- **Endpoint:** use the `ollama/api` client (e.g. `api.NewClient()` or custom endpoint). This adapter produces `*api.ChatRequest`; you send it with the Ollama client. No API key; typically local or self-hosted.
- **Default model:** `New()` uses `llama3.2`. Override with `WithModel(modelName)`, or set per execution via `exec.ModelConfig["model"]`.

## Capabilities

- **Types:** `Translate` / `TranslateTyped` return `*api.ChatRequest`; `ParseResponse(ctx, raw)` expects the Ollama chat response type; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant. **Tools:** native Ollama tool definitions and tool call/result format.
- **Images:** only base64. For image URLs call `exec.ResolveMedia(ctx, fetcher)` before `Translate`; otherwise the adapter returns `adapter.ErrMediaNotResolved`. Tool results: if the adapter does not support media in tool results, it returns `adapter.ErrUnsupportedContentType` when `MediaPart` is present in `ToolResultPart.Content`.
- **Helpers:** `adapter.TextFromParts`, `adapter.ExtractModelConfig`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/ollama) for the full API.
