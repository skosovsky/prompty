# Anthropic adapter for prompty

Maps prompty’s `PromptExecution` to the Anthropic Messages API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/anthropic
```

## Configuration

- **API key:** set `ANTHROPIC_API_KEY` (or the env var used by the Anthropic SDK). This adapter produces `*anthropic.MessageNewParams`; you use the Anthropic SDK client to send requests.
- **Default model:** `New()` uses a default Claude model. Override with `WithModel(anthropic.Model(...))`, or set per execution via `exec.ModelOptions.Model`.
- **Prompt caching:** use `ChatMessage.CacheControl` and optional part-level `CacheControl`. Message-level cache is used by default; part-level cache overrides it. Anthropic currently supports `type: ephemeral`. Example in a manifest:

```yaml
messages:
  - role: system
    cache_control:
      type: ephemeral
    content: "You are a helpful assistant..."
```

## Capabilities

- **Types:** `Translate` returns `*anthropic.MessageNewParams`; `ParseResponse(raw)` expects the Anthropic message response type; `ParseStreamChunk` if supported, or `adapter.ErrStreamNotImplemented`.
- **Messages:** system, user, assistant; tools and tool use. **Media:** `image/*` maps to image blocks (base64 or URL), `application/pdf` maps to PDF document blocks (base64 or URL), and `text/plain` maps to plain-text document blocks (base64 only). `MediaPart.MIMEType` is required for media translation; unsupported or missing MIME types return `adapter.ErrUnsupportedContentType`.
- **Tool results:** multimodal `ToolResultPart.Content` supports text and media blocks.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Helpers:** `prompty.TextFromParts`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/anthropic) for the full API.
