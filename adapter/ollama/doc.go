// Package ollama provides a prompty adapter for the Ollama Chat API.
// Translate returns *api.ChatRequest; ParseResponse expects *api.ChatResponse.
// Use TranslateTyped to get the concrete type without a type assertion.
//
// ImagePart: only inline Data ([]byte) is supported; image URLs are rejected.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
// ToolCall Index is assigned by the adapter from the order of ToolCallPart in the message Content.
// Model options (temperature, max_tokens, top_p, stop) are set on the request's Options map.
package ollama
