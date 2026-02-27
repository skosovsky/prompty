// Package gemini provides a prompty adapter for the Google Gemini (genai) API.
// Translate returns *gemini.Request (Contents + Config); ParseResponse expects *genai.GenerateContentResponse.
// Use TranslateTyped to get the concrete type without a type assertion.
//
// Model: the Gemini SDK sets the model on the client, not per-request. This adapter
// does not read "model" from exec.ModelConfig; use the genai client's model when calling the API.
// MaxOutputTokens is clamped to math.MaxInt32 when max_tokens exceeds int32 range.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package gemini
