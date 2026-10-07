package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"mime"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
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
	endpoint     string
	profiles     []prompty.CapabilityProfile
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelOptions does not contain Model.
func WithModel(m shared.ChatModel) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the OpenAI SDK client for Execute. Required for Execute/Invoker flow.
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

// RuntimeDescriptor exposes adapter runtime metadata.
func (a *Adapter) RuntimeDescriptor() adapter.RuntimeDescriptor {
	endpoint := a.endpoint
	if endpoint == "" && a.client == nil {
		endpoint = "https://api.openai.com/v1"
	}
	return adapter.CloneRuntimeDescriptor(adapter.RuntimeDescriptor{
		Name: "openai", Endpoint: endpoint, DefaultModel: a.defaultModel,
		Profiles:          a.profiles,
		ReasoningSettings: []string{"reasoning_effort"},
	})
}

// WithProfiles declares caller-owned knowledge for the injected client's actual
// endpoint. This option does not configure transport; the caller must bind the
// SDK client to the same endpoint. Unknown models retain unknown capabilities.
func WithProfiles(endpoint string, profiles ...prompty.CapabilityProfile) Option {
	snapshot := adapter.CloneRuntimeDescriptor(adapter.RuntimeDescriptor{Profiles: profiles})
	return func(a *Adapter) {
		a.endpoint = endpoint
		a.profiles = adapter.CloneRuntimeDescriptor(snapshot).Profiles
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

// Translate converts PromptExecution into *openai.ChatCompletionNewParams.
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*openai.ChatCompletionNewParams, error) {
	if err := adapter.RejectUnsupportedHostedTools(exec); err != nil {
		return nil, err
	}

	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, err
	}
	if err := adapter.ValidateMessageAnnotationMappings(exec, a.RuntimeDescriptor()); err != nil {
		return nil, err
	}
	if err := adapter.RejectSyntheticOutput(exec); err != nil {
		return nil, err
	}
	if err := adapter.RejectCacheResource(exec); err != nil {
		return nil, err
	}
	if cacheErr := adapter.RejectUnmappedCache(exec); cacheErr != nil {
		return nil, cacheErr
	}
	if stateErr := adapter.RejectProviderState(exec); stateErr != nil {
		return nil, stateErr
	}
	// Chat has distinct native system/developer roles; preserve message boundaries.
	working := exec.Clone()
	// Whole-request controls do not introduce message/part cache markers.
	params := &openai.ChatCompletionNewParams{
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(working.Messages)),
		Model:    a.defaultModel,
	}
	if err := applyChatRequestCache(params, exec.RequestCache); err != nil {
		return nil, err
	}
	if exec.ModelOptions != nil {
		if exec.ModelOptions.Model != "" {
			params.Model = shared.ChatModel(exec.ModelOptions.Model) //nolint:unconvert // ChatModel is a distinct type
		}
		if exec.ModelOptions.TopP != nil {
			params.TopP = openai.Float(*exec.ModelOptions.TopP)
		}
		if len(exec.ModelOptions.Stop) > 0 {
			params.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: exec.ModelOptions.Stop}
		}

		providerSettings, settingsErr := prompty.ProviderSettingsMap(exec.ModelOptions)
		if settingsErr != nil {
			return nil, settingsErr
		}
		_, err := applyOpenAIProviderSettings(params, providerSettings)
		if err != nil {
			return nil, err
		}
		if exec.ModelOptions.MaxTokens != nil {
			params.MaxCompletionTokens = openai.Int(*exec.ModelOptions.MaxTokens)
		}
		if exec.ModelOptions.Temperature != nil {
			params.Temperature = openai.Float(*exec.ModelOptions.Temperature)
		}
	}
	for _, msg := range working.Messages {
		unions, err := a.messageToUnions(msg)
		if err != nil {
			return nil, err
		}
		params.Messages = append(params.Messages, unions...)
	}
	if len(exec.Tools) > 0 {
		params.Tools = make([]openai.ChatCompletionToolUnionParam, 0, len(exec.Tools))
		for _, t := range exec.Tools {
			toolParams := map[string]any{"type": "object"}
			if len(t.Parameters) != 0 {
				var toolParamsErr error
				toolParams, toolParamsErr = prompty.SchemaDocumentAsMap(t.Parameters)
				if toolParamsErr != nil {
					return nil, fmt.Errorf("tool %q parameters: %w", t.Name, toolParamsErr)
				}
			}
			params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  shared.FunctionParameters(schemaSDKMap(toolParams)),
			}))
		}
		if exec.ForcedTool != "" {
			params.ToolChoice = openai.ToolChoiceOptionFunctionToolChoice(
				openai.ChatCompletionNamedToolChoiceFunctionParam{Name: exec.ForcedTool},
			)
		}
	}
	if exec.ResponseFormat != nil {
		name := exec.ResponseFormat.Name
		if name == "" {
			name = "response_schema"
		}
		schemaMap, schemaErr := prompty.SchemaMap(exec.ResponseFormat)
		if schemaErr != nil {
			return nil, fmt.Errorf("response_format schema: %w", schemaErr)
		}
		_, schemaErr = mapOutputSchema(schemaMap, prompty.SchemaPolicy{Mode: prompty.SchemaStrict})
		if schemaErr != nil {
			return nil, schemaErr
		}
		schema := schemaSDKNumbers(schemaMap)
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
	return a.client.Chat.Completions.New(ctx, *req, option.WithMaxRetries(0))
}

