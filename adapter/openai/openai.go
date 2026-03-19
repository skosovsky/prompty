package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"sort"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Adapter implements adapter.ProviderAdapter for the OpenAI Chat Completions API.
// Req = *openai.ChatCompletionNewParams, Resp = *openai.ChatCompletion.
type Adapter struct {
	defaultModel shared.ChatModel
	client       *openai.Client
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelConfig does not contain "model".
func WithModel(m shared.ChatModel) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the OpenAI SDK client for Execute. Required for Execute/LLMClient flow.
func WithClient(c *openai.Client) Option {
	return func(a *Adapter) { a.client = c }
}

// New returns an Adapter with default model set to gpt-4o. Options can override the default model.
func New(opts ...Option) *Adapter {
	a := &Adapter{defaultModel: openai.ChatModelGPT4o}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// normalizeSchemaForStrict returns a schema copy with additionalProperties: false for type object
// (required by OpenAI strict mode). Does not mutate the original.
func normalizeSchemaForStrict(schema any) any {
	return normalizeStrictSchemaNode(cloneStrictSchemaNode(schema))
}

func normalizeStrictSchemaNode(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok {
		return schema
	}

	if properties, ok := m["properties"].(map[string]any); ok {
		required := requiredNames(m["required"])
		missing := make([]string, 0)
		for name, rawProp := range properties {
			propMap, ok := rawProp.(map[string]any)
			if ok {
				propMap = normalizeStrictSchemaNode(propMap).(map[string]any)
				if !required[name] {
					makeStrictPropertyNullable(propMap)
					missing = append(missing, name)
				}
				properties[name] = propMap
				continue
			}
			if !required[name] {
				missing = append(missing, name)
			}
		}
		if schemaTypeIncludes(m["type"], "object") || len(properties) > 0 {
			if _, has := m["additionalProperties"]; !has {
				m["additionalProperties"] = false
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				requiredList := requiredNamesInOrder(m["required"])
				requiredList = append(requiredList, missing...)
				m["required"] = requiredList
			}
		}
	}
	if items, ok := m["items"].(map[string]any); ok {
		m["items"] = normalizeStrictSchemaNode(items)
	}
	return m
}

func cloneStrictSchemaNode(value any) any {
	switch x := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(x))
		for key, item := range x {
			clone[key] = cloneStrictSchemaNode(item)
		}
		return clone
	case []any:
		clone := make([]any, len(x))
		for i, item := range x {
			clone[i] = cloneStrictSchemaNode(item)
		}
		return clone
	case []string:
		clone := make([]string, len(x))
		copy(clone, x)
		return clone
	default:
		return value
	}
}

func requiredNames(value any) map[string]bool {
	names := make(map[string]bool)
	switch x := value.(type) {
	case []string:
		for _, name := range x {
			names[name] = true
		}
	case []any:
		for _, item := range x {
			if name, ok := item.(string); ok {
				names[name] = true
			}
		}
	}
	return names
}

func requiredNamesInOrder(value any) []string {
	switch x := value.(type) {
	case []string:
		out := make([]string, len(x))
		copy(out, x)
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if name, ok := item.(string); ok {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaTypeIncludes(value any, target string) bool {
	switch x := value.(type) {
	case string:
		return x == target
	case []string:
		if slices.Contains(x, target) {
			return true
		}
	case []any:
		for _, item := range x {
			if item == target {
				return true
			}
			if s, ok := item.(string); ok && s == target {
				return true
			}
		}
	}
	return false
}

func makeStrictPropertyNullable(schema map[string]any) {
	switch t := schema["type"].(type) {
	case string:
		schema["type"] = []any{t, "null"}
	case []string:
		if !schemaTypeIncludes(t, "null") {
			out := make([]string, len(t), len(t)+1)
			copy(out, t)
			out = append(out, "null")
			schema["type"] = out
		}
	case []any:
		if !schemaTypeIncludes(t, "null") {
			out := make([]any, len(t), len(t)+1)
			copy(out, t)
			out = append(out, "null")
			schema["type"] = out
		}
	}
}

// Translate converts PromptExecution into *openai.ChatCompletionNewParams (populates from exec.ModelConfig).
func (a *Adapter) Translate(ctx context.Context, exec *prompty.PromptExecution) (*openai.ChatCompletionNewParams, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	_ = ctx // interface requirement; OpenAI accepts image URL natively, no I/O in Translate
	// CachePoint is ignored: OpenAI Prompt Caching is applied automatically by the API (e.g. by prefix/size).
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
		unions, err := a.messageToUnions(msg)
		if err != nil {
			return nil, err
		}
		params.Messages = append(params.Messages, unions...)
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
	if exec.ResponseFormat != nil {
		name := exec.ResponseFormat.Name
		if name == "" {
			name = "response_schema"
		}
		schema := normalizeSchemaForStrict(exec.ResponseFormat.Schema)
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   name,
					Strict: openai.Bool(true),
					Schema: schema,
				},
			},
		}
	}
	return params, nil
}

// Execute performs the API call. Requires WithClient.
func (a *Adapter) Execute(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	return a.client.Chat.Completions.New(ctx, *req)
}

