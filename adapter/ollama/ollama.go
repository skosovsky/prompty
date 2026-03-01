package ollama

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Adapter implements adapter.ProviderAdapter for the Ollama Chat API.
// Translate returns *api.ChatRequest; ParseResponse expects *api.ChatResponse.
type Adapter struct {
	defaultModel string
}

// Option configures an Adapter (e.g. WithModel).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelConfig does not contain "model".
func WithModel(m string) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// New returns an Adapter with default model set to "llama3.2". Options can override the default model.
func New(opts ...Option) *Adapter {
	a := &Adapter{defaultModel: "llama3.2"}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Translate converts PromptExecution into *api.ChatRequest.
func (a *Adapter) Translate(ctx context.Context, exec *prompty.PromptExecution) (any, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	return a.TranslateTyped(ctx, exec)
}

// TranslateTyped returns the concrete type so callers avoid type assertion.
func (a *Adapter) TranslateTyped(ctx context.Context, exec *prompty.PromptExecution) (*api.ChatRequest, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	if exec.ResponseFormat != nil {
		return nil, adapter.ErrStructuredOutputNotSupported
	}
	model := a.defaultModel
	if exec.ModelConfig != nil {
		if m, ok := exec.ModelConfig["model"].(string); ok && m != "" {
			model = m
		}
	}
	req := &api.ChatRequest{
		Model:    model,
		Messages: make([]api.Message, 0, len(exec.Messages)),
	}
	if exec.ModelConfig != nil {
		mp := adapter.ExtractModelConfig(exec.ModelConfig)
		if mp.Temperature != nil || mp.MaxTokens != nil || mp.TopP != nil || len(mp.Stop) > 0 {
			req.Options = make(map[string]any)
			if mp.Temperature != nil {
				req.Options["temperature"] = *mp.Temperature
			}
			if mp.MaxTokens != nil {
				req.Options["num_predict"] = *mp.MaxTokens
			}
			if mp.TopP != nil {
				req.Options["top_p"] = *mp.TopP
			}
			if len(mp.Stop) > 0 {
				req.Options["stop"] = mp.Stop
			}
		}
	}
	for _, msg := range exec.Messages {
		m, err := a.translateMessage(ctx, msg)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, m...)
	}
	if len(exec.Tools) > 0 {
		req.Tools = make(api.Tools, 0, len(exec.Tools))
		for _, t := range exec.Tools {
			tool, err := a.translateTool(t)
			if err != nil {
				return nil, err
			}
			req.Tools = append(req.Tools, tool)
		}
	}
	return req, nil
}

func (a *Adapter) translateMessage(_ context.Context, msg prompty.ChatMessage) ([]api.Message, error) {
	switch msg.Role {
	case prompty.RoleSystem, prompty.RoleDeveloper:
		for _, p := range msg.Content {
			if _, ok := p.(prompty.MediaPart); ok {
				return nil, fmt.Errorf("%w: Ollama does not support images in system messages", adapter.ErrUnsupportedContentType)
			}
		}
		text := adapter.TextFromParts(msg.Content)
		return []api.Message{{Role: "system", Content: text}}, nil
	case prompty.RoleUser:
		var images []api.ImageData
		for _, p := range msg.Content {
			if img, ok := p.(prompty.MediaPart); ok && img.MediaType == "image" {
				data := img.Data
				if len(data) == 0 && img.URL != "" {
					return nil, fmt.Errorf("%w", adapter.ErrMediaNotResolved)
				}
				if len(data) == 0 {
					return nil, fmt.Errorf("%w: MediaPart has neither Data nor URL", adapter.ErrUnsupportedContentType)
				}
				images = append(images, api.ImageData(data))
			}
		}
		text := adapter.TextFromParts(msg.Content)
		return []api.Message{{Role: "user", Content: text, Images: images}}, nil
	case prompty.RoleAssistant:
		for _, p := range msg.Content {
			if _, ok := p.(prompty.MediaPart); ok {
				return nil, fmt.Errorf("%w: Ollama does not support images in assistant messages", adapter.ErrUnsupportedContentType)
			}
		}
		var toolCalls []api.ToolCall
		var tcIndex int
		for _, p := range msg.Content {
			if tc, ok := p.(prompty.ToolCallPart); ok {
				var args api.ToolCallFunctionArguments
				if tc.Args != "" {
					if err := json.Unmarshal([]byte(tc.Args), &args); err != nil {
						return nil, fmt.Errorf("%w: invalid tool call args JSON: %w", adapter.ErrMalformedArgs, err)
					}
				}
				toolCalls = append(toolCalls, api.ToolCall{
					ID: tc.ID,
					Function: api.ToolCallFunction{
						Index:     tcIndex,
						Name:      tc.Name,
						Arguments: args,
					},
				})
				tcIndex++
			}
		}
		text := adapter.TextFromParts(msg.Content)
		return []api.Message{{Role: "assistant", Content: text, ToolCalls: toolCalls}}, nil
	case prompty.RoleTool:
		var toolResult prompty.ToolResultPart
		var foundToolResult bool
		for _, p := range msg.Content {
			switch x := p.(type) {
			case prompty.MediaPart:
				return nil, fmt.Errorf("%w: Ollama does not support images in tool messages", adapter.ErrUnsupportedContentType)
			case prompty.ToolResultPart:
				if !foundToolResult {
					toolResult = x
					foundToolResult = true
				}
			}
		}
		if foundToolResult {
			// Fail-fast on MediaPart inside tool result content
			for _, cp := range toolResult.Content {
				if _, ok := cp.(prompty.MediaPart); ok {
					return nil, fmt.Errorf("%w: Ollama does not support images in tool result content", adapter.ErrUnsupportedContentType)
				}
			}
			text := adapter.TextFromParts(toolResult.Content)
			return []api.Message{{
				Role:       "tool",
				Content:    text,
				ToolCallID: toolResult.ToolCallID,
			}}, nil
		}
		return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	default:
		return nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
	}
}

