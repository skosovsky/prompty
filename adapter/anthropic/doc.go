// Package anthropic provides a prompty adapter for the Anthropic Messages API.
// Translate returns *anthropic.MessageNewParams; ParseResponse expects *anthropic.Message.
// Use TranslateTyped to get the concrete type without a type assertion.
//
// MediaPart: only MediaType "image" is supported. When Data is set it is sent as base64; when only URL is set,
// the adapter downloads it in Translate(ctx) (respecting ctx, size limit, and image MIME check; only https). Data takes precedence over URL.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
//
// Tool schema: only "properties" and "required" from ToolDefinition.Parameters are mapped
// to the Anthropic input schema. Other JSON Schema fields (e.g. additionalProperties,
// items, description, oneOf/anyOf) are not supported by the SDK and are omitted.
package anthropic
