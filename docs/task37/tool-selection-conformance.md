# Native named local-tool selection

ForcedTool is not an instruction hint or the conditional forced-tool structured-output mode. Checked execution infers `CapabilityForcedToolSelection` separately from ordinary tools and requires an actual `ForcedToolAdapter` port. Caller profiles are destination-specific knowledge; overrides cannot add a mapping. Native request selection does not establish account/model support or promise provider infallibility. The execution-scoped helper independently validates all returned calls before any handler.

| Mapping | Evidence | Boundary |
|---|---|---|
| OpenAI named function choice | `openai.go`, `TestTranslate_ForcedToolChoice` in `openai_test.go` | Actual SDK request field; no live forced-selection request claimed |
| Anthropic named tool choice | `anthropic.go`, forced-choice assertion in `anthropic_test.go`; `TestNativeOutputPreservesForcedToolAndSourceSchema` | Actual SDK fields remain independent from native output format; no live claim |
| Gemini ANY + single allowed name | `gemini.go`, `TestTranslate_WithNativeForcedToolSelection`; `TestForcedToolSelectionThroughActualSDK` | Actual SDK/local HTTP sync/buffered wire, one request, unchanged execution, no injected system instruction; native transport is not implemented |
| Ollama unavailable mapping | raw `TestTranslate_ForcedToolRequiresNativeMapping`; checked `TestForcedToolOverrideCannotCreateOllamaMapping` | Typed rejection, including a lying override, without dispatch; ordinary tools are not removed |

`adapter/forced_tool_test.go` tests the universal boundary using synthetic trusted ports: unknown/unsupported profile knowledge and missing mapping fail before Translate/dispatch in all three checked modes. Declining and implemented ports are tested independently of profile knowledge in all modes. These fixtures do not certify a model or turn ordinary tools into forced selection. The shared negative test has a real stream method so native-port absence cannot mask the forced-mapping check.

The [Generate Content function-calling contract](https://ai.google.dev/gemini-api/docs/generate-content/function-calling) documents ANY with allowed function names. Its existence is distinct from local conformance evidence and profile support. No system-prompt fallback, provider-prefix shortcut, automatic replay or agent loop is introduced.
