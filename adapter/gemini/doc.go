// Package gemini provides a prompty adapter for the Google Gemini (genai) API.
// Translate returns *gemini.Request (Contents + Config); ParseResponse expects *genai.GenerateContentResponse.
//
// Model: this adapter reads ModelOptions.Model when present and otherwise falls back
// to the adapter's default model.
// MaxOutputTokens is clamped to math.MaxInt32 when max_tokens exceeds int32 range.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package gemini
