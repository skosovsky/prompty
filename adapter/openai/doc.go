// Package openai provides a prompty adapter for the OpenAI Chat Completions API.
// Translate returns *openai.ChatCompletionNewParams; ParseResponse expects *openai.ChatCompletion.
// Use TranslateTyped to get the concrete type without a type assertion.
//
// ImagePart: detail is hardcoded to "auto" for image URL parts.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
package openai
