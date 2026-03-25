// Package ollama provides a prompty adapter for the Ollama Chat API.
// Translate returns *api.ChatRequest; ParseResponse expects *api.ChatResponse.
//
// MediaPart: only MediaType "image" is supported. When Data is set it is sent inline; when only URL is set,
// callers must resolve media before Translate. Data takes precedence over URL.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
// ToolCall Index is assigned by the adapter from the order of ToolCallPart in the message Content.
// Model options (temperature, max_tokens, top_p, stop) are set on the request's Options map.
package ollama
