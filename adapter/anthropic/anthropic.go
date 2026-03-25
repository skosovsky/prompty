package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const defaultMaxTokens int64 = 1024

// Adapter implements adapter.ProviderAdapter for the Anthropic Messages API.
// Req = *anthropic.MessageNewParams, Resp = *anthropic.Message.
type Adapter struct {
	defaultModel anthropic.Model
	client       *anthropic.Client
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelOptions does not contain Model.
func WithModel(m anthropic.Model) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the Anthropic SDK client for Execute. Required for Execute/Invoker flow.
func WithClient(c *anthropic.Client) Option {
	return func(a *Adapter) { a.client = c }
}

// New returns an Adapter with a default model. Options can override the default model.
func New(opts ...Option) *Adapter {
	a := &Adapter{defaultModel: anthropic.ModelClaudeSonnet4_5_20250929}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

const outputFormatToolName = "output_format"

// Translate converts PromptExecution into *anthropic.MessageNewParams.
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*anthropic.MessageNewParams, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	if exec.ResponseFormat != nil && len(exec.Tools) > 0 {
		return nil, fmt.Errorf("anthropic adapter: cannot use both Tools and ResponseFormat simultaneously: %w", prompty.ErrConflictingDirectives)
	}
	params := &anthropic.MessageNewParams{
		MaxTokens: defaultMaxTokens,
		Model:     a.defaultModel,
	}
	if exec.ModelOptions != nil {
		if exec.ModelOptions.Model != "" {
			params.Model = anthropic.Model(exec.ModelOptions.Model)
		}
		if exec.ModelOptions.MaxTokens != nil {
			params.MaxTokens = *exec.ModelOptions.MaxTokens
		}
		if exec.ModelOptions.Temperature != nil {
			params.Temperature = anthropic.Float(*exec.ModelOptions.Temperature)
		}
		if exec.ModelOptions.TopP != nil {
			params.TopP = anthropic.Float(*exec.ModelOptions.TopP)
		}
		if len(exec.ModelOptions.Stop) > 0 {
			params.StopSequences = exec.ModelOptions.Stop
		}
	}
	var systemBlocks []anthropic.TextBlockParam
	var messages []anthropic.MessageParam
	for _, msg := range exec.Messages {
		switch msg.Role {
		case prompty.RoleSystem, prompty.RoleDeveloper:
			text := prompty.TextFromParts(msg.Content)
			systemBlocks = append(systemBlocks, a.systemTextBlock(text, msg.CachePoint))
		case prompty.RoleUser:
			m, err := a.userMessage(msg.Content, msg.CachePoint)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		case prompty.RoleAssistant:
			m, err := a.assistantMessage(msg.Content, msg.CachePoint)
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
			return nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
		}
	}
	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}
	params.Messages = messages
	// ResponseFormat: add mandatory output_format tool with schema; force tool_choice.
	if exec.ResponseFormat != nil && len(exec.ResponseFormat.Schema) > 0 {
		schema := schemaToToolInput(exec.ResponseFormat.Schema)
		desc := exec.ResponseFormat.Description
		if desc == "" {
			desc = "Output must strictly follow this JSON schema"
		}
		outputTool := anthropic.ToolUnionParamOfTool(schema, outputFormatToolName)
		outputTool.OfTool.Description = anthropic.String(desc)
		params.Tools = append(params.Tools, outputTool)
		params.ToolChoice = anthropic.ToolChoiceParamOfTool(outputFormatToolName)
	}
	if len(exec.Tools) > 0 {
		if params.Tools == nil {
			params.Tools = make([]anthropic.ToolUnionParam, 0, len(exec.Tools))
		}
		for _, t := range exec.Tools {
			schema := toolSchemaFromParameters(t.Parameters)
			tool := anthropic.ToolUnionParamOfTool(schema, t.Name)
			if t.Description != "" {
				tool.OfTool.Description = anthropic.String(t.Description)
			}
			params.Tools = append(params.Tools, tool)
		}
	}
	return params, nil
}

// schemaToToolInput converts ResponseFormat.Schema (full JSON Schema) to ToolInputSchemaParam.
// Passes through type, properties, required; other top-level keys (additionalProperties, description, etc.) go to ExtraFields for strict output.
func schemaToToolInput(schema map[string]any) anthropic.ToolInputSchemaParam {
	s := anthropic.ToolInputSchemaParam{
		Type: constant.Object("object"),
	}
	if schema == nil {
		return s
	}
	if t, ok := schema["type"].(string); ok && t != "" {
		s.Type = constant.Object(t)
	}
	if p, ok := schema["properties"].(map[string]any); ok {
		s.Properties = p
	}
	if r, ok := schema["required"].([]any); ok {
		required := make([]string, 0, len(r))
		for _, x := range r {
			if str, ok := x.(string); ok {
				required = append(required, str)
			}
		}
		s.Required = required
	} else if r, ok := schema["required"].([]string); ok {
		s.Required = r
	}
	// Pass through remaining top-level schema keys for strict constrained decoding
	known := map[string]bool{"type": true, "properties": true, "required": true}
	extras := make(map[string]any)
	for k, v := range schema {
		if !known[k] && v != nil {
			extras[k] = v
		}
	}
	if len(extras) > 0 {
		s.ExtraFields = extras
	}
	return s
}

