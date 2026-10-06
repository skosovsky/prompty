package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty/adapter"
)

// ChatClient is the injected Ollama operation, not a core transport or agent loop.
// Implementations preserve exact numeric arguments as [json.Number], never float.
// The SDK implements this port, but its unverified float arguments are rejected.
type ChatClient interface {
	Chat(ctx context.Context, request *api.ChatRequest, consume api.ChatResponseFunc) error
}

// HTTPChatClient owns immutable endpoint/client configuration and no credentials,
// queue or response store. Host transport supplies auth and egress policy.
type HTTPChatClient struct {
	endpoint url.URL
	client   http.Client
	maxFrame int
}

// NewHTTPChatClient creates a number-preserving chat transport with a positive
// per-frame byte limit. HTTP client configuration is copied, never modified.
// It does not inherit SDK automatic authentication; the host supplies auth.
func NewHTTPChatClient(endpoint *url.URL, client *http.Client, maxFrame int) (*HTTPChatClient, error) {
	if endpoint == nil || client == nil || maxFrame <= 0 || endpoint.Host == "" ||
		(endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return nil, adapter.ErrInvalidResponse
	}
	return &HTTPChatClient{endpoint: *endpoint, client: *client, maxFrame: maxFrame}, nil
}

// Chat performs exactly one explicitly requested call, closing its body on every
// return. A callback error stops reading immediately, including consumer break.
func (c *HTTPChatClient) Chat(ctx context.Context, request *api.ChatRequest, consume api.ChatResponseFunc) error {
	if c == nil {
		return adapter.ErrNoClient
	}
	if request == nil || consume == nil {
		return adapter.ErrNilExecution
	}
	data, err := json.Marshal(request)
	if err != nil {
		return adapter.ErrMalformedArgs
	}
	endpoint := c.endpoint.JoinPath("api/chat")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return adapter.ErrInvalidResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &ChatHTTPError{StatusCode: resp.StatusCode}
	}
	reader := bufio.NewReader(resp.Body)
	for {
		frame, readErr := readChatFrame(reader, c.maxFrame)
		if len(bytes.TrimSpace(frame)) > 0 {
			response, decodeErr := decodeChatFrame(frame)
			if decodeErr != nil {
				return decodeErr
			}
			if consumeErr := consume(response); consumeErr != nil {
				return consumeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, errChatEOF) {
				return nil
			}
			return readErr
		}
	}
}

// ChatHTTPError reports observed HTTP status without retaining provider body.
type ChatHTTPError struct{ StatusCode int }

func (e *ChatHTTPError) Error() string {
	return fmt.Sprintf("ollama: chat HTTP status %d", e.StatusCode)
}

func decodeChatFrame(frame []byte) (api.ChatResponse, error) {
	var empty api.ChatResponse
	var root map[string]json.RawMessage
	if json.Unmarshal(frame, &root) != nil || root == nil {
		return empty, adapter.ErrInvalidResponse
	}
	if _, failed := root["error"]; failed {
		return empty, adapter.ErrInvalidResponse
	}
	var message map[string]json.RawMessage
	var calls []map[string]json.RawMessage
	if raw := root["message"]; len(raw) != 0 {
		if json.Unmarshal(raw, &message) != nil || message == nil {
			return empty, adapter.ErrInvalidResponse
		}
		if rawCalls := message["tool_calls"]; len(rawCalls) != 0 && json.Unmarshal(rawCalls, &calls) != nil {
			return empty, adapter.ErrInvalidResponse
		}
	}
	// Decode the ordinary SDK fields without ever sending argument numbers through
	// the SDK's float-based ordered-map decoder.
	if len(calls) != 0 {
		message["tool_calls"] = json.RawMessage("[]")
		encoded, _ := json.Marshal(message)
		root["message"] = encoded
	}
	data, err := json.Marshal(root)
	if err != nil || json.Unmarshal(data, &empty) != nil {
		return api.ChatResponse{}, adapter.ErrInvalidResponse
	}
	for _, raw := range calls {
		var function map[string]json.RawMessage
		if json.Unmarshal(raw["function"], &function) != nil || function == nil {
			return api.ChatResponse{}, adapter.ErrMalformedArgs
		}
		arguments, argsErr := decodeExactToolArguments(function["arguments"])
		if argsErr != nil {
			return api.ChatResponse{}, argsErr
		}
		function["arguments"] = json.RawMessage("{}")
		encoded, _ := json.Marshal(function)
		raw["function"] = encoded
		encoded, _ = json.Marshal(raw)
		var call api.ToolCall
		if json.Unmarshal(encoded, &call) != nil {
			return api.ChatResponse{}, adapter.ErrMalformedArgs
		}
		call.Function.Arguments = arguments
		empty.Message.ToolCalls = append(empty.Message.ToolCalls, call)
	}
	return empty, nil
}
