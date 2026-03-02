# Anthropic adapter for prompty

Maps prompty’s `PromptExecution` to the Anthropic Messages API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/anthropic
```

## Configuration

- **API key:** set `ANTHROPIC_API_KEY` (or the env var used by the Anthropic SDK). This adapter produces `*anthropic.MessageNewParams`; you use the Anthropic SDK client to send requests.
- **Default model:** `New()` uses a default Claude model. Override with `WithModel(anthropic.Model(...))`, or set per execution via `exec.ModelConfig["model"]`.
- **Message metadata:** use `ChatMessage.Metadata["anthropic_cache"]` for prompt caching when supported by the API.

## Capabilities

- **Types:** `Translate` / `TranslateTyped` return `*anthropic.MessageNewParams`; `ParseResponse(ctx, raw)` expects the Anthropic message response type; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant; tools and tool use. **Images:** only base64. For image URLs you must call `exec.ResolveMedia(ctx, fetcher)` before `Translate` (e.g. with `mediafetch.DefaultFetcher{}`); otherwise the adapter returns `adapter.ErrMediaNotResolved`.
- **Tool results:** multimodal `ToolResultPart.Content`; if media in tool results is not supported, the adapter returns `adapter.ErrUnsupportedContentType` when `MediaPart` is present.
- **Helpers:** `adapter.TextFromParts`, `adapter.ExtractModelConfig` for model params from `exec.ModelConfig`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/anthropic) for the full API.
