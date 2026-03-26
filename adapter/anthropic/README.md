# Anthropic adapter for prompty

Maps prompty’s `PromptExecution` to the Anthropic Messages API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/anthropic
```

## Configuration

- **API key:** set `ANTHROPIC_API_KEY` (or the env var used by the Anthropic SDK). This adapter produces `*anthropic.MessageNewParams`; you use the Anthropic SDK client to send requests.
- **Default model:** `New()` uses a default Claude model. Override with `WithModel(anthropic.Model(...))`, or set per execution via `exec.ModelOptions.Model`.
- **Prompt caching:** use `ChatMessage.CachePoint` (or `cache: true` per message in YAML). When `CachePoint == true`, the adapter sets ephemeral cache control for that message. Example in a manifest:

```yaml
messages:
  - role: system
    cache: true   # enables ephemeral cache control in Anthropic
    content: "You are a helpful assistant..."
```

## Capabilities

- **Types:** `Translate` returns `*anthropic.MessageNewParams`; `ParseResponse(raw)` expects the Anthropic message response type; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant; tools and tool use. **Media:** images and PDF documents via base64. For media URLs you must call `exec.ResolvedMedia(ctx, fetcher)` before `Translate` (e.g. with `mediafetch.DefaultFetcher{}`); otherwise the adapter returns `adapter.ErrMediaNotResolved`.
- **Tool results:** multimodal `ToolResultPart.Content`; if media in tool results is not supported, the adapter returns `adapter.ErrUnsupportedContentType` when `MediaPart` is present.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Helpers:** `prompty.TextFromParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/anthropic) for the full API.
