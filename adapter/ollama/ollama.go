package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/cast"
)

// Adapter implements adapter.ProviderAdapter for the Ollama Chat API.
// Req = *api.ChatRequest, Resp = *api.ChatResponse.
type Adapter struct {
	defaultModel string
	client       *api.Client
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelOptions does not contain Model.
func WithModel(m string) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the Ollama SDK client for Execute. Required for Execute/Invoker flow.
func WithClient(c *api.Client) Option {
	return func(a *Adapter) { a.client = c }
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
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*api.ChatRequest, error) {
	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, err
	}
	working := adapter.PrepareTranslateExecution(exec)
	if exec.ResponseFormat != nil {
		return nil, adapter.ErrStructuredOutputNotSupported
	}
	model := a.defaultModel
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		model = exec.ModelOptions.Model
	}
	req := &api.ChatRequest{
		Model:    model,
		Messages: make([]api.Message, 0, len(working.Messages)),
	}
	if exec.ModelOptions != nil {
		providerSettings, settingsErr := prompty.ProviderSettingsMap(exec.ModelOptions)
		if settingsErr != nil {
			return nil, settingsErr
		}
		if exec.ModelOptions.Temperature != nil || exec.ModelOptions.MaxTokens != nil ||
			exec.ModelOptions.TopP != nil ||
			len(exec.ModelOptions.Stop) > 0 ||
			len(providerSettings) > 0 {
			req.Options = make(map[string]any)
			if exec.ModelOptions.Temperature != nil {
				req.Options["temperature"] = *exec.ModelOptions.Temperature
			}
			if exec.ModelOptions.MaxTokens != nil {
				req.Options["num_predict"] = *exec.ModelOptions.MaxTokens
			}
			if exec.ModelOptions.TopP != nil {
				req.Options["top_p"] = *exec.ModelOptions.TopP
			}
			if len(exec.ModelOptions.Stop) > 0 {
				req.Options["stop"] = exec.ModelOptions.Stop
			}
			if err := applyOllamaProviderSettings(req.Options, providerSettings); err != nil {
				return nil, err
			}
		}
	}
	for _, msg := range working.Messages {
		m, err := a.translateMessage(msg)
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
	if exec.ForcedTool != "" && len(req.Tools) > 0 {
		req.Messages = append([]api.Message{{
			Role:    "system",
			Content: fmt.Sprintf("You must call tool %q for the next tool-use step.", exec.ForcedTool),
		}}, req.Messages...)
	}
	return req, nil
}

// Execute performs the API call. Requires WithClient. Uses Stream: false for a single response.
func (a *Adapter) Execute(ctx context.Context, req *api.ChatRequest) (*api.ChatResponse, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	reqStream := req.Stream
	streamOff := false
	req.Stream = &streamOff
	var lastResp api.ChatResponse
	err := a.client.Chat(ctx, req, func(r api.ChatResponse) error {
		lastResp = r
		return nil
	})
	req.Stream = reqStream
	if err != nil {
		return nil, err
	}
	return &lastResp, nil
}

