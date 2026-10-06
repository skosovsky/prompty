// Package openai provides a prompty adapter for the OpenAI Chat Completions API.
// Translate returns *openai.ChatCompletionNewParams; ParseResponse expects *openai.ChatCompletion.
// Checked clients require effective endpoint/model knowledge for requested
// guarantees. Native StreamEvents supplies a lazy single-use checked stream;
// SDK retries are disabled and trailing usage precedes lifecycle completion.
// SDK capture is semantic JSON, not HTTP bytes or physical-attempt evidence.
//
// MediaPart: routed by MIME type (image/audio/file). image URL parts use detail "auto".
// Checked cache policy reports unsupported block markers and profile-scoped
// implicit caching knowledge. Raw Translate rejects required unmapped intent.
// MessageProvenance is not mapped to the OpenAI provider API.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package openai
