package ollama

import (
	"context"
	"encoding/json"
	"errors"
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
	client       ChatClient
	endpoint     string
	profiles     []prompty.CapabilityProfile
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelOptions does not contain Model.
func WithModel(m string) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the Ollama SDK client for Execute. Required for Execute/Invoker flow.
func WithClient(c ChatClient) Option {
	if client, ok := c.(*api.Client); ok && client == nil {
		c = nil
	}
	return func(a *Adapter) { a.client = c }
}

// New returns an Adapter with default model set to "llama3.2". Options can override the default model.
func New(opts ...Option) *Adapter {
	a := &Adapter{client: nil, endpoint: "", profiles: nil, defaultModel: "llama3.2"}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// RuntimeDescriptor exposes adapter runtime metadata.
func (a *Adapter) RuntimeDescriptor() adapter.RuntimeDescriptor {
	endpoint := a.endpoint
	if endpoint == "" && a.client == nil {
		endpoint = "http://localhost:11434"
	}
	return adapter.CloneRuntimeDescriptor(
		adapter.RuntimeDescriptor{HostedToolCapabilities: nil, MessageAnnotationCapabilities: nil,
			Name: wireOllama, Endpoint: endpoint, DefaultModel: a.defaultModel,
			Profiles: a.profiles, ReasoningSettings: []string{"think"},
		},
	)
}

// WithProfiles declares caller-owned knowledge for the injected client's actual
// endpoint. This option does not configure transport; the caller must bind the
// SDK client to the same endpoint. Unknown models retain unknown capabilities.
func WithProfiles(endpoint string, profiles ...prompty.CapabilityProfile) Option {
	snapshot := adapter.CloneRuntimeDescriptor(
		adapter.RuntimeDescriptor{
			Name:                          "",
			Endpoint:                      "",
			DefaultModel:                  "",
			ReasoningSettings:             nil,
			HostedToolCapabilities:        nil,
			MessageAnnotationCapabilities: nil,
			Profiles:                      profiles,
		},
	)
	return func(a *Adapter) {
		a.endpoint = endpoint
		a.profiles = adapter.CloneRuntimeDescriptor(snapshot).Profiles
	}
}

// Translate converts PromptExecution into *api.ChatRequest.
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*api.ChatRequest, error) {
	if err := a.validateTranslation(exec); err != nil {
		return nil, err
	}
	working := adapter.PrepareTranslateExecution(exec)
	if exec.ForcedTool != "" {
		return nil, fmt.Errorf(
			"%w: native named-tool selection mapping is not implemented",
			adapter.ErrUnsupportedCapability,
		)
	}
	settings, err := prompty.ProviderSettingsMap(exec.ModelOptions)
	if err != nil {
		return nil, err
	}
	if _, thinking := settings["think"]; thinking || exec.ResponseFormat != nil {
		checked := exec.Clone()
		if thinking {
			checked.Requirements.Capabilities = append(checked.Requirements.Capabilities, prompty.CapabilityReasoning)
		}
		if _, err := adapter.Preflight(a.RuntimeDescriptor(), checked, "", true); err != nil {
			return nil, err
		}
	}
	model := a.defaultModel
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		model = exec.ModelOptions.Model
	}
	req := &api.ChatRequest{
		Model:    model,
		Messages: make([]api.Message, 0, len(working.Messages)),
	}
	if err := a.applyResponseFormat(req, exec); err != nil {
		return nil, err
	}
	if err := applyModelOptions(req, exec); err != nil {
		return nil, err
	}
	for _, msg := range working.Messages {
		m, err := a.translateMessage(msg)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, m...)
	}
	if err := a.applyExecutionTools(req, exec); err != nil {
		return nil, err
	}
	return req, nil
}