func (a *Adapter) translateTool(t prompty.ToolDefinition) (api.Tool, error) {
	params := api.ToolFunctionParameters{
		Type:       "object",
		Properties: api.NewToolPropertiesMap(),
	}
	if t.Parameters != nil {
		b, err := json.Marshal(t.Parameters)
		if err != nil {
			return api.Tool{}, fmt.Errorf("%w: failed to marshal tool parameters: %w", adapter.ErrMalformedArgs, err)
		}
		if err = json.Unmarshal(b, &params); err != nil {
			return api.Tool{}, fmt.Errorf("%w: failed to unmarshal tool parameters: %w", adapter.ErrMalformedArgs, err)
		}
		if params.Properties == nil {
			params.Properties = api.NewToolPropertiesMap()
		}
	}
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		},
	}, nil
}

// ParseResponse converts *api.ChatResponse into []prompty.ContentPart.
func (a *Adapter) ParseResponse(ctx context.Context, raw any) ([]prompty.ContentPart, error) {
	_ = ctx
	resp, ok := raw.(*api.ChatResponse)
	if !ok {
		return nil, adapter.ErrInvalidResponse
	}
	msg := &resp.Message
	var out []prompty.ContentPart
	if msg.Content != "" {
		out = append(out, prompty.TextPart{Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		argsMap := tc.Function.Arguments.ToMap()
		var args string
		if len(argsMap) > 0 {
			b, err := json.Marshal(argsMap)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal tool call args: %w", adapter.ErrMalformedArgs, err)
			}
			args = string(b)
		} else {
			args = "{}"
		}
		out = append(out, prompty.ToolCallPart{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	return out, nil
}

// ParseStreamChunk parses a single Ollama stream chunk (*api.ChatResponse, Done: false).
// Emits one ContentPart per chunk; client glues ArgsChunk for tool calls.
func (a *Adapter) ParseStreamChunk(ctx context.Context, rawChunk any) ([]prompty.ContentPart, error) {
	_ = ctx
	chunk, ok := rawChunk.(*api.ChatResponse)
	if !ok {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	if chunk.Message.Content != "" {
		out = append(out, prompty.TextPart{Text: chunk.Message.Content})
	}
	for _, tc := range chunk.Message.ToolCalls {
		argsMap := tc.Function.Arguments.ToMap()
		var argsChunk string
		if len(argsMap) > 0 {
			b, err := json.Marshal(argsMap)
			if err != nil {
				continue
			}
			argsChunk = string(b)
		}
		out = append(out, prompty.ToolCallPart{ID: tc.ID, Name: tc.Function.Name, ArgsChunk: argsChunk})
	}
	return out, nil
}

var _ adapter.ProviderAdapter = (*Adapter)(nil)
