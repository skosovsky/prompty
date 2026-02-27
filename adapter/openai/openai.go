package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Adapter implements adapter.ProviderAdapter for the OpenAI Chat Completions API.
// Translate returns *openai.ChatCompletionNewParams; ParseResponse expects *openai.ChatCompletion.
type Adapter struct {
	defaultModel shared.ChatModel
}

// Option configures an Adapter (e.g. WithModel).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelConfig does not contain "model".
func WithModel(m shared.ChatModel) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// New returns an Adapter with default model set to gpt-4o. Options can override the default model.
func New(opts ...Option) *Adapter {
	a := &Adapter{defaultModel: openai.ChatModelGPT4o}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Translate converts PromptExecution into *openai.ChatCompletionNewParams.
func (a *Adapter) Translate(exec *prompty.PromptExecution) (any, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	return a.TranslateTyped(exec)
}

// TranslateTyped returns the concrete type so callers avoid type assertion.
func (a *Adapter) TranslateTyped(exec *prompty.PromptExecution) (*openai.ChatCompletionNewParams, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	params := &openai.ChatCompletionNewParams{
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(exec.Messages)),
		Model:    a.defaultModel,
	}
	if exec.ModelConfig != nil {
		if m, ok := exec.ModelConfig["model"].(string); ok && m != "" {
			params.Model = shared.ChatModel(m) //nolint:unconvert // ChatModel is a distinct type
		}
		mp := adapter.ExtractModelConfig(exec.ModelConfig)
		if mp.Temperature != nil {
			params.Temperature = openai.Float(*mp.Temperature)
		}
		if mp.MaxTokens != nil {
			params.MaxTokens = openai.Int(*mp.MaxTokens)
		}
		if mp.TopP != nil {
			params.TopP = openai.Float(*mp.TopP)
		}
		if len(mp.Stop) > 0 {
			params.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: mp.Stop}
		}
	}
	for _, msg := range exec.Messages {
		union, err := a.messageToUnion(msg)
		if err != nil {
			return nil, err
		}
		params.Messages = append(params.Messages, union)
	}
	if len(exec.Tools) > 0 {
		params.Tools = make([]openai.ChatCompletionToolUnionParam, 0, len(exec.Tools))
		for _, t := range exec.Tools {
			params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  shared.FunctionParameters(t.Parameters),
			}))
		}
	}
	return params, nil
}

func (a *Adapter) messageToUnion(msg prompty.ChatMessage) (openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case prompty.RoleSystem:
		text := adapter.TextFromParts(msg.Content)
		return openai.SystemMessage(text), nil
	case prompty.RoleUser:
		return a.userMessage(msg.Content)
	case prompty.RoleAssistant:
		return a.assistantMessage(msg.Content)
	case prompty.RoleTool:
		return a.toolResultMessage(msg.Content)
	default:
		return openai.ChatCompletionMessageParamUnion{}, adapter.ErrUnsupportedRole
	}
}

func (a *Adapter) userMessage(parts []prompty.ContentPart) (openai.ChatCompletionMessageParamUnion, error) {
	var contentParts []openai.ChatCompletionContentPartUnionParam
	hasImage := false
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			contentParts = append(contentParts, openai.TextContentPart(x.Text))
		case prompty.ImagePart:
			hasImage = true
			part, err := imagePartToOpenAI(x)
			if err != nil {
				return openai.ChatCompletionMessageParamUnion{}, err
			}
			contentParts = append(contentParts, part)
		default:
			return openai.ChatCompletionMessageParamUnion{}, adapter.ErrUnsupportedContentType
		}
	}
	if !hasImage {
		return openai.UserMessage(adapter.TextFromParts(parts)), nil
	}
	return openai.UserMessage(contentParts), nil
}

func imagePartToOpenAI(p prompty.ImagePart) (openai.ChatCompletionContentPartUnionParam, error) {
	url := p.URL
	if len(p.Data) > 0 {
		mime := p.MIMEType
		if mime == "" {
			mime = "image/png"
		}
		url = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
	}
	if url == "" {
		return openai.ChatCompletionContentPartUnionParam{}, adapter.ErrUnsupportedContentType
	}
	return openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
		URL:    url,
		Detail: "auto",
	}), nil
}

func (a *Adapter) assistantMessage(parts []prompty.ContentPart) (openai.ChatCompletionMessageParamUnion, error) {
	var b strings.Builder
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			b.WriteString(x.Text)
		case prompty.ToolCallPart:
			if x.Args != "" && !json.Valid([]byte(x.Args)) {
				return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("%w: invalid tool call args JSON", adapter.ErrMalformedArgs)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: x.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      x.Name,
						Arguments: x.Args,
					},
					Type: "function",
				},
			})
		default:
			return openai.ChatCompletionMessageParamUnion{}, adapter.ErrUnsupportedContentType
		}
	}
	text := b.String()
	if len(toolCalls) > 0 {
		return openai.ChatCompletionMessageParamUnion{
			OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(text)},
				ToolCalls: toolCalls,
				Role:      constant.Assistant("assistant"),
			},
		}, nil
	}
	return openai.AssistantMessage(text), nil
}

func (a *Adapter) toolResultMessage(parts []prompty.ContentPart) (openai.ChatCompletionMessageParamUnion, error) {
	for _, p := range parts {
		if tr, ok := p.(prompty.ToolResultPart); ok {
			return openai.ToolMessage(tr.Content, tr.ToolCallID), nil
		}
	}
	return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
}

// ParseResponse converts *openai.ChatCompletion into []prompty.ContentPart.
func (a *Adapter) ParseResponse(raw any) ([]prompty.ContentPart, error) {
	completion, ok := raw.(*openai.ChatCompletion)
	if !ok {
		return nil, adapter.ErrInvalidResponse
	}
	if len(completion.Choices) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	msg := completion.Choices[0].Message
	var out []prompty.ContentPart
	if msg.Content != "" {
		out = append(out, prompty.TextPart{Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		if tc.Type == "function" {
			out = append(out, prompty.ToolCallPart{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: tc.Function.Arguments,
			})
		}
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	return out, nil
}

// Compile-time check that Adapter implements ProviderAdapter.
var _ adapter.ProviderAdapter = (*Adapter)(nil)
