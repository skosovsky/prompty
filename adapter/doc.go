// Package adapter defines the ProviderAdapter interface for mapping prompty's
// canonical PromptExecution to provider-specific request/response types (e.g. OpenAI,
// Anthropic, Gemini, Ollama). Implementations live in provider-specific subpackages
// (adapter/openai, adapter/anthropic, adapter/gemini, adapter/ollama), each as a
// separate Go module so that only the SDK you use is pulled into your dependency graph.
//
// # Usage
//
// Create an LLMClient from an adapter using NewClient, then call Generate or GenerateStream:
//
//	adp := openaiadapter.New(openaiadapter.WithClient(openaisdk.NewClient(...)))
//	client := adapter.NewClient(adp)
//	resp, err := client.Generate(ctx, exec)
//	fmt.Println(resp.Text())
//
// # Implementing a custom adapter
//
// Implement ProviderAdapter[Req, Resp] with three methods:
//
//  1. Translate(ctx, exec) (Req, error)
//     - Map exec.Messages ([]ChatMessage) to the provider's message format.
//     - Map exec.Tools, exec.ModelConfig. Use ExtractModelConfig for well-known keys.
//     - Return adapter.ErrUnsupportedRole or ErrUnsupportedContentType when unsupported.
//
//  2. Execute(ctx, req) (Resp, error)
//     - Call the provider API. Inject the SDK client via provider options (e.g. openai.WithClient).
//     - Return ErrNoClient if client was not set.
//
//  3. ParseResponse(ctx, raw Resp) (*prompty.Response, error)
//     - Build *prompty.Response{Content, Usage} from the provider's response.
//     - Return ErrInvalidResponse or ErrEmptyResponse on invalid/empty content.
//
// Optional: implement StreamerAdapter[Req] with ExecuteStream(ctx, req) for native streaming.
// If not implemented, GenerateStream falls back to a polyfill (one chunk from Generate).
//
// Helper functions: prompty.TextFromParts (extract text from []ContentPart),
// ExtractModelConfig (temperature, max_tokens, top_p, stop from map[string]any).
//
// MediaPart: when both Data and URL are set, Data takes precedence for providers that
// support base64 (OpenAI, Gemini). For providers that do not accept URL (Anthropic, Ollama),
// adapters may download the URL in Translate(ctx) and send inline data.
package adapter