func (a *Adapter) messageToUnions(msg prompty.ChatMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case prompty.RoleSystem, prompty.RoleDeveloper:
		text := prompty.TextFromParts(msg.Content)
		return []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(text)}, nil
	case prompty.RoleUser:
		union, err := a.userMessage(msg.Content)
		if err != nil {
			return nil, err
		}
		return []openai.ChatCompletionMessageParamUnion{union}, nil
	case prompty.RoleAssistant:
		union, err := a.assistantMessage(msg.Content)
		if err != nil {
			return nil, err
		}
		return []openai.ChatCompletionMessageParamUnion{union}, nil
	case prompty.RoleTool:
		return a.toolResultMessages(msg.Content)
	default:
		return nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
	}
}

func (a *Adapter) userMessage(parts []prompty.ContentPart) (openai.ChatCompletionMessageParamUnion, error) {
	var contentParts []openai.ChatCompletionContentPartUnionParam
	hasImage := false
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			contentParts = append(contentParts, openai.TextContentPart(x.Text))
		case prompty.MediaPart:
			if x.MediaType != "image" {
				return openai.ChatCompletionMessageParamUnion{}, adapter.ErrUnsupportedContentType
			}
			hasImage = true
			part, err := mediaPartToOpenAI(x)
			if err != nil {
				return openai.ChatCompletionMessageParamUnion{}, err
			}
			contentParts = append(contentParts, part)
		default:
			return openai.ChatCompletionMessageParamUnion{}, adapter.ErrUnsupportedContentType
		}
	}
	if !hasImage {
		return openai.UserMessage(prompty.TextFromParts(parts)), nil
	}
	return openai.UserMessage(contentParts), nil
}

func mediaPartToOpenAI(p prompty.MediaPart) (openai.ChatCompletionContentPartUnionParam, error) {
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

func (a *Adapter) toolResultMessages(parts []prompty.ContentPart) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(parts))
	for _, p := range parts {
		if tr, ok := p.(prompty.ToolResultPart); ok {
			// SDK tool message content: string or array of text parts; fail-fast on MediaPart (SDK OfArrayOfContentParts type is text-only).
			for _, cp := range tr.Content {
				if _, ok := cp.(prompty.MediaPart); ok {
					return nil, adapter.ErrUnsupportedContentType
				}
			}
			text := prompty.TextFromParts(tr.Content)
			messages = append(messages, openai.ToolMessage(text, tr.ToolCallID))
		}
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	}
	return messages, nil
}

// ParseResponse converts *openai.ChatCompletion into *prompty.Response.
func (a *Adapter) ParseResponse(ctx context.Context, completion *openai.ChatCompletion) (*prompty.Response, error) {
	_ = ctx
	if completion == nil {
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
	usage := prompty.Usage{}
	if completion.Usage.PromptTokens != 0 || completion.Usage.CompletionTokens != 0 || completion.Usage.TotalTokens != 0 {
		usage.PromptTokens = int(completion.Usage.PromptTokens)
		usage.CompletionTokens = int(completion.Usage.CompletionTokens)
		usage.TotalTokens = int(completion.Usage.TotalTokens)
	}
	finishReason := ""
	if completion.Choices[0].FinishReason != "" {
		finishReason = completion.Choices[0].FinishReason
	}
	return &prompty.Response{Content: out, Usage: usage, FinishReason: finishReason}, nil
}

// ExecuteStream performs streaming chat completion. Requires WithClient.
func (a *Adapter) ExecuteStream(ctx context.Context, req *openai.ChatCompletionNewParams) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		if a.client == nil {
			yield(nil, adapter.ErrNoClient)
			return
		}
		stream := a.client.Chat.Completions.NewStreaming(ctx, *req)
		defer func() { _ = stream.Close() }()

		for stream.Next() {
			chunk := stream.Current()
			var content []prompty.ContentPart
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					content = append(content, prompty.TextPart{Text: delta.Content})
				}
				for _, tc := range delta.ToolCalls {
					part := prompty.ToolCallPart{ID: tc.ID, Name: tc.Function.Name, ArgsChunk: tc.Function.Arguments}
					content = append(content, part)
				}
			}
			usage := prompty.Usage{}
			if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 || chunk.Usage.TotalTokens != 0 {
				usage.PromptTokens = int(chunk.Usage.PromptTokens)
				usage.CompletionTokens = int(chunk.Usage.CompletionTokens)
				usage.TotalTokens = int(chunk.Usage.TotalTokens)
			}
			isFinished := len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != ""
			finishReason := ""
			if isFinished && chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
			resChunk := &prompty.ResponseChunk{Content: content, Usage: usage, IsFinished: isFinished, FinishReason: finishReason}
			if !yield(resChunk, nil) {
				// Policy: always check stream.Err() before exit; propagate if consumer stopped early.
				if err := stream.Err(); err != nil {
					yield(nil, err)
				}
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// Compile-time checks that Adapter implements ProviderAdapter and StreamerAdapter.
var (
	_ adapter.ProviderAdapter[*openai.ChatCompletionNewParams, *openai.ChatCompletion] = (*Adapter)(nil)
	_ adapter.StreamerAdapter[*openai.ChatCompletionNewParams]                         = (*Adapter)(nil)
)
