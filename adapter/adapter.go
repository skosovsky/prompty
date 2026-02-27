package adapter

import (
	"errors"
	"math"
	"strings"

	"github.com/skosovsky/prompty"
)

// ProviderAdapter maps the canonical PromptExecution to a provider-specific request type
// and parses the provider response back to []ContentPart. No implementations in this package.
type ProviderAdapter interface {
	// Translate converts PromptExecution into the provider request payload (e.g. OpenAI chat params).
	// Callers must type-assert the result to the provider-specific type.
	Translate(exec *prompty.PromptExecution) (any, error)
	// ParseResponse converts the raw provider response into canonical content parts.
	ParseResponse(raw any) ([]prompty.ContentPart, error)
}

// Sentinel errors for adapter implementations. Callers should use errors.Is.
var (
	ErrUnsupportedRole        = errors.New("adapter: unsupported message role for this provider")
	ErrUnsupportedContentType = errors.New("adapter: unsupported ContentPart type for this provider")
	ErrInvalidResponse        = errors.New("adapter: raw response has unexpected type")
	ErrEmptyResponse          = errors.New("adapter: response contains no content")
	ErrNilExecution           = errors.New("adapter: execution must not be nil")
	ErrMalformedArgs          = errors.New("adapter: tool call args or tool parameters JSON is malformed")
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
		if f, ok := toFloat64(v); ok {
			out.Temperature = &f
		}
	}
	if v, ok := cfg["max_tokens"]; ok {
		if i, ok := toInt64(v); ok {
			out.MaxTokens = &i
		}
	}
	if v, ok := cfg["top_p"]; ok {
		if f, ok := toFloat64(v); ok {
			out.TopP = &f
		}
	}
	if v, ok := cfg["stop"]; ok {
		if ss, ok := toStringSlice(v); ok {
			out.Stop = ss
		}
	}
	return out
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case int16:
		return float64(x), true
	case int8:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int16:
		return int64(x), true
	case int8:
		return int64(x), true
	case uint:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		if x > math.MaxInt64 {
			return math.MaxInt64, true
		}
		return int64(x), true
	case float64:
		return int64(x), true
	case float32:
		return int64(x), true
	default:
		return 0, false
	}
}

func toStringSlice(v any) ([]string, bool) {
	if ss, ok := v.([]string); ok {
		return ss, true
	}
	slice, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(slice))
	for _, e := range slice {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
