package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const defaultMaxTokens = 1024

// Adapter implements adapter.ProviderAdapter for the Anthropic Messages API.
// Translate returns *anthropic.MessageNewParams; ParseResponse expects *anthropic.Message.
type Adapter struct {
	defaultModel anthropic.Model
}

// Option configures an Adapter (e.g. WithModel).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelConfig does not contain "model".
func WithModel(m anthropic.Model) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// New returns an Adapter with a default model. Options can override the default model.
func New(opts ...Option) *Adapter {
	a := &Adapter{defaultModel: anthropic.ModelClaudeSonnet4_5_20250929}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Translate converts PromptExecution into *anthropic.MessageNewParams.
func (a *Adapter) Translate(exec *prompty.PromptExecution) (any, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	return a.TranslateTyped(exec)
}

// TranslateTyped returns the concrete type so callers avoid type assertion.
func (a *Adapter) TranslateTyped(exec *prompty.PromptExecution) (*anthropic.MessageNewParams, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	params := &anthropic.MessageNewParams{
		MaxTokens: defaultMaxTokens,
		Model:     a.defaultModel,
	}
	if exec.ModelConfig != nil {
		if m, ok := exec.ModelConfig["model"].(string); ok && m != "" {
			params.Model = anthropic.Model(m)
		}
		mp := adapter.ExtractModelConfig(exec.ModelConfig)
		if mp.MaxTokens != nil {
			params.MaxTokens = *mp.MaxTokens
		}
		if mp.Temperature != nil {
			params.Temperature = anthropic.Float(*mp.Temperature)
		}
		if mp.TopP != nil {
			params.TopP = anthropic.Float(*mp.TopP)
		}
		if len(mp.Stop) > 0 {
			params.StopSequences = mp.Stop
		}
	}
	var systemTexts []string
	var messages []anthropic.MessageParam
	for _, msg := range exec.Messages {
		switch msg.Role {
		case prompty.RoleSystem:
			systemTexts = append(systemTexts, adapter.TextFromParts(msg.Content))
		case prompty.RoleUser:
			m, err := a.userMessage(msg.Content)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		case prompty.RoleAssistant:
			m, err := a.assistantMessage(msg.Content)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		case prompty.RoleTool:
			m, err := a.toolResultMessage(msg.Content)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		default:
			return nil, adapter.ErrUnsupportedRole
		}
	}
	if len(systemTexts) > 0 {
		params.System = []anthropic.TextBlockParam{{Text: strings.Join(systemTexts, "\n\n")}}
	}
	params.Messages = messages
	if len(exec.Tools) > 0 {
		params.Tools = make([]anthropic.ToolUnionParam, 0, len(exec.Tools))
		for _, t := range exec.Tools {
			var props map[string]any
			var required []string
			if t.Parameters != nil {
				if p, ok := t.Parameters["properties"].(map[string]any); ok {
					props = p
				}
				if r, ok := t.Parameters["required"].([]any); ok {
					for _, x := range r {
						if s, ok := x.(string); ok {
							required = append(required, s)
						}
					}
				}
			}
			schema := anthropic.ToolInputSchemaParam{
				Type:       constant.Object("object"),
				Properties: props,
				Required:   required,
			}
			tool := anthropic.ToolUnionParamOfTool(schema, t.Name)
			if t.Description != "" {
				tool.OfTool.Description = anthropic.String(t.Description)
			}
			params.Tools = append(params.Tools, tool)
		}
	}
	return params, nil
}

func (a *Adapter) userMessage(parts []prompty.ContentPart) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			blocks = append(blocks, anthropic.NewTextBlock(x.Text))
		case prompty.ImagePart:
			if len(x.Data) > 0 {
				// Data takes precedence per TD.md
				mime := x.MIMEType
				if mime == "" {
					mime = "image/png"
				}
				blocks = append(blocks, anthropic.NewImageBlockBase64(mime, base64.StdEncoding.EncodeToString(x.Data)))
			} else if x.URL != "" {
				return anthropic.MessageParam{}, fmt.Errorf("%w: Anthropic does not support image URLs, provide base64 Data", adapter.ErrUnsupportedContentType)
			} else {
				return anthropic.MessageParam{}, fmt.Errorf("%w: ImagePart has neither Data nor URL", adapter.ErrUnsupportedContentType)
			}
		default:
			return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
		}
	}
	return anthropic.NewUserMessage(blocks...), nil
}

func (a *Adapter) assistantMessage(parts []prompty.ContentPart) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			blocks = append(blocks, anthropic.NewTextBlock(x.Text))
		case prompty.ToolCallPart:
			if x.Args != "" && !json.Valid([]byte(x.Args)) {
				return anthropic.MessageParam{}, fmt.Errorf("%w: invalid tool call args JSON", adapter.ErrMalformedArgs)
			}
			var input json.RawMessage
			if x.Args != "" {
				input = json.RawMessage(x.Args)
			}
			blocks = append(blocks, anthropic.NewToolUseBlock(x.ID, input, x.Name))
		default:
			return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
		}
	}
	return anthropic.NewAssistantMessage(blocks...), nil
}

func (a *Adapter) toolResultMessage(parts []prompty.ContentPart) (anthropic.MessageParam, error) {
	for _, p := range parts {
		if tr, ok := p.(prompty.ToolResultPart); ok {
			return anthropic.NewUserMessage(anthropic.NewToolResultBlock(tr.ToolCallID, tr.Content, tr.IsError)), nil
		}
	}
	return anthropic.MessageParam{}, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
}

// ParseResponse converts *anthropic.Message into []prompty.ContentPart.
func (a *Adapter) ParseResponse(raw any) ([]prompty.ContentPart, error) {
	msg, ok := raw.(*anthropic.Message)
	if !ok {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			// SDK may set text on block.Text or only via AsText() depending on response shape.
			text := block.Text
			if text == "" {
				text = block.AsText().Text
			}
			if text != "" {
				out = append(out, prompty.TextPart{Text: text})
			}
		case "tool_use":
			tu := block.AsToolUse()
			args := string(tu.Input)
			if args == "" {
				args = "{}"
			}
			out = append(out, prompty.ToolCallPart{ID: tu.ID, Name: tu.Name, Args: args})
		}
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	return out, nil
}

var _ adapter.ProviderAdapter = (*Adapter)(nil)