func (a *Adapter) messageToUnions(msg prompty.ChatMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case prompty.RoleSystem, prompty.RoleDeveloper:
		text, err := prompty.StrictTextFromParts(msg.Content)
		if err != nil {
			return nil, err
		}
		if msg.Role == prompty.RoleDeveloper {
			return []openai.ChatCompletionMessageParamUnion{openai.DeveloperMessage(text)}, nil
		}
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
	hasMedia := false
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			contentParts = append(contentParts, openai.TextContentPart(x.Text))
		case prompty.MediaPart:
			hasMedia = true
			part, err := mediaPartToOpenAI(x)
			if err != nil {
				return openai.ChatCompletionMessageParamUnion{}, err
			}
			contentParts = append(contentParts, part)
		default:
			return openai.ChatCompletionMessageParamUnion{}, adapter.ErrUnsupportedContentType
		}
	}
	if !hasMedia {
		text, err := prompty.JoinAdapterTextParts(parts)
		if err != nil {
			return openai.ChatCompletionMessageParamUnion{}, err
		}
		return openai.UserMessage(text), nil
	}
	return openai.UserMessage(contentParts), nil
}

func mediaPartToOpenAI(p prompty.MediaPart) (openai.ChatCompletionContentPartUnionParam, error) {
	mime := strings.ToLower(strings.TrimSpace(p.MIMEType))
	if mime == "" {
		mime = "application/octet-stream"
	}

	if strings.HasPrefix(mime, "audio/") {
		if len(p.Data) == 0 {
			if p.URL != "" {
				return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("%w", adapter.ErrMediaNotResolved)
			}
			return openai.ChatCompletionContentPartUnionParam{}, adapter.ErrUnsupportedContentType
		}
		format, err := openAIInputAudioFormat(mime)
		if err != nil {
			return openai.ChatCompletionContentPartUnionParam{}, err
		}
		return openai.InputAudioContentPart(openai.ChatCompletionContentPartInputAudioInputAudioParam{
			Data:   base64.StdEncoding.EncodeToString(p.Data),
			Format: format,
		}), nil
	}

	if strings.HasPrefix(mime, "image/") {
		url := p.URL
		if len(p.Data) > 0 {
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

	if mime != "application/pdf" {
		return openai.ChatCompletionContentPartUnionParam{}, adapter.ErrUnsupportedContentType
	}

	if len(p.Data) == 0 {
		if p.URL != "" {
			return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("%w", adapter.ErrMediaNotResolved)
		}
		return openai.ChatCompletionContentPartUnionParam{}, adapter.ErrUnsupportedContentType
	}
	return openai.FileContentPart(openai.ChatCompletionContentPartFileFileParam{
		Filename: openai.String("input.pdf"),
		FileData: openai.String("data:application/pdf;base64," + base64.StdEncoding.EncodeToString(p.Data)),
	}), nil
}

func openAIInputAudioFormat(value string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "", adapter.ErrUnsupportedContentType
	}
	switch mediaType {
	case "audio/mp3", "audio/mpeg":
		return "mp3", nil
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "wav", nil
	default:
		return "", adapter.ErrUnsupportedContentType
	}
}

func (a *Adapter) assistantMessage(parts []prompty.ContentPart) (openai.ChatCompletionMessageParamUnion, error) {
	var b strings.Builder
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			b.WriteString(x.Text)
		case prompty.ToolCallPart:
			args, argsErr := prompty.ToolCallArgsForTranslate(x)
			if argsErr != nil {
				return openai.ChatCompletionMessageParamUnion{}, argsErr
			}
			if args != "" && !json.Valid([]byte(args)) {
				return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf(
					"%w: invalid tool call args JSON",
					adapter.ErrMalformedArgs,
				)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: x.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      x.Name,
						Arguments: args,
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
		var tr prompty.ToolResultPart
		switch x := p.(type) {
		case prompty.ToolResultPart:
			tr = x
		case *prompty.ToolResultPart:
			if x == nil {
				return nil, adapter.ErrUnsupportedContentType
			}
			tr = *x
		default:
			return nil, fmt.Errorf("%w: unexpected %T in tool message", adapter.ErrUnsupportedContentType, p)
		}
		if tr.IsError {
			return nil, adapter.ErrUnsupportedContentType
		}
		// SDK tool message content: string or array of text parts; fail-fast on MediaPart (SDK OfArrayOfContentParts type is text-only).
		for _, cp := range tr.Content {
			if _, ok := cp.(prompty.MediaPart); ok {
				return nil, adapter.ErrUnsupportedContentType
			}
		}
		text, err := prompty.StrictTextFromParts(tr.Content)
		if err != nil {
			return nil, err
		}
		messages = append(messages, openai.ToolMessage(text, tr.ToolCallID))
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	}
	return messages, nil
}

// ParseResponse converts *openai.ChatCompletion into *prompty.Response.
func (a *Adapter) ParseResponse(completion *openai.ChatCompletion) (*prompty.Response, error) {
	if completion == nil {
		return nil, adapter.ErrInvalidResponse
	}
	if len(completion.Choices) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	if len(completion.Choices) != 1 || completion.Choices[0].Index != 0 {
		return nil, fmt.Errorf("%w: expected one choice at index zero", adapter.ErrInvalidResponse)
	}
	msg := completion.Choices[0].Message
	var out []prompty.ContentPart
	var annotations []prompty.PartAnnotation
	if msg.Content != "" {
		out = append(out, prompty.TextPart{Text: msg.Content})
	}
	if len(msg.Annotations) != 0 && len(out) == 0 {
		return nil, fmt.Errorf("%w: annotations without content", adapter.ErrInvalidResponse)
	}
	for _, annotation := range msg.Annotations {
		value := prompty.JSONDocument(annotation.RawJSON())
		if len(value) == 0 {
			var err error
			value, err = prompty.MarshalJSONDocument(annotation)
			if err != nil {
				return nil, fmt.Errorf("%w: annotation JSON: %w", adapter.ErrInvalidResponse, err)
			}
		}
		annotations = append(annotations, prompty.PartAnnotation{
			PartIndex: 0, Kind: "openai.annotation", Value: value,
		})
	}
	refused := msg.Refusal != "" || msg.JSON.Refusal.Valid()
	if refused {
		annotation := refusalAnnotation(len(out), msg.Refusal)
		annotations = append(annotations, annotation)
		out = append(out, prompty.TextPart{Text: msg.Refusal})
	}
	for _, tc := range msg.ToolCalls {
		if tc.Type != "function" {
			if tc.Type == "" {
				return nil, fmt.Errorf("%w: tool_call missing type", adapter.ErrMalformedArgs)
			}
			return nil, fmt.Errorf(
				"%w: unknown tool_call type %q",
				adapter.ErrUnsupportedContentType,
				tc.Type,
			)
		}
		if tc.Function.Name == "" {
			return nil, fmt.Errorf("%w: tool call missing function name", adapter.ErrMalformedArgs)
		}
		if tc.Function.Arguments == "" {
			return nil, fmt.Errorf(
				"%w: empty tool call args for %q",
				adapter.ErrMalformedArgs,
				tc.Function.Name,
			)
		}
		out = append(out, prompty.ToolCallPart{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}
	finishReason := completion.Choices[0].FinishReason
	outcome := outcomeFromOpenAI(finishReason)
	if refused {
		outcome = prompty.OutcomeRefusal
	}
	if len(out) == 0 && outcome != prompty.OutcomeIncomplete {
		return nil, adapter.ErrEmptyResponse
	}
	usage, usageErr := usageFromOpenAI(completion.Usage)
	model, modelErr := observedModel(completion.RawJSON(), completion.Model)
	if modelErr != nil && outcome != prompty.OutcomeRefusal {
		outcome = prompty.OutcomeIncomplete
	}
	return &prompty.Response{
		ContinuationUnavailable: modelErr != nil,
		Model:                   model,
		Content:                 out,
		Annotations:             annotations,
		Usage:                   usage,
		FinishReason:            finishReason,
		Outcome:                 outcome,
	}, errors.Join(usageErr, modelErr)
}

func refusalAnnotation(index int, refusal string) prompty.PartAnnotation {
	// JSON encoding a string cannot fail.
	value, _ := json.Marshal(refusal)
	return prompty.PartAnnotation{PartIndex: index, Kind: "openai.refusal", Value: value}
}

// StreamEvents performs native streaming chat completion. Requires WithClient.
func (a *Adapter) StreamEvents(
	ctx context.Context,
	req *openai.ChatCompletionNewParams,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		if a.client == nil {
			yield(nil, adapter.ErrNoClient)
			return
		}
		params := *req
		params.StreamOptions.IncludeUsage = openai.Bool(true)
		stream := a.client.Chat.Completions.NewStreaming(ctx, params, option.WithMaxRetries(0))
		defer func() { _ = stream.Close() }()
		identities := make(map[int64]prompty.ToolCallPart)
		indices := make(map[string]int64)
		refused := false

		for stream.Next() {
			chunk := stream.Current()
			captureErr := adapter.CaptureProviderChunk(ctx, chunk.RawJSON(), chunkCaptureMetadata(chunk))
			if len(chunk.Choices) > 1 || (len(chunk.Choices) == 1 &&
				(!chunk.Choices[0].JSON.Index.Valid() || chunk.Choices[0].Index != 0)) {
				yield(
					nil,
					fmt.Errorf(
						"%w: multiple/nonzero choices are not a single canonical response",
						errors.Join(adapter.ErrInvalidResponse, captureErr),
					),
				)
				return
			}
			var content []prompty.ContentPart
			var partIDs []string
			var annotations []prompty.PartAnnotation
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					content = append(content, prompty.TextPart{Text: delta.Content})
					partIDs = append(partIDs, "content:0")
				}
				if delta.Refusal != "" || delta.JSON.Refusal.Valid() {
					refused = true
					annotations = append(annotations, refusalAnnotation(len(content), delta.Refusal))
					content = append(content, prompty.TextPart{Text: delta.Refusal})
					partIDs = append(partIDs, "refusal:0")
				}
				for _, tc := range delta.ToolCalls {
					if !tc.JSON.Index.Valid() || tc.Index < 0 {
						yield(nil, fmt.Errorf("%w: missing or negative tool index",
							errors.Join(adapter.ErrInvalidResponse, captureErr)))
						return
					}
					identity := identities[tc.Index]
					if tc.ID != "" {
						if identity.ID != "" && identity.ID != tc.ID {
							yield(
								nil,
								fmt.Errorf(
									"%w: conflicting tool identity",
									errors.Join(adapter.ErrInvalidResponse, captureErr),
								),
							)
							return
						}
						identity.ID = tc.ID
					}
					if tc.Function.Name != "" {
						if identity.Name != "" && identity.Name != tc.Function.Name {
							yield(
								nil,
								fmt.Errorf(
									"%w: conflicting tool name",
									errors.Join(adapter.ErrInvalidResponse, captureErr),
								),
							)
							return
						}
						identity.Name = tc.Function.Name
					}
					if identity.ID == "" || identity.Name == "" {
						yield(
							nil,
							fmt.Errorf(
								"%w: uncorrelated tool delta",
								errors.Join(adapter.ErrInvalidResponse, captureErr),
							),
						)
						return
					}
					if previous, exists := indices[identity.ID]; exists && previous != tc.Index {
						yield(nil, fmt.Errorf("%w: tool identity reused across indices",
							errors.Join(adapter.ErrInvalidResponse, captureErr)))
						return
					}
					indices[identity.ID] = tc.Index
					identities[tc.Index] = identity
					part := prompty.ToolCallPart{
						ID: identity.ID, Name: identity.Name, ArgsChunk: tc.Function.Arguments,
					}
					content = append(content, part)
					partIDs = append(partIDs, fmt.Sprintf("tool:%d", tc.Index))
				}
			}
			usage, usageErr := usageFromOpenAI(chunk.Usage)
			model, modelErr := observedModel(chunk.RawJSON(), chunk.Model)
			isFinished := len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != ""
			finishReason := ""
			if isFinished && chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
			resChunk := &prompty.ResponseChunk{
				Model:        model,
				Identity:     prompty.StreamIdentity{MessageID: "", PartIDs: partIDs},
				Content:      content,
				Annotations:  annotations,
				Usage:        usage,
				Kind:         prompty.StreamDelta,
				FinishReason: finishReason,
			}
			if refused {
				resChunk.Outcome = prompty.OutcomeRefusal
			}
			if isFinished {
				resChunk.Kind = prompty.StreamFinish
				resChunk.Outcome = outcomeFromOpenAI(finishReason)
				if refused {
					resChunk.Outcome = prompty.OutcomeRefusal
				}
			} else if len(chunk.Choices) == 0 {
				resChunk.Kind = prompty.StreamUsage
			}
			if modelErr != nil {
				resChunk.Kind = prompty.StreamDelta
				resChunk.FinishReason = ""
				if !refused {
					resChunk.Outcome = prompty.OutcomeIncomplete
				}
				modelErr = errors.Join(modelErr, prompty.ErrStreamProtocol)
			}
			if !yield(resChunk, nil) {
				// Consumer requested stop; do not call yield again.
				return
			}
			if frameErr := errors.Join(usageErr, captureErr, modelErr); frameErr != nil {
				yield(nil, frameErr)
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

func usageFromOpenAI(usage openai.CompletionUsage) (prompty.Usage, error) {
	var out prompty.Usage
	out.Mode = prompty.UsageCumulative
	var usageErr error
	for _, field := range []struct {
		key         prompty.UsageCounter
		present     bool
		value       int64
		destination *int
	}{
		{prompty.UsagePrompt, usage.JSON.PromptTokens.Valid(), usage.PromptTokens, &out.PromptTokens},
		{prompty.UsageCompletion, usage.JSON.CompletionTokens.Valid(), usage.CompletionTokens, &out.CompletionTokens},
		{prompty.UsageTotal, usage.JSON.TotalTokens.Valid(), usage.TotalTokens, &out.TotalTokens},
		{prompty.UsageCacheRead, usage.PromptTokensDetails.JSON.CachedTokens.Valid(), usage.PromptTokensDetails.CachedTokens, &out.PromptTokensCached},
		{prompty.UsageReasoning, usage.CompletionTokensDetails.JSON.ReasoningTokens.Valid(),
			usage.CompletionTokensDetails.ReasoningTokens, &out.CompletionTokensReasoning},
	} {
		converted, err := adapter.SumUsageCounters(field.value)
		if err != nil {
			usageErr = errors.Join(usageErr, err)
			continue
		}
		*field.destination = converted
		// SDK fixtures constructed in Go have no wire presence metadata. A
		// nonzero value is known; an absent zero must remain unknown.
		if field.present || field.value != 0 {
			out.Known = append(out.Known, field.key)
		}
	}
	return out, usageErr
}

func outcomeFromOpenAI(reason string) prompty.Outcome {
	switch reason {
	case "stop":
		return prompty.OutcomeCompleted
	case "length", "content_filter":
		return prompty.OutcomeIncomplete
	case "tool_calls":
		return prompty.OutcomeToolCalls
	default:
		return prompty.OutcomeIncomplete
	}
}

func parseReasoningEffort(v string) (shared.ReasoningEffort, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none":
		return shared.ReasoningEffortNone, true
	case "minimal":
		return shared.ReasoningEffortMinimal, true
	case "low":
		return shared.ReasoningEffortLow, true
	case "medium":
		return shared.ReasoningEffortMedium, true
	case "high":
		return shared.ReasoningEffortHigh, true
	case "xhigh":
		return shared.ReasoningEffortXhigh, true
	default:
		return "", false
	}
}
