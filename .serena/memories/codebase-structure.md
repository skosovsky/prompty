# Codebase Structure

- **Root:** core prompty package, go.work
- **adapter/** — OpenAI, Anthropic, Gemini, Ollama adapters
- **remoteregistry/git** — Git remote registry
- **parser/yaml** — YAML manifest parser
- **cmd/prompty-gen** — code generator (consts/types modes)
- **examples/** — basic_chat, secure_prompt, git_prompts, funcmap_tools
- **manifest/** — manifest parsing, testdata
- **.cursor/rules/** — project rules (Go, React, integration)