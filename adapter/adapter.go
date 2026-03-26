package adapter

import (
	"context"
	"errors"
	"iter"

	"github.com/skosovsky/prompty"
)

// ProviderAdapter maps the canonical PromptExecution to a provider-specific request type,
// executes the request via the provider API, and parses the response.
// Req and Resp are the provider SDK types (e.g. *openai.ChatCompletionNewParams, *openai.ChatCompletion).
// Implementations inject the SDK client via provider-specific options (e.g. adapter/openai.WithClient).
type ProviderAdapter[Req any, Resp any] interface {
	// Translate converts PromptExecution into the provider request payload.
	Translate(exec *prompty.PromptExecution) (Req, error)
	// Execute performs the API call. The adapter must hold the SDK client (injected via provider options).
	Execute(ctx context.Context, req Req) (Resp, error)
	// ParseResponse converts the raw provider response into canonical *prompty.Response.
	ParseResponse(raw Resp) (*prompty.Response, error)
}

// StreamerAdapter is an optional capability for adapters that support native streaming.
// When implemented, Invoker.ExecuteStream uses it; otherwise a polyfill runs Execute and yields one chunk.
type StreamerAdapter[Req any] interface {
	ExecuteStream(ctx context.Context, req Req) iter.Seq2[*prompty.ResponseChunk, error]
}

// Sentinel errors for adapter implementations. Callers should use [errors.Is].
var (
	ErrUnsupportedRole              = errors.New("adapter: unsupported message role for this provider")
	ErrUnsupportedContentType       = errors.New("adapter: unsupported ContentPart type for this provider")
	ErrInvalidResponse              = errors.New("adapter: raw response has unexpected type")
	ErrEmptyResponse                = errors.New("adapter: response contains no content")
	ErrNilExecution                 = errors.New("adapter: execution must not be nil")
	ErrMalformedArgs                = errors.New("adapter: tool call args or tool parameters JSON is malformed")
	ErrStreamNotImplemented         = errors.New("adapter: streaming not implemented for this provider")
	ErrStructuredOutputNotSupported = errors.New(
		"adapter: structured output (response_format) not supported by this provider",
	)
	ErrMediaNotResolved = errors.New("adapter: media URL not resolved (call ResolvedMedia first)")
	ErrNoClient         = errors.New("adapter: SDK client not set (use WithClient)")
)