// Execute performs the API call. Requires WithClient. Uses Stream: false for a single response.
func (a *Adapter) Execute(ctx context.Context, req *api.ChatRequest) (*api.ChatResponse, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	if req == nil {
		return nil, adapter.ErrNilExecution
	}
	request := *req
	streamOff := false
	request.Stream = &streamOff
	var lastResp api.ChatResponse
	callbacks := 0
	err := a.client.Chat(ctx, &request, func(r api.ChatResponse) error {
		callbacks++
		if callbacks != 1 {
			return &chatCallbackError{cause: adapter.ErrInvalidResponse}
		}
		lastResp = r
		return nil
	})
	if err != nil {
		return nil, safeChatError(ctx, err)
	}
	if callbacks != 1 {
		return nil, adapter.ErrInvalidResponse
	}
	return &lastResp, nil
}

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
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
		thinking, thinkingErr := ollamaThinking(msg.Content)
		if thinkingErr != nil {
			return nil, thinkingErr
		}
		var toolCalls []api.ToolCall
		var tcIndex int
		for _, p := range msg.Content {
			if tc, ok := p.(prompty.ToolCallPart); ok {
				argsJSON, argsErr := prompty.ToolCallArgsForTranslate(tc)
				if argsErr != nil {
					return nil, argsErr
				}
				args, err := decodeExactToolArguments([]byte(argsJSON))
				if err != nil {
					return nil, err
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
		return []api.Message{{Role: "assistant", Content: text, Thinking: thinking, ToolCalls: toolCalls}}, nil
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
				if x.IsError {
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
					ToolName:   x.Name,
				})
			case *prompty.ToolResultPart:
				if x == nil || x.IsError {
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
					ToolName:   x.Name,
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
	params, err := decodeToolSchema(t.Parameters)
	if err != nil {
		return api.Tool{}, err
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
	if err := validateOllamaMessage(resp.Message); err != nil {
		return nil, err
	}
	msg := &resp.Message
	var out []prompty.ContentPart
	if msg.Thinking != "" {
		out = append(out, prompty.ReasoningPart{CachePolicy: nil, Text: msg.Thinking})
	}
	if msg.Content != "" {
		out = append(out, prompty.TextPart{CachePolicy: nil, Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		argsMap := tc.Function.Arguments.ToMap()
		if argsMap == nil || tc.Function.Name == "" {
			return partialOllamaFailure(resp, out, adapter.ErrMalformedArgs)
		}
		b, err := json.Marshal(argsMap)
		if err != nil {
			return partialOllamaFailure(resp, out, adapter.ErrMalformedArgs)
		}
		out = append(
			out,
			prompty.ToolCallPart{ArgsChunk: "", CachePolicy: nil, ID: tc.ID, Name: tc.Function.Name, Args: string(b)},
		)
	}
	if len(out) == 0 && resp.DoneReason == "" {
		return nil, adapter.ErrEmptyResponse
	}
	result := prompty.NewResponse(out)
	result.Model = resp.Model
	result.FinishReason = resp.DoneReason
	result.Outcome = prompty.OutcomeIncomplete
	if resp.Done && resp.DoneReason == "stop" {
		result.Outcome = prompty.OutcomeCompleted
		if len(msg.ToolCalls) != 0 {
			result.Outcome = prompty.OutcomeToolCalls
		}
	}
	usage, err := usageFromOllama(resp)
	result.Usage = usage
	return result, err
}

func usageFromOllama(resp *api.ChatResponse) (prompty.Usage, error) {
	var usage prompty.Usage
	usage.Mode = prompty.UsageCumulative
	if resp.PromptEvalCount != 0 {
		usage.PromptTokens = resp.PromptEvalCount
		usage.Known = append(usage.Known, prompty.UsagePrompt)
	}
	if resp.EvalCount != 0 {
		usage.CompletionTokens = resp.EvalCount
		usage.Known = append(usage.Known, prompty.UsageCompletion)
	}
	if resp.PromptEvalCount != 0 && resp.EvalCount != 0 {
		total, err := adapter.SumUsageCounters(int64(resp.PromptEvalCount), int64(resp.EvalCount))
		if err != nil {
			return usage, err
		}
		usage.TotalTokens = total
		usage.Known = append(usage.Known, prompty.UsageTotal)
	}
	return usage, nil
}

// ParseStreamChunk maps a single SDK callback. Tool arguments are complete
// snapshots in Args, never concatenated JSON in ArgsChunk.
// This is a parts-only decoder, without finish, correlation or lifecycle events.
// Use StreamEvents/checked ExecuteStream for authoritative completed outcomes.
func (a *Adapter) ParseStreamChunk(rawChunk any) ([]prompty.ContentPart, error) {
	chunk, ok := rawChunk.(*api.ChatResponse)
	if !ok || chunk == nil {
		return nil, adapter.ErrInvalidResponse
	}
	if err := validateOllamaMessage(chunk.Message); err != nil {
		return nil, err
	}
	var out []prompty.ContentPart
	if chunk.Message.Thinking != "" {
		out = append(out, prompty.ReasoningPart{CachePolicy: nil, Text: chunk.Message.Thinking})
	}
	if chunk.Message.Content != "" {
		out = append(out, prompty.TextPart{CachePolicy: nil, Text: chunk.Message.Content})
	}
	for _, tc := range chunk.Message.ToolCalls {
		argsMap := tc.Function.Arguments.ToMap()
		if argsMap == nil || tc.Function.Name == "" {
			return out, adapter.ErrMalformedArgs
		}
		b, err := json.Marshal(argsMap)
		if err != nil {
			return out, adapter.ErrMalformedArgs
		}
		out = append(
			out,
			prompty.ToolCallPart{ArgsChunk: "", CachePolicy: nil, ID: tc.ID, Name: tc.Function.Name, Args: string(b)},
		)
	}
	return out, nil
}

var _ adapter.ProviderAdapter[*api.ChatRequest, *api.ChatResponse] = (*Adapter)(nil)

func ollamaProviderSettingKeys() []string {
	return []string{"top_k", "seed", "num_ctx", "repeat_penalty", "think"}
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func applyOllamaProviderSettings(request *api.ChatRequest, settings map[string]any) error {
	if len(settings) == 0 {
		return nil
	}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, ollamaProviderSettingKeys()); err != nil {
		return err
	}
	options := request.Options
	if raw, ok := settings["think"]; ok {
		think := &api.ThinkValue{Value: raw}
		if !think.IsValid() {
			return adapter.ProviderSettingError("think", errors.New("expected boolean, null, low, medium or high"))
		}
		request.Think = think
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

// Ollama has one Thinking field; accept a contiguous reasoning prefix only.
func ollamaThinking(parts []prompty.ContentPart) (string, error) {
	var result strings.Builder
	ordinary := false
	for _, part := range parts {
		switch value := part.(type) {
		case prompty.ReasoningPart:
			if ordinary {
				return "", adapter.ErrUnsupportedContentType
			}
			result.WriteString(value.Text)
		case *prompty.ReasoningPart:
			if value == nil || ordinary {
				return "", adapter.ErrUnsupportedContentType
			}
			result.WriteString(value.Text)
		default:
			ordinary = true
		}
	}
	return result.String(), nil
}

func partialOllamaFailure(raw *api.ChatResponse, parts []prompty.ContentPart, cause error) (*prompty.Response, error) {
	usage, err := usageFromOllama(raw)
	if len(parts) == 0 && len(usage.Known) == 0 {
		return nil, errors.Join(cause, err)
	}
	response := prompty.NewResponse(parts)
	response.Model = raw.Model
	response.Outcome = "" // Failed argument decoding cannot confirm completion.
	response.ContinuationUnavailable = true
	response.Usage = usage
	return response, errors.Join(cause, err)
}

func validateOllamaMessage(message api.Message) error {
	if message.Role != "" && message.Role != "assistant" {
		return adapter.ErrInvalidResponse
	}
	if len(message.Images) != 0 {
		return adapter.ErrUnsupportedContentType
	}
	return nil
}

func applyModelOptions(req *api.ChatRequest, exec *prompty.PromptExecution) error {
	if exec.ModelOptions == nil {
		return nil
	}
	providerSettings, settingsErr := prompty.ProviderSettingsMap(exec.ModelOptions)
	if settingsErr != nil {
		return settingsErr
	}
	if exec.ModelOptions.Temperature == nil && exec.ModelOptions.MaxTokens == nil && exec.ModelOptions.TopP == nil &&
		len(exec.ModelOptions.Stop) == 0 &&
		len(providerSettings) == 0 {
		return nil
	}
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
	if err := applyOllamaProviderSettings(req, providerSettings); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) applyResponseFormat(req *api.ChatRequest, exec *prompty.PromptExecution) error {
	if exec.ResponseFormat != nil {
		mapping, err := a.MapSchema(
			prompty.SchemaMappingRequest{
				Dialect: "",
				Policy:  prompty.SchemaPolicy{Mode: "", AllowedTransforms: nil},
				Profile: prompty.ProfileIdentity{
					Endpoint:    "",
					Model:       "",
					ModelFamily: "",
					Provider:    wireOllama,
				},
				Role:   prompty.SchemaOutput,
				Source: exec.ResponseFormat.Schema,
			},
		)
		if err != nil {
			return err
		}
		req.Format = mapping.Schema
	}

	return nil
}

func (a *Adapter) validateTranslation(exec *prompty.PromptExecution) error {
	if err := adapter.RejectUnsupportedHostedTools(exec); err != nil {
		return err
	}

	if err := adapter.ValidateExecution(exec); err != nil {
		return err
	}
	if err := adapter.ValidateMessageAnnotationMappings(exec, a.RuntimeDescriptor()); err != nil {
		return err
	}
	if err := adapter.RejectRequestCache(exec); err != nil {
		return err
	}
	if err := adapter.RejectSyntheticOutput(exec); err != nil {
		return err
	}
	if err := adapter.RejectCacheResource(exec); err != nil {
		return err
	}
	if cacheErr := adapter.RejectUnmappedCache(exec); cacheErr != nil {
		return cacheErr
	}
	if stateErr := adapter.RejectProviderState(exec); stateErr != nil {
		return stateErr
	}

	return nil
}

func (a *Adapter) applyExecutionTools(req *api.ChatRequest, exec *prompty.PromptExecution) error {
	if len(exec.Tools) > 0 {
		req.Tools = make(api.Tools, 0, len(exec.Tools))
		for _, t := range exec.Tools {
			tool, err := a.translateTool(t)
			if err != nil {
				return err
			}
			req.Tools = append(req.Tools, tool)
		}
	}

	return nil
}
