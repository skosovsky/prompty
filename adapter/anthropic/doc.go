// Package anthropic provides checked Messages, explicit synthetic output and Message Batches ports.
// Versioned hosted search/fetch retain scoped source history across sync/native/batch.
// OutcomePaused requires explicit caller continuation and never authorizes local handlers.
// The separate synthetic profile does not implement hosted tools or native streaming.
// Translate returns *anthropic.MessageNewParams; ParseResponse expects *anthropic.Message.
// Caller-owned effective profiles gate native output, reasoning, cache controls
// and combinations. StreamEvents supplies native Messages SSE; use explicit buffered
// mode when synchronous execution is acceptable. Signed/redacted thinking uses
// scoped envelopes, explicit wire policy and AssistantMessage continuation.
//
// MediaPart: image/* is mapped to image blocks (base64 or URL), application/pdf to
// PDF document blocks (base64 or URL), and text/plain to plain-text document blocks
// (base64 only). Data takes precedence over URL. MIMEType is required; adapter does not
// synthesize default MIME values from MediaType.
// CachePolicy: message-level cache is the default for all generated blocks; part-level
// cache overrides message-level cache. Block markers map to ephemeral controls with
// optional provider-scoped TTL "5m"/"1h". Checked preflight requires model-profile
// knowledge; diagnostics describe selected controls, never a cache hit.
// MessageProvenance is not mapped to the Anthropic provider API (observability/wire only).
// ToolCallPart.Args must be valid JSON when non-empty; otherwise adapter.ErrMalformedArgs is returned.
//
// Tool schemas preserve their original fields through the SDK extra-field mapping;
// checked execution compiles and validates the original canonical contract.
package anthropic
