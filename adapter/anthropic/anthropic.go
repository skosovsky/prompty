package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

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
			blocks, err := a.systemMessageBlocks(msg.Content, msg.CacheControl)
			if err != nil {
				return nil, err
			}
			systemBlocks = append(systemBlocks, blocks...)
		case prompty.RoleUser:
			m, err := a.userMessage(msg.Content, msg.CacheControl)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		case prompty.RoleAssistant:
			m, err := a.assistantMessage(msg.Content, msg.CacheControl)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		case prompty.RoleTool:
			m, err := a.toolResultMessage(msg.Content, msg.CacheControl)
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

func (a *Adapter) systemMessageBlocks(parts []prompty.ContentPart, messageCache *prompty.CacheControl) ([]anthropic.TextBlockParam, error) {
	blocks := make([]anthropic.TextBlockParam, 0, len(parts))
	for _, p := range parts {
		var textPart prompty.TextPart
		switch x := p.(type) {
		case prompty.TextPart:
			textPart = x
		case *prompty.TextPart:
			if x == nil {
				return nil, adapter.ErrUnsupportedContentType
			}
			textPart = *x
		default:
			return nil, adapter.ErrUnsupportedContentType
		}
		cache, err := toAnthropicCacheControl(resolveCacheControl(messageCache, textPart.CacheControl))
		if err != nil {
			return nil, err
		}
		block := anthropic.TextBlockParam{Text: textPart.Text}
		if cache != nil {
			block.CacheControl = *cache
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (a *Adapter) userMessage(parts []prompty.ContentPart, messageCache *prompty.CacheControl) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CacheControl))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.TextPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CacheControl))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case prompty.MediaPart:
			cache := resolveCacheControl(messageCache, x.CacheControl)
			block, err := a.mediaBlock(x, cache)
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.MediaPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			cache := resolveCacheControl(messageCache, x.CacheControl)
			block, err := a.mediaBlock(*x, cache)
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		default:
			return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
		}
	}
	return anthropic.NewUserMessage(blocks...), nil
}

func (a *Adapter) assistantMessage(parts []prompty.ContentPart, messageCache *prompty.CacheControl) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CacheControl))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.TextPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CacheControl))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case prompty.ToolCallPart:
			if x.Args != "" && !json.Valid([]byte(x.Args)) {
				return anthropic.MessageParam{}, fmt.Errorf("%w: invalid tool call args JSON", adapter.ErrMalformedArgs)
			}
			var input json.RawMessage
			if x.Args != "" {
				input = json.RawMessage(x.Args)
			}
			block := anthropic.NewToolUseBlock(x.ID, input, x.Name)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CacheControl))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.ToolCallPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			if x.Args != "" && !json.Valid([]byte(x.Args)) {
				return anthropic.MessageParam{}, fmt.Errorf("%w: invalid tool call args JSON", adapter.ErrMalformedArgs)
			}
			var input json.RawMessage
			if x.Args != "" {
				input = json.RawMessage(x.Args)
			}
			block := anthropic.NewToolUseBlock(x.ID, input, x.Name)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CacheControl))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		default:
			return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
		}
	}
	return anthropic.NewAssistantMessage(blocks...), nil
}

func (a *Adapter) toolResultMessage(parts []prompty.ContentPart, messageCache *prompty.CacheControl) (anthropic.MessageParam, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for _, p := range parts {
		var tr prompty.ToolResultPart
		switch x := p.(type) {
		case prompty.ToolResultPart:
			tr = x
		case *prompty.ToolResultPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			tr = *x
		default:
			continue
		}
		{
			toolResultCache := resolveCacheControl(messageCache, tr.CacheControl)
			content := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(tr.Content))
			for _, cp := range tr.Content {
				block, err := a.toolResultContentBlock(cp, toolResultCache)
				if err != nil {
					return anthropic.MessageParam{}, err
				}
				content = append(content, block)
			}
			block := anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: tr.ToolCallID,
					IsError:   anthropic.Bool(tr.IsError),
					Content:   content,
				},
			}
			block, err := a.applyCacheControl(block, toolResultCache)
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return anthropic.MessageParam{}, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	}
	return anthropic.NewUserMessage(blocks...), nil
}

func (a *Adapter) toolResultContentBlock(part prompty.ContentPart, inheritedCache *prompty.CacheControl) (anthropic.ToolResultBlockParamContentUnion, error) {
	cache := resolveCacheControl(inheritedCache, contentPartCacheControl(part))
	switch x := part.(type) {
	case prompty.TextPart:
		c, err := toAnthropicCacheControl(cache)
		if err != nil {
			return anthropic.ToolResultBlockParamContentUnion{}, err
		}
		block := anthropic.ToolResultBlockParamContentUnion{OfText: &anthropic.TextBlockParam{Text: x.Text}}
		if c != nil && block.OfText != nil {
			block.OfText.CacheControl = *c
		}
		return block, nil
	case prompty.MediaPart:
		b, err := a.mediaBlock(x, cache)
		if err != nil {
			return anthropic.ToolResultBlockParamContentUnion{}, err
		}
		return anthropic.ToolResultBlockParamContentUnion{
			OfImage:    b.OfImage,
			OfDocument: b.OfDocument,
		}, nil
	case *prompty.TextPart:
		if x == nil {
			return anthropic.ToolResultBlockParamContentUnion{}, adapter.ErrUnsupportedContentType
		}
		return a.toolResultContentBlock(*x, inheritedCache)
	case *prompty.MediaPart:
		if x == nil {
			return anthropic.ToolResultBlockParamContentUnion{}, adapter.ErrUnsupportedContentType
		}
		return a.toolResultContentBlock(*x, inheritedCache)
	default:
		return anthropic.ToolResultBlockParamContentUnion{}, adapter.ErrUnsupportedContentType
	}
}

