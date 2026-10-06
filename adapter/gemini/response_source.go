package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

type responseSourceKey struct{}

const defaultResponsePayloadLimit = 4 << 20

type responseSource struct {
	data     bytes.Buffer
	limit    int
	overflow bool
	complete bool
	status   int
	// Batch operations observe SDK-discarded body-close failures separately.
	closeFailure error
}

func isolatedResponseClient(client *genai.Client) *genai.Client {
	if client == nil {
		return nil
	}
	config := client.ClientConfig()
	if config.HTTPClient == nil {
		return nil
	}
	httpClient := *config.HTTPClient
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpClient.Transport = responseSourceTransport{base: transport}
	config.HTTPClient = &httpClient
	config.HTTPOptions.Headers = config.HTTPOptions.Headers.Clone()
	isolated, err := genai.NewClient(context.Background(), &config)
	if err != nil {
		return nil // SDK initializer errors may contain credentials; do not expose them.
	}
	return isolated
}

type responseSourceTransport struct{ base http.RoundTripper }

func (t responseSourceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	if source, ok := request.Context().Value(batchSourceKey{}).(*responseSource); ok {
		source.status = response.StatusCode
		response.Body = &batchSourceBody{ReadCloser: response.Body, source: source}
		return response, nil
	}
	if source, ok := request.Context().Value(responseSourceKey{}).(*responseSource); ok {
		source.data.Reset()
		source.overflow, source.complete = false, false
		source.status = response.StatusCode
		response.Body = &responseSourceBody{ReadCloser: response.Body, source: source}
	}
	if source, ok := request.Context().Value(streamSourceKey{}).(*streamSource); ok {
		source.payload = nil
		source.status, source.failure = response.StatusCode, nil
		response.Body = &streamSourceBody{
			ReadCloser: response.Body, reader: bufio.NewReader(response.Body), source: source,
		}
	}
	return response, nil
}

type responseSourceBody struct {
	io.ReadCloser

	source *responseSource
}

func (b *responseSourceBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	remaining := b.source.limit - b.source.data.Len()
	kept := min(n, remaining)
	_, _ = b.source.data.Write(data[:kept])
	if kept < n {
		b.source.overflow = true
	}
	if err == io.EOF {
		b.source.complete = true
	}
	return n, err
}

func (a *Adapter) executeObservedResponse(
	ctx context.Context,
	request *Request,
) (*genai.GenerateContentResponse, error) {
	if a.responseClient == nil || a.responseLimit <= 0 {
		return nil, prompty.ErrProviderState
	}
	source := &responseSource{limit: a.responseLimit}
	ctx = context.WithValue(ctx, responseSourceKey{}, source)
	response, err := a.generateObservedContent(ctx, request, source)
	if err != nil && source.complete && source.status >= 200 && source.status < 300 {
		return nil, safeObservedResponseError(ctx, err)
	}
	if err != nil || response == nil {
		return response, err
	}
	if source.complete && !source.overflow {
		metadata := genai.HTTPResponse{}
		if response.SDKHTTPResponse != nil {
			metadata = *response.SDKHTTPResponse
		}
		metadata.Body = source.data.String()
		response.SDKHTTPResponse = &metadata
	} else if hasSignedGeminiResponse(response) {
		return nil, prompty.ErrProviderState
	}
	return response, nil
}

func (a *Adapter) generateObservedContent(
	ctx context.Context,
	request *Request,
	source *responseSource,
) (*genai.GenerateContentResponse, error) {
	var response *genai.GenerateContentResponse
	var err error
	func() {
		defer func() {
			if failure := recover(); failure != nil {
				if !source.complete || source.status < 200 || source.status >= 300 {
					panic(failure) // Do not convert caller transport panics into response errors.
				}
				// SDK converters can panic on malformed provider shapes after reading EOF.
				response, err = nil, safeObservedResponseError(ctx, nil)
			}
		}()
		response, err = a.responseClient.Models.GenerateContent(ctx, request.Model, request.Contents, request.Config)
	}()
	return response, err
}

func safeObservedResponseError(ctx context.Context, sdkError error) error {
	cause := ctx.Err()
	if cause == nil {
		switch {
		case errors.Is(sdkError, context.Canceled):
			cause = context.Canceled
		case errors.Is(sdkError, context.DeadlineExceeded):
			cause = context.DeadlineExceeded
		}
	}
	if cause != nil {
		return errors.Join(prompty.ErrProviderState, cause)
	}
	return prompty.ErrProviderState
}

func hasSignedGeminiResponse(response *genai.GenerateContentResponse) bool {
	for _, candidate := range response.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part != nil && len(part.ThoughtSignature) > 0 {
				return true
			}
		}
	}
	return false
}

func originalSignedGeminiPart(raw *genai.GenerateContentResponse, index int, part *genai.Part) (*genai.Part, error) {
	if part == nil || len(part.ThoughtSignature) == 0 {
		return part, nil
	}
	if raw.SDKHTTPResponse == nil || raw.SDKHTTPResponse.Body == "" {
		if part.FunctionCall != nil && containsSDKFloat(part.FunctionCall.Args) {
			return nil, prompty.ErrProviderState
		}
		return part, nil
	}
	payload := []byte(raw.SDKHTTPResponse.Body)
	if !json.Valid(payload) || uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil {
		return nil, prompty.ErrProviderState
	}
	var document struct {
		Candidates []struct {
			Content struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(payload, &document) != nil || len(document.Candidates) != 1 ||
		len(document.Candidates[0].Content.Parts) != len(raw.Candidates[0].Content.Parts) {
		return nil, prompty.ErrProviderState
	}
	original, err := decodeGeminiState(prompty.PartState{Envelope: prompty.ProviderEnvelope{
		Payload: document.Candidates[0].Content.Parts[index],
	}})
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(part)
	if err != nil {
		return nil, prompty.ErrProviderState
	}
	var sdkValue map[string]any
	originalBytes, err := json.Marshal(original)
	if err != nil || json.Unmarshal(originalBytes, &sdkValue) != nil {
		return nil, prompty.ErrProviderState
	}
	normalized, err := json.Marshal(sdkValue)
	if err != nil || !reflect.DeepEqual(geminiJSONValue(encoded), geminiJSONValue(normalized)) {
		return nil, prompty.ErrProviderState
	}
	return original, nil
}

func containsSDKFloat(value any) bool {
	switch v := value.(type) {
	case float32, float64:
		return true
	case map[string]any:
		for _, child := range v {
			if containsSDKFloat(child) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, containsSDKFloat)
	}
	return false
}
