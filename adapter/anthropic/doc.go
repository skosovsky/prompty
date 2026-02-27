// Package anthropic provides a prompty adapter for the Anthropic Messages API.
// Translate returns *anthropic.MessageNewParams; ParseResponse expects *anthropic.Message.
// Use TranslateTyped to get the concrete type without a type assertion.
//
// ImagePart: only base64 Data is supported; URL is rejected. When both Data and URL are set, Data takes precedence.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
//
// Tool schema: only "properties" and "required" from ToolDefinition.Parameters are mapped
// to the Anthropic input schema. Other JSON Schema fields (e.g. additionalProperties,
// items, description, oneOf/anyOf) are not supported by the SDK and are omitted.
package anthropic
