// Package openai provides a prompty adapter for the OpenAI Chat Completions API.
// Translate returns *openai.ChatCompletionNewParams; ParseResponse expects *openai.ChatCompletion.
//
// MediaPart: routed by MIME type (image/audio/file). image URL parts use detail "auto".
// CacheControl is accepted and ignored by this adapter in current OpenAI APIs.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package openai
