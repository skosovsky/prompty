// Package openai provides a prompty adapter for the OpenAI Chat Completions API.
// Translate returns *openai.ChatCompletionNewParams; ParseResponse expects *openai.ChatCompletion.
//
// MediaPart: only MediaType "image" is supported; detail is hardcoded to "auto" for image URL parts.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package openai
