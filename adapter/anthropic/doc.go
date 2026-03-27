// Package anthropic provides a prompty adapter for the Anthropic Messages API.
// Translate returns *anthropic.MessageNewParams; ParseResponse expects *anthropic.Message.
//
// MediaPart: image/* is mapped to image blocks (base64 or URL), application/pdf to
// PDF document blocks (base64 or URL), and text/plain to plain-text document blocks
// (base64 only). Data takes precedence over URL. MIMEType is required; adapter does not
// synthesize default MIME values from MediaType.
// CacheControl: message-level cache is the default for all generated blocks; part-level
// cache overrides message-level cache. Anthropic currently supports type "ephemeral".
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
//
// Tool schema: only "properties" and "required" from ToolDefinition.Parameters are mapped
// to the Anthropic input schema. Other JSON Schema fields (e.g. additionalProperties,
// items, description, oneOf/anyOf) are not supported by the SDK and are omitted.
package anthropic
