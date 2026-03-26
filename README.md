# prompty

[![Go Reference](https://pkg.go.dev/badge/github.com/skosovsky/prompty.svg)](https://pkg.go.dev/github.com/skosovsky/prompty)

**TL;DR** — prompty is a library for prompt management, templating, and unified interaction with LLMs in Go. It supports loading prompts from files, Git, or HTTP and works with multiple backends (OpenAI, Anthropic, Gemini, Ollama) without locking you to a single vendor.

## Modules and installation

The project is split into multiple Go modules. Install only what you need:

| Layer   | Package | Install |
|---------|---------|---------|
| Core    | prompty (templates, registries in-tree) | `go get github.com/skosovsky/prompty` |
| Adapters | OpenAI  | `go get github.com/skosovsky/prompty/adapter/openai` |
|         | Gemini  | `go get github.com/skosovsky/prompty/adapter/gemini` |
|         | Anthropic | `go get github.com/skosovsky/prompty/adapter/anthropic` |
|         | Ollama  | `go get github.com/skosovsky/prompty/adapter/ollama` |
| Registries | Git (remote) | `go get github.com/skosovsky/prompty/remoteregistry/git` |

`fileregistry` and `embedregistry` are part of the core module (`github.com/skosovsky/prompty`).

## Quick Start

Minimal example: create an `Invoker` from an adapter, format a prompt, call `Execute`, and read the response text. Requires `OPENAI_API_KEY` in the environment.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

func main() {
	adp := openaiadapter.New(openaiadapter.WithClient(
		openai.NewClient(option.WithAPIKey(os.Getenv("OPENAI_API_KEY")))),
	)
	client := adapter.NewClient(adp)

	exec := prompty.SimpleChat("You are a helpful assistant.", "What is 2+2?")
	resp, err := client.Execute(context.Background(), exec)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text())
}
```

## Main abstractions

- **Registry** — supplies `ChatPromptTemplate` by id (from files, embed, or remote). Interface: `GetTemplate(ctx, id) (*ChatPromptTemplate, error)`.
- **Adapter** — maps `PromptExecution` to a provider request and parses the response. Recommended: `adapter.NewClient(providerAdapter)` → `client.Execute(ctx, exec)` → `resp.Text()`. Low-level: `Translate` → `Execute` → `ParseResponse`. For streaming use `ExecuteStream`; adapters implement `StreamerAdapter.ExecuteStream` for native streaming.
- **Templating** — `ChatPromptTemplate` is built from message templates and optional tools; you pass a typed payload (struct with `prompt` tags) to `FormatStruct(payload)` to get a `PromptExecution`. Registries can load manifests (JSON or YAML) and support `WithPartials` for shared `{{ template "name" }}` partials. Template functions (funcmaps) include `truncate_chars`, `truncate_tokens`, `render_tools_as_xml`, `render_tools_as_json`, `escapeXML`, and `randomHex`.

Pipeline: **Registry** → **Template** + payload → **PromptExecution** → **Adapter** → provider API. HTTP/transport is the caller’s responsibility.

## Features

- **Domain model**: `ContentPart` (text/media/tool call/result), `ChatMessage`, `ToolDefinition`, `PromptExecution` with metadata; open-ended roles in manifests (validation in adapters). **Message-level:** prompt caching uses `ChatMessage.CachePoint` (or `cache: true` per message in YAML). **Execution-level provider knobs:** use `PromptExecution.ModelOptions.ProviderSettings` (e.g. `gemini_search_grounding` for Gemini).
- **Media**: `exec.ResolvedMedia(ctx, fetcher)` returns a cloned execution with `MediaPart.Data` filled via a `Fetcher` (e.g. `mediafetch.DefaultFetcher{}`); use it before `Translate` for adapters that require inline data (Anthropic, Ollama). OpenAI and Gemini accept URL natively.
- **Templating**: `text/template` with fail-fast validation, `PartialVariables`, optional messages, chat history splicing. **DRY:** registries support `WithPartials(pattern)` so manifests can use `{{ template "name" }}` with shared partials (e.g. `_partials/*.tmpl`).
- **Template functions**: `truncate_chars`, `truncate_tokens`, `render_tools_as_xml` / `render_tools_as_json` for tool injection.
- **Registries**: load manifests from filesystem (`fileregistry`), embed (`embedregistry`), or remote HTTP/Git (`remoteregistry`). Remote cache is explicit via `remoteregistry.WithCache(...)`.
- **Adapters**: map `PromptExecution` to provider request types (OpenAI, Anthropic, Gemini, Ollama); parse responses back to `[]ContentPart`. Tool result is multimodal: `ToolResultPart.Content` is `[]ContentPart` (text and/or images). Adapters that do not support media in tool results return `ErrUnsupportedContentType` when `MediaPart` is present in `ToolResultPart.Content`.
- **Observability**: `PromptMetadata` (ID, version, description, tags, environment) on every execution.

## Registries

| Package | Description |
|---------|-------------|
| `github.com/skosovsky/prompty/fileregistry` | Load manifests (JSON or YAML via WithParser) from a directory; lazy load with cache; `Reload()` to clear cache; `WithPartials(relativePattern)` for `{{ template "name" }}` |
| `github.com/skosovsky/prompty/embedregistry` | Load from `embed.FS` at build time; eager load; no mutex; `WithPartials(pattern)` for shared partials |
| `github.com/skosovsky/prompty/remoteregistry` | Fetch via `Fetcher` (HTTP or Git); explicit cache via `WithCache`; `Close()` for resource cleanup |

All three registries also implement optional `prompty.Lister` (`List(ctx)`) and `prompty.Statter` (`Stat(ctx, id)`). When you have a variable of type `prompty.Registry` and need to list IDs or get template metadata, use a type assertion: `if l, ok := reg.(prompty.Lister); ok { ids, err := l.List(ctx); ... }`.

Template name and environment resolve to `{name}.{env}.json`, `{name}.{env}.yaml` (or `.yml`), with fallback to `{name}.json`, `{name}.yaml`. Name must not contain `':'`.

## Adapters

| Package | Translate result | Notes |
|---------|------------------|--------|
| `github.com/skosovsky/prompty/adapter/openai` | `*openai.ChatCompletionNewParams` | Tools, MIME-routed media (image/audio/file), tool calls |
| `github.com/skosovsky/prompty/adapter/anthropic` | `*anthropic.MessageNewParams` | Images and PDF as base64 |
| `github.com/skosovsky/prompty/adapter/gemini` | `*gemini.Request` | Default model + overrides (`WithModel`, `ModelOptions.Model`); generic media URI/bytes |
| `github.com/skosovsky/prompty/adapter/ollama` | `*api.ChatRequest` | Native Ollama tools |

Each adapter implements `Translate(exec) (Req, error)` where Req is the provider request type; `ParseResponse(raw)` returns `*prompty.Response`; use `resp.Text()` for plain text. `PromptExecution.ModelOptions` carries typed model overrides such as `Model`, `Temperature`, `MaxTokens`, `TopP`, and `Stop`. **Tool result:** `ToolResultPart.Content` is `[]ContentPart` (multimodal). Adapters that do not support media in tool results return `adapter.ErrUnsupportedContentType` when `MediaPart` is present. **Media:** OpenAI and Gemini can map URL media natively for supported types. For Anthropic and Ollama (base64-only request bodies), call `exec.ResolvedMedia(ctx, fetcher)` before `Translate` when using media URLs; pass a `Fetcher` (e.g. `mediafetch.DefaultFetcher{}` or a custom implementation). Otherwise the adapter returns `adapter.ErrMediaNotResolved`. The core has no HTTP dependency; the default implementation lives in `mediafetch`.

## Architecture

```mermaid
flowchart LR
    Registry[Registry]
    Template[ChatPromptTemplate]
    Format[FormatStruct]
    Exec[PromptExecution]
    Adapter[ProviderAdapter]
    API[LLM API]

    Registry -->|GetTemplate| Template
    Template -->|FormatStruct + payload| Format
    Format --> Exec
    Exec -->|Translate| Adapter
    Adapter -->|request| API
    API -->|raw response| Adapter
    Adapter -->|ParseResponse| Response["*Response"]
```

Pipeline: **Registry** → **Template** + typed payload → **Fail-fast validation** → **Rendering** (with tool injection) → **PromptExecution** → **Adapter** → LLM API. HTTP/transport is the caller’s responsibility.

## Template functions

- `truncate_chars .text 4000` — trim by rune count
- `truncate_tokens .text 2000` — trim by token count (uses `TokenCounter` from template options; default `CharFallbackCounter`)
- `render_tools_as_xml .Tools` / `render_tools_as_json .Tools` — inject tool definitions into the prompt (e.g. for local Llama)
- `escapeXML` — escape `<`, `>`, `&`, `"`, `'` so user input does not break XML structure (see Prompt Security)
- `randomHex N` — cryptographically random hex string of N bytes (2N chars); for randomized delimiters

## Prompt Security (Data Isolation)

To prevent prompt injection (e.g. user input closing a `<patient_input>` tag), escape user content and use randomized delimiters so the model cannot guess them. Example in a message template:

```yaml
{{ $delim := randomHex 8 }}
<data_{{ $delim }}>
{{ .UserInput | escapeXML }}
</data_{{ $delim }}>
```

- **escapeXML** — uses `html.EscapeString`; keeps user text from being interpreted as markup.
- **randomHex** — e.g. `randomHex 8` yields a 16-character hex string; use in opening and closing tags so the delimiter is unpredictable.

See `examples/secure_prompt` for a runnable example.

## Development

This repo uses **Go Workspaces** (`go.work`). The root and all adapter/registry submodules must be listed there so that changes to the core `prompty` package and adapters compile together in one PR without publishing intermediate versions.

**Build and test** (from repo root):

```bash
go work sync
go build ./...
go test ./...
cd adapter/openai && go build . && go test . && cd ../..
cd adapter/anthropic && go build . && go test . && cd ../..
cd adapter/gemini && go build . && go test . && cd ../..
cd adapter/ollama && go build . && go test . && cd ../..
cd remoteregistry/git && go build . && go test . && cd ../..
```

Ensure `go.work` includes: `.`, `./adapter/openai`, `./adapter/anthropic`, `./adapter/gemini`, `./adapter/ollama`, `./remoteregistry/git`.

**Benchmarks:** the library is optimized for zero-allocation rendering (sync.Pool). To check allocs/op and B/op (and ensure PRs do not regress them), run:

```bash
go test -bench=BenchmarkFormatStruct -benchmem ./...
```

**Running examples locally:** `go.work` includes `./examples/basic_chat`, `./examples/git_prompts`, `./examples/funcmap_tools`, and `./examples/secure_prompt`. From the repo root run `go run ./examples/basic_chat` (or `go run ./examples/secure_prompt` for the data-isolation example). Or `cd` into an example dir and `go run .`. The secure_prompt example embeds its manifest and works from any working directory; it demonstrates both escapeXML and randomHex (randomized delimiters). Each example’s `go.mod` uses `replace` for local development; remove those when using a published module.

## License

MIT. See [LICENSE](LICENSE).
