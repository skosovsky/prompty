// Package adapter defines the ProviderAdapter interface for mapping prompty's
// canonical PromptExecution to provider-specific request/response types (e.g. OpenAI,
// Anthropic, Gemini, Ollama). Implementations live in provider-specific subpackages
// (adapter/openai, adapter/anthropic, adapter/gemini, adapter/ollama), each as a
// separate Go module so that only the SDK you use is pulled into your dependency graph.
//
// # Implementing a custom adapter
//
// To adapt another LLM SDK to prompty, implement ProviderAdapter in your own package:
//
//  1. Translate(ctx context.Context, exec *prompty.PromptExecution) (any, error)
//     - Map exec.Messages ([]ChatMessage) to the provider's message format.
//     - When mapping messages, read msg.Metadata for provider-specific options (e.g. anthropic_cache, gemini_search_grounding); ignore unknown keys.
//     - For messages with RoleTool, each message SHOULD contain exactly one ToolResultPart;
//     some adapters use only the first part when multiple are present. ToolResultPart.Content
//     is []ContentPart (multimodal). Adapters that do not support media in tool results
//     return ErrUnsupportedContentType when MediaPart is present.
//     - Map exec.Tools ([]ToolDefinition) to the provider's tool schema.
//     - Apply exec.ModelConfig (e.g. temperature, max_tokens) to the request.
//     - Return the provider's request type; callers will type-assert (e.g. req.(*MySDK.ChatParams)).
//     - Use adapter.ExtractModelConfig(exec.ModelConfig) for well-known keys.
//     - Return adapter.ErrUnsupportedRole or adapter.ErrUnsupportedContentType when the provider
//     does not support a role or ContentPart type (e.g. MediaPart with URL when the SDK requires base64).
//     - ctx is part of the interface for cancellation and timeouts; adapters that perform I/O in Translate
//     (e.g. URL fetch for MediaPart) must pass it through. Adapters that do not perform I/O in Translate
//     (e.g. when the provider accepts image URL natively) may leave ctx unused but must accept it for interface consistency.
//
//  2. ParseResponse(ctx context.Context, raw any) ([]prompty.ContentPart, error)
//     - Type-assert raw to the provider's response type (e.g. *MySDK.ChatResponse).
//     - Extract text and tool calls; return []prompty.ContentPart (TextPart, ToolCallPart).
//     - Return adapter.ErrInvalidResponse if raw has unexpected type, adapter.ErrEmptyResponse if no content.
//
//  3. ParseStreamChunk(ctx context.Context, rawChunk any) ([]prompty.ContentPart, error)
//     - Type-assert rawChunk to the provider's streaming chunk type; return incremental content parts.
//     - Return (nil, adapter.ErrStreamNotImplemented) if streaming is not supported.
//
// Example minimal stub:
//
//	type MyAdapter struct{}
//
//	func (MyAdapter) Translate(ctx context.Context, exec *prompty.PromptExecution) (any, error) {
//	    params := mypkg.ChatParams{}
//	    for _, msg := range exec.Messages {
//	        // map msg.Role and msg.Content to params.Messages...
//	    }
//	    return &params, nil
//	}
//
//	func (MyAdapter) ParseResponse(ctx context.Context, raw any) ([]prompty.ContentPart, error) {
//	    resp, ok := raw.(*mypkg.ChatResponse)
//	    if !ok {
//	        return nil, adapter.ErrInvalidResponse
//	    }
//	    return []prompty.ContentPart{prompty.TextPart{Text: resp.Text}}, nil
//	}
//
//	func (MyAdapter) ParseStreamChunk(ctx context.Context, rawChunk any) ([]prompty.ContentPart, error) {
//	    return nil, adapter.ErrStreamNotImplemented
//	}
//
// Helper functions in this package: TextFromParts (extract text from []ContentPart),
// ExtractModelConfig (typed temperature, max_tokens, top_p, stop from map[string]any).
//
// MediaPart: when both Data and URL are set, Data takes precedence for providers that
// support base64 (OpenAI, Gemini). For providers that do not accept URL (Anthropic, Ollama),
// adapters may download the URL in Translate(ctx) and send inline data; see adapter docs.
package adapter