// toolSchemaFromParameters builds ToolInputSchemaParam from a JSON Schema map.
// Passes through type, properties, required; other top-level keys go to ExtraFields.
func toolSchemaFromParameters(params map[string]any) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{
		Type: constant.Object("object"),
	}
	if params == nil {
		return schema
	}
	if t, ok := params["type"].(string); ok && t != "" {
		schema.Type = constant.Object(t)
	}
	if p, ok := params["properties"].(map[string]any); ok {
		schema.Properties = p
	}
	if r, ok := params["required"].([]any); ok {
		required := make([]string, 0, len(r))
		for _, x := range r {
			if s, ok := x.(string); ok {
				required = append(required, s)
			}
		}
		schema.Required = required
	} else if r, ok := params["required"].([]string); ok {
		schema.Required = r
	}
	known := map[string]bool{"type": true, "properties": true, "required": true}
	extras := make(map[string]any)
	for k, v := range params {
		if !known[k] && v != nil {
			extras[k] = v
		}
	}
	if len(extras) > 0 {
		schema.ExtraFields = extras
	}
	return schema
}

func (a *Adapter) userMessage(parts []prompty.ContentPart, cachePoint bool) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			blocks = append(blocks, a.textBlockWithCacheControl(x.Text, cachePoint))
		case prompty.MediaPart:
			if x.MediaType != "image" {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			data := x.Data
			mime := x.MIMEType
			if len(data) == 0 && x.URL != "" {
				return anthropic.MessageParam{}, fmt.Errorf("%w", adapter.ErrMediaNotResolved)
			}
			if len(data) == 0 {
				return anthropic.MessageParam{}, fmt.Errorf("%w: MediaPart has neither Data nor URL", adapter.ErrUnsupportedContentType)
			}
			if mime == "" {
				mime = "image/png"
			}
			blocks = append(blocks, a.imageBlockWithCacheControl(mime, base64.StdEncoding.EncodeToString(data)))
		default:
			return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
		}
	}
	return anthropic.NewUserMessage(blocks...), nil
}

func (a *Adapter) assistantMessage(parts []prompty.ContentPart, cachePoint bool) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			blocks = append(blocks, a.textBlockWithCacheControl(x.Text, cachePoint))
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
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for _, p := range parts {
		if tr, ok := p.(prompty.ToolResultPart); ok {
			// SDK NewToolResultBlock(toolUseID, content string, isError). Build text from parts; fail on MediaPart.
			for _, cp := range tr.Content {
				if _, ok := cp.(prompty.MediaPart); ok {
					return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
				}
			}
			text := prompty.TextFromParts(tr.Content)
			blocks = append(blocks, anthropic.NewToolResultBlock(tr.ToolCallID, text, tr.IsError))
		}
	}
	if len(blocks) == 0 {
		return anthropic.MessageParam{}, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	}
	return anthropic.NewUserMessage(blocks...), nil
}

// systemTextBlock returns a system text block; sets CacheControl when cachePoint is true (ephemeral).
func (a *Adapter) systemTextBlock(text string, cachePoint bool) anthropic.TextBlockParam {
	block := anthropic.TextBlockParam{Text: text}
	if cachePoint {
		block.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return block
}

// textBlockWithCacheControl returns a text block; sets CacheControl when cachePoint is true (ephemeral).
func (a *Adapter) textBlockWithCacheControl(text string, cachePoint bool) anthropic.ContentBlockParamUnion {
	if cachePoint {
		return anthropic.ContentBlockParamUnion{
			OfText: &anthropic.TextBlockParam{
				Text:         text,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		}
	}
	return anthropic.NewTextBlock(text)
}

// imageBlockWithCacheControl returns an image block (no cache control on image blocks per SDK).
func (a *Adapter) imageBlockWithCacheControl(mime, base64Data string) anthropic.ContentBlockParamUnion {
	return anthropic.NewImageBlockBase64(mime, base64Data)
}

// Execute performs the API call. Requires WithClient.
func (a *Adapter) Execute(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	return a.client.Messages.New(ctx, *req)
}

// ParseResponse converts *anthropic.Message into *prompty.Response.
func (a *Adapter) ParseResponse(msg *anthropic.Message) (*prompty.Response, error) {
	if msg == nil {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			text := block.Text
			if text != "" {
				out = append(out, prompty.TextPart{Text: text})
			}
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			// Structured output: output_format tool returns JSON as text for consumer to parse
			if block.Name == outputFormatToolName {
				out = append(out, prompty.TextPart{Text: args})
			} else {
				out = append(out, prompty.ToolCallPart{ID: block.ID, Name: block.Name, Args: args})
			}
		}
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	resp := prompty.NewResponse(out)
	resp.Usage = usageFromAnthropic(msg.Usage)
	if msg.StopReason != "" {
		resp.FinishReason = string(msg.StopReason)
	}
	return resp, nil
}

// ParseStreamChunk parses a single Anthropic stream event. The SDK uses RawContentBlockDeltaUnion and
// ContentBlockStartEventContentBlockUnion; exact field names depend on anthropic-sdk-go version.
// Until the adapter is updated to match the SDK struct, callers can type-assert to *ContentBlockDeltaEvent
// or *ContentBlockStartEvent and extract delta/block manually, or use this stub.
func (a *Adapter) ParseStreamChunk(ctx context.Context, rawChunk any) ([]prompty.ContentPart, error) {
	_ = ctx
	_ = rawChunk
	return nil, adapter.ErrStreamNotImplemented
}

var _ adapter.ProviderAdapter[*anthropic.MessageNewParams, *anthropic.Message] = (*Adapter)(nil)

func usageFromAnthropic(usage anthropic.Usage) prompty.Usage {
	promptTokens := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	return prompty.Usage{
		PromptTokens:              int(promptTokens),
		CompletionTokens:          int(usage.OutputTokens),
		TotalTokens:               int(promptTokens + usage.OutputTokens),
		PromptTokensCached:        int(usage.CacheReadInputTokens),
		PromptTokensCacheCreation: int(usage.CacheCreationInputTokens),
	}
}