func (a *Adapter) translateMessage(msg prompty.ChatMessage) ([]api.Message, error) {
	switch msg.Role {
	case prompty.RoleSystem, prompty.RoleDeveloper:
		for _, p := range msg.Content {
			if _, ok := p.(prompty.MediaPart); ok {
				return nil, fmt.Errorf(
					"%w: Ollama does not support images in system messages",
					adapter.ErrUnsupportedContentType,
				)
			}
		}
		text, err := prompty.StrictTextFromParts(msg.Content)
		if err != nil {
			return nil, err
		}
		return []api.Message{{Role: "system", Content: text}}, nil
	case prompty.RoleUser:
		var images []api.ImageData
		for _, p := range msg.Content {
			img, ok := p.(prompty.MediaPart)
			if !ok {
				continue
			}
			if !isOllamaImageMediaPart(img) {
				return nil, fmt.Errorf(
					"%w: Ollama only supports image media in user messages (media_type=%q mime_type=%q)",
					adapter.ErrUnsupportedContentType,
					img.MediaType,
					img.MIMEType,
				)
			}
			data := img.Data
			if len(data) == 0 && img.URL != "" {
				return nil, fmt.Errorf("%w", adapter.ErrMediaNotResolved)
			}
			if len(data) == 0 {
				return nil, fmt.Errorf("%w: MediaPart has neither Data nor URL", adapter.ErrUnsupportedContentType)
			}
			images = append(images, api.ImageData(data))
		}
		text, err := prompty.JoinAdapterTextParts(msg.Content)
		if err != nil {
			return nil, err
		}
		return []api.Message{{Role: "user", Content: text, Images: images}}, nil
	case prompty.RoleAssistant:
		for _, p := range msg.Content {
			if _, ok := p.(prompty.MediaPart); ok {
				return nil, fmt.Errorf(
					"%w: Ollama does not support images in assistant messages",
					adapter.ErrUnsupportedContentType,
				)
			}
		}
		var toolCalls []api.ToolCall
		var tcIndex int
		for _, p := range msg.Content {
			if tc, ok := p.(prompty.ToolCallPart); ok {
				argsJSON, argsErr := prompty.ToolCallArgsForTranslate(tc)
				if argsErr != nil {
					return nil, argsErr
				}
				var args api.ToolCallFunctionArguments
				if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
					return nil, fmt.Errorf("%w: invalid tool call args JSON: %w", adapter.ErrMalformedArgs, err)
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
		text, err := prompty.JoinAdapterTextParts(msg.Content)
		if err != nil {
			return nil, err
		}
		return []api.Message{{Role: "assistant", Content: text, ToolCalls: toolCalls}}, nil
	case prompty.RoleTool:
		messages := make([]api.Message, 0, len(msg.Content))
		for _, p := range msg.Content {
			switch x := p.(type) {
			case prompty.MediaPart, *prompty.MediaPart:
				return nil, fmt.Errorf(
					"%w: Ollama does not support images in tool messages",
					adapter.ErrUnsupportedContentType,
				)
			case prompty.ToolResultPart:
				for _, cp := range x.Content {
					if _, ok := cp.(prompty.MediaPart); ok {
						return nil, fmt.Errorf(
							"%w: Ollama does not support images in tool result content",
							adapter.ErrUnsupportedContentType,
						)
					}
				}
				text, err := prompty.StrictTextFromParts(x.Content)
				if err != nil {
					return nil, err
				}
				messages = append(messages, api.Message{
					Role:       "tool",
					Content:    text,
					ToolCallID: x.ToolCallID,
				})
			case *prompty.ToolResultPart:
				if x == nil {
					return nil, adapter.ErrUnsupportedContentType
				}
				for _, cp := range x.Content {
					if _, ok := cp.(prompty.MediaPart); ok {
						return nil, fmt.Errorf(
							"%w: Ollama does not support images in tool result content",
							adapter.ErrUnsupportedContentType,
						)
					}
				}
				text, err := prompty.StrictTextFromParts(x.Content)
				if err != nil {
					return nil, err
				}
				messages = append(messages, api.Message{
					Role:       "tool",
					Content:    text,
					ToolCallID: x.ToolCallID,
				})
			default:
				return nil, fmt.Errorf("%w: unexpected %T in tool message", adapter.ErrUnsupportedContentType, p)
			}
		}
		if len(messages) > 0 {
			return messages, nil
		}
		return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	default:
		return nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
	}
}

func isOllamaImageMediaPart(mp prompty.MediaPart) bool {
	mime := strings.ToLower(strings.TrimSpace(mp.MIMEType))
	if mime != "" {
		return strings.HasPrefix(mime, "image/")
	}
	return strings.EqualFold(strings.TrimSpace(mp.MediaType), "image")
}

func (a *Adapter) translateTool(t prompty.ToolDefinition) (api.Tool, error) {
	params := api.ToolFunctionParameters{
		Type:       "object",
		Properties: api.NewToolPropertiesMap(),
	}
	if len(t.Parameters) > 0 {
		if err := json.Unmarshal(t.Parameters, &params); err != nil {
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

// ParseResponse converts *api.ChatResponse into *prompty.Response.
func (a *Adapter) ParseResponse(resp *api.ChatResponse) (*prompty.Response, error) {
	if resp == nil {
		return nil, adapter.ErrInvalidResponse
	}
	msg := &resp.Message
	var out []prompty.ContentPart
	if msg.Content != "" {
		out = append(out, prompty.TextPart{Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		argsMap := tc.Function.Arguments.ToMap()
		if len(argsMap) == 0 {
			return nil, fmt.Errorf(
				"%w: empty tool call args for %q",
				adapter.ErrMalformedArgs,
				tc.Function.Name,
			)
		}
		b, err := json.Marshal(argsMap)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to marshal tool call args: %w", adapter.ErrMalformedArgs, err)
		}
		out = append(out, prompty.ToolCallPart{ID: tc.ID, Name: tc.Function.Name, Args: string(b)})
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	return prompty.NewResponse(out), nil
}

// ParseStreamChunk parses a single Ollama stream chunk (*api.ChatResponse, Done: false).
// Emits one ContentPart per chunk; client glues ArgsChunk for tool calls.
func (a *Adapter) ParseStreamChunk(rawChunk any) ([]prompty.ContentPart, error) {
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
				return nil, fmt.Errorf("%w: failed to marshal tool call args: %w", adapter.ErrMalformedArgs, err)
			}
			argsChunk = string(b)
		}
		out = append(out, prompty.ToolCallPart{ID: tc.ID, Name: tc.Function.Name, ArgsChunk: argsChunk})
	}
	return out, nil
}

var _ adapter.ProviderAdapter[*api.ChatRequest, *api.ChatResponse] = (*Adapter)(nil)

func ollamaProviderSettingKeys() []string {
	return []string{"top_k", "seed", "num_ctx", "repeat_penalty"}
}

func applyOllamaProviderSettings(options map[string]any, settings map[string]any) error {
	if options == nil || len(settings) == 0 {
		return nil
	}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, ollamaProviderSettingKeys()); err != nil {
		return err
	}
	if raw, ok := settings["top_k"]; ok {
		topK, err := cast.ToInt32(raw)
		if err != nil {
			return adapter.ProviderSettingError("top_k", err)
		}
		if err := adapter.ValidateInt32Min("top_k", topK, 1); err != nil {
			return err
		}
		options["top_k"] = int(topK)
	}
	if raw, ok := settings["seed"]; ok {
		seed, err := cast.ToInt32(raw)
		if err != nil {
			return adapter.ProviderSettingError("seed", err)
		}
		options["seed"] = int(seed)
	}
	if raw, ok := settings["num_ctx"]; ok {
		numCtx, err := cast.ToInt32(raw)
		if err != nil {
			return adapter.ProviderSettingError("num_ctx", err)
		}
		if err := adapter.ValidateInt32Min("num_ctx", numCtx, 1); err != nil {
			return err
		}
		options["num_ctx"] = int(numCtx)
	}
	if raw, ok := settings["repeat_penalty"]; ok {
		repeatPenalty, err := cast.ToFloat32(raw)
		if err != nil {
			return adapter.ProviderSettingError("repeat_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("repeat_penalty", repeatPenalty, 0, 2); err != nil {
			return err
		}
		options["repeat_penalty"] = float64(repeatPenalty)
	}
	return nil
}
