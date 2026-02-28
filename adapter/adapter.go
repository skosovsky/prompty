package adapter

import (
	"context"
	"errors"
	"strings"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/cast"
)

// ProviderAdapter maps the canonical PromptExecution to a provider-specific request type
// and parses the provider response back to []ContentPart. No implementations in this package.
type ProviderAdapter interface {
	// Translate converts PromptExecution into the provider request payload (e.g. OpenAI chat params).
	// Callers must type-assert the result to the provider-specific type. ctx is used for timeouts and cancellation.
	Translate(ctx context.Context, exec *prompty.PromptExecution) (any, error)
	// ParseResponse converts the raw provider (unary) response into canonical content parts.
	ParseResponse(ctx context.Context, raw any) ([]prompty.ContentPart, error)
	// ParseStreamChunk parses a single stream chunk (e.g. SSE). Return ErrStreamNotImplemented if not supported.
	ParseStreamChunk(ctx context.Context, rawChunk any) ([]prompty.ContentPart, error)
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
)

// ModelParams holds well-known model config keys extracted from PromptExecution.ModelConfig.
// Use ExtractModelConfig to populate from map[string]any.
type ModelParams struct {
	Temperature *float64
	MaxTokens   *int64
	TopP        *float64
	Stop        []string
}

// TextFromParts extracts concatenated text from []ContentPart, ignoring non-text parts.
func TextFromParts(parts []prompty.ContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if t, ok := p.(prompty.TextPart); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
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
