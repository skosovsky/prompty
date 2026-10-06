package gemini

import (
	"encoding/json"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

// exactGenerationRequest restores checked native contents after the SDK's
// float64 normalization, immediately before its final JSON encoder.
func exactGenerationRequest(request *Request) (*Request, error) {
	copied := copyNativeRequest(request)
	if copied.Config == nil {
		copied.Config = &genai.GenerateContentConfig{}
	}
	options, err := exactContentOptions(copied.Config.HTTPOptions, copied.Contents, "contents")
	if err != nil {
		return nil, err
	}
	copied.Config.HTTPOptions = options
	return copied, nil
}

func exactContentOptions(
	options *genai.HTTPOptions,
	contents []*genai.Content,
	key string,
) (*genai.HTTPOptions, error) {
	encoded, err := encodeExactGeminiContents(contents)
	if err != nil {
		return nil, prompty.ErrProviderState
	}
	// The SDK performs this conversion before the callback can restore numbers.
	var sdkShape any
	if err := json.Unmarshal(encoded, &sdkShape); err != nil {
		return nil, prompty.ErrProviderState
	}
	copied := &genai.HTTPOptions{}
	if options != nil {
		*copied = *options
		copied.Headers = options.Headers.Clone()
	}
	previous := copied.ExtrasRequestProvider
	copied.ExtrasRequestProvider = func(body map[string]any) map[string]any {
		if previous != nil {
			body = previous(body)
		}
		if body == nil {
			return map[string]any{"invalid_content_encoding": json.Number("invalid")}
		}
		body[key] = geminiJSONValue(encoded)
		return body
	}
	return copied, nil
}
