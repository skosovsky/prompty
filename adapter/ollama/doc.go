// Package ollama provides a prompty adapter for the Ollama Chat API.
// Translate returns *api.ChatRequest; ParseResponse expects *api.ChatResponse.
// Caller-owned effective profiles gate requested guarantees. Native streaming
// uses Chat callbacks. Native Format and Think require explicit effective profiles;
// completed output validates against the original schema. Opaque-state translation is unsupported.
// Explicit buffered streaming preserves available outcome/usage. Tool schemas
// must survive actual SDK serialization without field loss or numeric rounding.
//
// MediaPart: Ollama chat request uses an image-only field (Images), so only image/* user media
// is accepted. For URL-only media (no Data), callers must resolve media before Translate.
// Data takes precedence over URL.
// Checked cache policy reports unsupported block markers and profile-scoped
// implicit caching knowledge. Raw Translate rejects required unmapped intent.
// MessageProvenance is not mapped to the Ollama provider API.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
// ToolCall Index is assigned by the adapter from the order of ToolCallPart in the message Content.
// Native tool arguments are complete snapshots in Args, not ArgsChunk deltas.
// Incoming SDK indices are not presence-aware and never establish identity.
// Model options (temperature, max_tokens, top_p, stop) are set on the request's Options map.
package ollama