func (a *Adapter) mediaBlock(part prompty.MediaPart, cache *prompty.CacheControl) (anthropic.ContentBlockParamUnion, error) {
	mime := strings.ToLower(strings.TrimSpace(part.MIMEType))
	if mime == "" {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf(
			"%w: MediaPart.MIMEType is required for Anthropic media translation",
			adapter.ErrUnsupportedContentType,
		)
	}
	var (
		block anthropic.ContentBlockParamUnion
		err   error
	)
	switch {
	case len(part.Data) > 0:
		block, err = mediaBlockFromData(mime, part.Data)
	case part.URL != "":
		block, err = mediaBlockFromURL(mime, part.URL)
	default:
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("%w: MediaPart has neither Data nor URL", adapter.ErrUnsupportedContentType)
	}
	if err != nil {
		return anthropic.ContentBlockParamUnion{}, err
	}
	return a.applyCacheControl(block, cache)
}

func mediaBlockFromData(mime string, data []byte) (anthropic.ContentBlockParamUnion, error) {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return anthropic.NewImageBlockBase64(mime, base64.StdEncoding.EncodeToString(data)), nil
	case mime == "application/pdf":
		return anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
			Data:      base64.StdEncoding.EncodeToString(data),
			MediaType: constant.ValueOf[constant.ApplicationPDF](),
			Type:      constant.ValueOf[constant.Base64](),
		}), nil
	case mime == "text/plain":
		return anthropic.NewDocumentBlock(anthropic.PlainTextSourceParam{
			Data:      string(data),
			MediaType: constant.ValueOf[constant.TextPlain](),
			Type:      constant.ValueOf[constant.Text](),
		}), nil
	default:
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("%w: unsupported media MIME %q", adapter.ErrUnsupportedContentType, mime)
	}
}

func mediaBlockFromURL(mime, url string) (anthropic.ContentBlockParamUnion, error) {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return anthropic.NewImageBlock(anthropic.URLImageSourceParam{
			URL:  url,
			Type: constant.ValueOf[constant.URL](),
		}), nil
	case mime == "application/pdf":
		return anthropic.NewDocumentBlock(anthropic.URLPDFSourceParam{
			URL:  url,
			Type: constant.ValueOf[constant.URL](),
		}), nil
	default:
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("%w: unsupported URL media MIME %q", adapter.ErrUnsupportedContentType, mime)
	}
}

func (a *Adapter) applyCacheControl(block anthropic.ContentBlockParamUnion, cache *prompty.CacheControl) (anthropic.ContentBlockParamUnion, error) {
	c, err := toAnthropicCacheControl(cache)
	if err != nil {
		return anthropic.ContentBlockParamUnion{}, err
	}
	if c == nil {
		return block, nil
	}
	switch {
	case block.OfText != nil:
		block.OfText.CacheControl = *c
	case block.OfImage != nil:
		block.OfImage.CacheControl = *c
	case block.OfDocument != nil:
		block.OfDocument.CacheControl = *c
	case block.OfToolUse != nil:
		block.OfToolUse.CacheControl = *c
	case block.OfToolResult != nil:
		block.OfToolResult.CacheControl = *c
	}
	return block, nil
}

func toAnthropicCacheControl(cache *prompty.CacheControl) (*anthropic.CacheControlEphemeralParam, error) {
	if cache == nil {
		return nil, nil
	}
	cacheType := strings.ToLower(strings.TrimSpace(cache.Type))
	if cacheType == "" {
		return nil, nil
	}
	if cacheType != "ephemeral" {
		return nil, fmt.Errorf("anthropic adapter: unsupported cache_control.type %q", cache.Type)
	}
	c := anthropic.NewCacheControlEphemeralParam()
	return &c, nil
}

func resolveCacheControl(messageCache, partCache *prompty.CacheControl) *prompty.CacheControl {
	if partCache != nil {
		return partCache
	}
	return messageCache
}

func contentPartCacheControl(part prompty.ContentPart) *prompty.CacheControl {
	switch x := part.(type) {
	case prompty.TextPart:
		return x.CacheControl
	case *prompty.TextPart:
		if x == nil {
			return nil
		}
		return x.CacheControl
	case prompty.MediaPart:
		return x.CacheControl
	case *prompty.MediaPart:
		if x == nil {
			return nil
		}
		return x.CacheControl
	case prompty.ReasoningPart:
		return x.CacheControl
	case *prompty.ReasoningPart:
		if x == nil {
			return nil
		}
		return x.CacheControl
	case prompty.ToolCallPart:
		return x.CacheControl
	case *prompty.ToolCallPart:
		if x == nil {
			return nil
		}
		return x.CacheControl
	case prompty.ToolResultPart:
		return x.CacheControl
	case *prompty.ToolResultPart:
		if x == nil {
			return nil
		}
		return x.CacheControl
	default:
		return nil
	}
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
func (a *Adapter) ParseStreamChunk(rawChunk any) ([]prompty.ContentPart, error) {
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
