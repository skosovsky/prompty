package adapter

import (
	"context"
	"errors"
	"iter"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/cast"
)

// ProviderAdapter maps the canonical PromptExecution to a provider-specific request type,
// executes the request via the provider API, and parses the response.
// Req and Resp are the provider SDK types (e.g. *openai.ChatCompletionNewParams, *openai.ChatCompletion).
// Implementations inject the SDK client via provider-specific options (e.g. adapter/openai.WithClient).
type ProviderAdapter[Req any, Resp any] interface {
	// Translate converts PromptExecution into the provider request payload.
	Translate(ctx context.Context, exec *prompty.PromptExecution) (Req, error)
	// Execute performs the API call. The adapter must hold the SDK client (injected via provider options).
	Execute(ctx context.Context, req Req) (Resp, error)
	// ParseResponse converts the raw provider response into canonical *prompty.Response.
	ParseResponse(ctx context.Context, raw Resp) (*prompty.Response, error)
}

// StreamerAdapter is an optional capability for adapters that support native streaming.
// When implemented, LLMClient.GenerateStream uses it; otherwise a polyfill runs Generate and yields one chunk.
type StreamerAdapter[Req any] interface {
	ExecuteStream(ctx context.Context, req Req) iter.Seq2[*prompty.ResponseChunk, error]
}

// Sentinel errors for adapter implementations. Callers should use errors.Is.
var (
	ErrUnsupportedRole              = errors.New("adapter: unsupported message role for this provider")
	ErrUnsupportedContentType       = errors.New("adapter: unsupported ContentPart type for this provider")
	ErrInvalidResponse              = errors.New("adapter: raw response has unexpected type")
	ErrEmptyResponse                = errors.New("adapter: response contains no content")
	ErrNilExecution                 = errors.New("adapter: execution must not be nil")
	ErrMalformedArgs                = errors.New("adapter: tool call args or tool parameters JSON is malformed")
	ErrStreamNotImplemented         = errors.New("adapter: streaming not implemented for this provider")
	ErrStructuredOutputNotSupported = errors.New("adapter: structured output (response_format) not supported by this provider")
	ErrMediaNotResolved             = errors.New("adapter: media URL not resolved (call ResolveMedia first)")
	ErrNoClient                     = errors.New("adapter: SDK client not set (use WithClient)")
)

// ModelParams holds well-known model config keys extracted from PromptExecution.ModelConfig.
// Use ExtractModelConfig to populate from map[string]any.
type ModelParams struct {
	Temperature *float64
	MaxTokens   *int64
	TopP        *float64
	Stop        []string
}

// ExtractModelConfig reads well-known keys from ModelConfig and returns typed ModelParams.
// Well-known keys: "temperature" (float64), "max_tokens" (int64), "top_p" (float64), "stop" ([]string).
func ExtractModelConfig(cfg map[string]any) ModelParams {
	var out ModelParams
	if cfg == nil {
		return out
	}
	if v, ok := cfg["temperature"]; ok {
		if f, ok := cast.ToFloat64(v); ok {
			out.Temperature = &f
		}
	}
	if v, ok := cfg["max_tokens"]; ok {
		if i, ok := cast.ToInt64(v); ok {
			out.MaxTokens = &i
		}
	}
	if v, ok := cfg["top_p"]; ok {
		if f, ok := cast.ToFloat64(v); ok {
			out.TopP = &f
		}
	}
	if v, ok := cfg["stop"]; ok {
		if ss, ok := cast.ToStringSlice(v); ok {
			out.Stop = ss
		}
	}
	return out
}
