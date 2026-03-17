# Prompty Project Overview

## Purpose

prompty is a Go library for prompt management, templating, and unified interaction with LLMs. It supports loading prompts from files, Git, or HTTP and works with multiple backends (OpenAI, Anthropic, Gemini, Ollama) without locking to a single vendor.

## Tech Stack

- **Language:** Go 1.26
- **Modules:** Go Workspaces (go.work) with multiple submodules
- **Templating:** text/template with fail-fast validation
- **Manifests:** JSON and YAML (via parser/yaml)
- **Adapters:** OpenAI, Anthropic, Gemini, Ollama
- **Registries:** fileregistry, embedregistry, remoteregistry (Git/HTTP)

## Key Abstractions

- **Registry** — supplies ChatPromptTemplate by id
- **Adapter** — maps PromptExecution to provider request
- **Pipeline:** Registry → Template + payload → PromptExecution → Adapter → LLM API