// Package gemini provides a prompty adapter for the Google Gemini (genai) API.
// Translate returns *gemini.Request (Contents + Config); ParseResponse expects *genai.GenerateContentResponse.
// Caller-owned effective profiles gate requested guarantees. Native streaming
// uses bounded original SSE JSON, complete tool snapshots and no sync fallback.
// Explicit CapabilityIncrementalToolArguments enables Vertex path/value progress;
// signed incremental calls retain verified Vertex source ordering/state; MLDev mapping is unsupported. Candidate citations/grounding are
// message-level annotations, including ordered metadata-only stream updates.
// Ordered signed parts
// use explicit wire/continuation projection. Unknown usage is not known zero;
// unsupported typed-schema fields are rejected rather than silently dropped.
//
// Model: this adapter reads ModelOptions.Model when present and otherwise falls back
// to the adapter's default model.
// MaxOutputTokens is clamped to [math.MaxInt32] when max_tokens exceeds int32 range.
// Checked cache policy reports unsupported block markers and profile-scoped
// implicit caching knowledge. Raw Translate rejects required unmapped intent.
// MessageProvenance is not mapped to the Gemini provider API.
// System-only prompts (no user/assistant messages) receive a synthetic user Content turn;
// Gemini requires non-empty Contents while system text is sent via SystemInstruction.
// Empty function-call args in ParseResponse become "{}"; adapter.ErrMalformedArgs is returned
// only for invalid JSON / marshal errors. ToolCallPart.Args must be valid JSON when non-empty.
package gemini
