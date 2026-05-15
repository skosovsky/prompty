// Package gemini provides a prompty adapter for the Google Gemini (genai) API.
// Translate returns *gemini.Request (Contents + Config); ParseResponse expects *genai.GenerateContentResponse.
//
// Model: this adapter reads ModelOptions.Model when present and otherwise falls back
// to the adapter's default model.
// MaxOutputTokens is clamped to [math.MaxInt32] when max_tokens exceeds int32 range.
// CacheControl is accepted and ignored by this adapter in current Gemini APIs.
// System-only prompts (no user/assistant messages) receive a synthetic user Content turn;
// Gemini requires non-empty Contents while system text is sent via SystemInstruction.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package gemini
