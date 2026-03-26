# OpenAI adapter for prompty

Maps prompty’s `PromptExecution` to the OpenAI Chat Completions API and parses responses back to `[]prompty.ContentPart`.

## Install

```bash
go get github.com/skosovsky/prompty/adapter/openai
```

## Configuration

- **API key:** set `OPENAI_API_KEY` in the environment, or pass it when creating the OpenAI client (e.g. `option.WithAPIKey(key)` from `github.com/openai/openai-go/v3/option`). This adapter only produces request params; you use the official `openai-go` client to send requests.
- **Default model:** `New()` uses `gpt-4o`. Override with `WithModel(shared.ChatModel(...))`. You can also set the model per execution via `exec.ModelOptions.Model`.
- **HTTP client:** configured on the OpenAI client you create, not on the adapter.

## Capabilities

- **Types:** `Translate` returns `*openai.ChatCompletionNewParams`; `ParseResponse(raw)` expects `*openai.ChatCompletion`; streaming uses `ExecuteStream` via `StreamerAdapter`.
- **Messages:** system, user, assistant; text and tool calls. `MediaPart` is routed by MIME type: `image/*` (URL/base64), `audio/*` (inline input audio), other MIME types as inline file blocks.
- **Tools:** tool definitions and tool call/result mapping; tool results can be multimodal (`ToolResultPart.Content` as `[]ContentPart`); if the adapter does not support media in tool results, it returns `adapter.ErrUnsupportedContentType` when `MediaPart` is present.
- **Model options:** `exec.ModelOptions` maps `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop` into the request.
- **Helpers:** With NewClient+Execute use `resp.Text()`. With direct Translate/Execute/ParseResponse use `prompty.TextFromParts(resp.Content)`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/adapter/openai) for the full API.
