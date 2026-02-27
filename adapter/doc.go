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
//  1. Translate(exec *prompty.PromptExecution) (any, error)
//     - Map exec.Messages ([]ChatMessage) to the provider's message format.
//     - For messages with RoleTool, each message SHOULD contain exactly one ToolResultPart;
//       some adapters use only the first part when multiple are present.
//     - Map exec.Tools ([]ToolDefinition) to the provider's tool schema.
//     - Apply exec.ModelConfig (e.g. temperature, max_tokens) to the request.
//     - Return the provider's request type; callers will type-assert (e.g. req.(*MySDK.ChatParams)).
//     - Use adapter.ExtractModelConfig(exec.ModelConfig) for well-known keys.
//     - Return adapter.ErrUnsupportedRole or adapter.ErrUnsupportedContentType when the provider
//       does not support a role or ContentPart type (e.g. ImagePart with URL when the SDK requires base64).
//
//  2. ParseResponse(raw any) ([]prompty.ContentPart, error)
//     - Type-assert raw to the provider's response type (e.g. *MySDK.ChatResponse).
//     - Extract text and tool calls; return []prompty.ContentPart (TextPart, ToolCallPart).
//     - Return adapter.ErrInvalidResponse if raw has unexpected type, adapter.ErrEmptyResponse if no content.
//
// Example minimal stub:
//
//	type MyAdapter struct{}
//
//	func (MyAdapter) Translate(exec *prompty.PromptExecution) (any, error) {
//	    params := mypkg.ChatParams{}
//	    for _, msg := range exec.Messages {
//	        // map msg.Role and msg.Content to params.Messages...
//	    }
//	    return &params, nil
//	}
//
//	func (MyAdapter) ParseResponse(raw any) ([]prompty.ContentPart, error) {
//	    resp, ok := raw.(*mypkg.ChatResponse)
//	    if !ok {
//	        return nil, adapter.ErrInvalidResponse
//	    }
//	    return []prompty.ContentPart{prompty.TextPart{Text: resp.Text}}, nil
//	}
//
// Helper functions in this package: TextFromParts (extract text from []ContentPart),
// ExtractModelConfig (typed temperature, max_tokens, top_p, stop from map[string]any).
//
// ImagePart: when both Data and URL are set, Data takes precedence for providers that
// support base64 (OpenAI, Gemini). Anthropic and Ollama do not support image URLs;
// use inline Data only.
package adapter
