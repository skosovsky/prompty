// Package anthropic provides a prompty adapter for the Anthropic Messages API.
// Translate returns *anthropic.MessageNewParams; ParseResponse expects *anthropic.Message.
//
// MediaPart: supports images and PDF documents. Data is sent as base64; when only URL is set,
// callers must resolve media before Translate. Data takes precedence over URL.
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
//
// Tool schema: only "properties" and "required" from ToolDefinition.Parameters are mapped
// to the Anthropic input schema. Other JSON Schema fields (e.g. additionalProperties,
// items, description, oneOf/anyOf) are not supported by the SDK and are omitted.
package anthropic
