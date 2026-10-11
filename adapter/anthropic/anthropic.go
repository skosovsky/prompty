package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/cast"
)

const defaultMaxTokens int64 = 1024

// Adapter implements adapter.ProviderAdapter for the Anthropic Messages API.
// Req = *anthropic.MessageNewParams, Resp = *anthropic.Message.
type Adapter struct {
	defaultModel anthropic.Model
	client       *anthropic.Client
	endpoint     string
	profiles     []prompty.CapabilityProfile
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
	a := &Adapter{client: nil, endpoint: "", profiles: nil, defaultModel: "claude-sonnet-4-5-20250929"}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// RuntimeDescriptor exposes adapter runtime metadata.
func (a *Adapter) RuntimeDescriptor() adapter.RuntimeDescriptor {
	endpoint := a.endpoint
	if endpoint == "" && a.client == nil {
		endpoint = "https://api.anthropic.com"
	}
	return adapter.CloneRuntimeDescriptor(adapter.RuntimeDescriptor{
		Name: wireAnthropic, Endpoint: endpoint, DefaultModel: a.defaultModel,
		Profiles:          a.profiles,
		ReasoningSettings: []string{"thinking_mode", "thinking_budget", "thinking_visibility", "reasoning_effort"},
		HostedToolCapabilities: map[string]string{HostedWebSearch: prompty.CapabilityHostedWebSearch,
			HostedWebFetch: prompty.CapabilityHostedWebFetch},
		MessageAnnotationCapabilities: map[string][]string{
			hostedHistoryAnnotation:         {prompty.CapabilityProviderState},
			hostedAnnotation(wireWebSearch): {prompty.CapabilityHostedWebSearch},
			hostedAnnotation(wireWebFetch):  {prompty.CapabilityHostedWebFetch},
		},
	})
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

// Translate converts PromptExecution into *anthropic.MessageNewParams.
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*anthropic.MessageNewParams, error) {
	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, err
	}
	if err := adapter.ValidateMessageAnnotationMappings(exec, a.RuntimeDescriptor()); err != nil {
		return nil, err
	}
	if err := adapter.RejectRequestCache(exec); err != nil {
		return nil, err
	}
	if err := adapter.RejectSyntheticOutput(exec); err != nil {
		return nil, err
	}
	if err := adapter.RejectCacheResource(exec); err != nil {
		return nil, err
	}
	working := adapter.PrepareTranslateExecution(exec)
	hosted, err := prepareHostedTools(working)
	if err != nil {
		return nil, err
	}
	if len(hosted) > 0 || hasHostedHistory(working.Messages) {
		if _, preflightErr := adapter.Preflight(
			a.RuntimeDescriptor(),
			working,
			prompty.StreamBuffered,
			true,
		); preflightErr != nil {
			return nil, preflightErr
		}
	}
	if stateErr := a.ValidateProviderState(working, a.destinationIdentity(working)); stateErr != nil {
		return nil, stateErr
	}
	params := &anthropic.MessageNewParams{
		MaxTokens: defaultMaxTokens,
		Model:     a.defaultModel,
		Tools:     hosted,
	}
	if optionsErr := applyModelOptions(params, exec); optionsErr != nil {
		return nil, optionsErr
	}
	systemBlocks, messages, err := a.translateMessages(working.Messages)
	if err != nil {
		return nil, err
	}
	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}
	params.Messages = messages
	if toolsErr := applyExecutionTools(params, exec); toolsErr != nil {
		return nil, toolsErr
	}
	if cacheErr := validateCacheMarkers(params); cacheErr != nil {
		return nil, cacheErr
	}
	return params, nil
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
	if t, ok := params[wireType].(string); ok && t != "" {
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
	known := map[string]bool{wireType: true, "properties": true, "required": true}
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

func (a *Adapter) systemMessageBlocks(
	parts []prompty.ContentPart,
	messageCache *prompty.CachePolicy,
) ([]anthropic.TextBlockParam, error) {
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
		cache, err := toAnthropicCacheControl(resolveCacheControl(messageCache, textPart.CachePolicy))
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

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (a *Adapter) userMessage(
	parts []prompty.ContentPart,
	messageCache *prompty.CachePolicy,
) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CachePolicy))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.TextPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CachePolicy))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case prompty.MediaPart:
			cache := resolveCacheControl(messageCache, x.CachePolicy)
			block, err := a.mediaBlock(x, cache)
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.MediaPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			cache := resolveCacheControl(messageCache, x.CachePolicy)
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

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (a *Adapter) assistantMessage(
	parts []prompty.ContentPart,
	messageCache *prompty.CachePolicy,
) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CachePolicy))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.TextPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			block := anthropic.NewTextBlock(x.Text)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CachePolicy))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case prompty.ToolCallPart:
			args, argsErr := prompty.ToolCallArgsForTranslate(x)
			if argsErr != nil {
				return anthropic.MessageParam{}, argsErr
			}
			if args != "" && !json.Valid([]byte(args)) {
				return anthropic.MessageParam{}, fmt.Errorf("%w: invalid tool call args JSON", adapter.ErrMalformedArgs)
			}
			block := anthropic.NewToolUseBlock(x.ID, json.RawMessage(args), x.Name)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CachePolicy))
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			blocks = append(blocks, block)
		case *prompty.ToolCallPart:
			if x == nil {
				return anthropic.MessageParam{}, adapter.ErrUnsupportedContentType
			}
			args, argsErr := prompty.ToolCallArgsForTranslate(*x)
			if argsErr != nil {
				return anthropic.MessageParam{}, argsErr
			}
			if args != "" && !json.Valid([]byte(args)) {
				return anthropic.MessageParam{}, fmt.Errorf("%w: invalid tool call args JSON", adapter.ErrMalformedArgs)
			}
			block := anthropic.NewToolUseBlock(x.ID, json.RawMessage(args), x.Name)
			block, err := a.applyCacheControl(block, resolveCacheControl(messageCache, x.CachePolicy))
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

func (a *Adapter) toolResultMessage(
	parts []prompty.ContentPart,
	messageCache *prompty.CachePolicy,
) (anthropic.MessageParam, error) {
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
			return anthropic.MessageParam{}, fmt.Errorf(
				"%w: unexpected %T in tool message",
				adapter.ErrUnsupportedContentType,
				p,
			)
		}
		{
			toolResultCache := resolveCacheControl(messageCache, tr.CachePolicy)
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
		return anthropic.MessageParam{}, fmt.Errorf(
			"%w: tool message missing ToolResultPart",
			adapter.ErrUnsupportedContentType,
		)
	}
	return anthropic.NewUserMessage(blocks...), nil
}

func (a *Adapter) toolResultContentBlock(
	part prompty.ContentPart,
	inheritedCache *prompty.CachePolicy,
) (anthropic.ToolResultBlockParamContentUnion, error) {
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

func (a *Adapter) mediaBlock(
	part prompty.MediaPart,
	cache *prompty.CachePolicy,
) (anthropic.ContentBlockParamUnion, error) {
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
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf(
			"%w: MediaPart has neither Data nor URL",
			adapter.ErrUnsupportedContentType,
		)
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
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf(
			"%w: unsupported media MIME %q",
			adapter.ErrUnsupportedContentType,
			mime,
		)
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
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf(
			"%w: unsupported URL media MIME %q",
			adapter.ErrUnsupportedContentType,
			mime,
		)
	}
}

func (a *Adapter) applyCacheControl(
	block anthropic.ContentBlockParamUnion,
	cache *prompty.CachePolicy,
) (anthropic.ContentBlockParamUnion, error) {
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

func toAnthropicCacheControl(cache *prompty.CachePolicy) (*anthropic.CacheControlEphemeralParam, error) {
	if cache == nil {
		return nil, nil //nolint:nilnil // No mapped value or emitted frame is a successful absence, not an operation failure.
	}
	if err := prompty.ValidateCachePolicy(cache); err != nil {
		return nil, err
	}
	if cache.Provider != "" && cache.Provider != wireAnthropic || cache.Mechanism != prompty.CacheBlockMarker {
		if cache.Mode == prompty.CacheRequired {
			return nil, prompty.ErrCacheRequirement
		}
		return nil, nil //nolint:nilnil // No mapped value or emitted frame is a successful absence, not an operation failure.
	}
	ttl, err := anthropicCacheTTL(cache)
	if err != nil {
		return nil, err
	}
	c := anthropic.NewCacheControlEphemeralParam()
	c.TTL = ttl
	return &c, nil
}

func resolveCacheControl(messageCache, partCache *prompty.CachePolicy) *prompty.CachePolicy {
	if partCache != nil {
		return partCache
	}
	return messageCache
}

func contentPartCacheControl(part prompty.ContentPart) *prompty.CachePolicy {
	switch x := part.(type) {
	case prompty.TextPart:
		return x.CachePolicy
	case *prompty.TextPart:
		if x == nil {
			return nil
		}
		return x.CachePolicy
	case prompty.MediaPart:
		return x.CachePolicy
	case *prompty.MediaPart:
		if x == nil {
			return nil
		}
		return x.CachePolicy
	case prompty.ReasoningPart:
		return x.CachePolicy
	case *prompty.ReasoningPart:
		if x == nil {
			return nil
		}
		return x.CachePolicy
	case prompty.ToolCallPart:
		return x.CachePolicy
	case *prompty.ToolCallPart:
		if x == nil {
			return nil
		}
		return x.CachePolicy
	case prompty.ToolResultPart:
		return x.CachePolicy
	case *prompty.ToolResultPart:
		if x == nil {
			return nil
		}
		return x.CachePolicy
	default:
		return nil
	}
}

// Execute performs the API call. Requires WithClient.
func (a *Adapter) Execute(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	if req == nil {
		return nil, adapter.ErrNilExecution
	}
	raw, err := a.client.Messages.New(ctx, *req, option.WithMaxRetries(0))
	return raw, adapter.SafeProviderError(ctx, err)
}

// ParseResponse converts *anthropic.Message into *prompty.Response.
func (a *Adapter) ParseResponse(msg *anthropic.Message) (*prompty.Response, error) {
	desc := a.RuntimeDescriptor()
	return a.ParseResponseForProfile(msg, prompty.ProfileIdentity{ModelFamily: "",
		Provider: desc.Name, Endpoint: desc.Endpoint, Model: desc.DefaultModel,
	})
}

// ParseResponseForProfile binds opaque continuation blocks to the effective request destination.
//
//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (a *Adapter) ParseResponseForProfile(
	msg *anthropic.Message,
	destination prompty.ProfileIdentity,
) (*prompty.Response, error) {
	if msg == nil {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	var states []prompty.PartState
	var annotations []prompty.PartAnnotation
	for _, block := range msg.Content {
		if err := validateConsumedBlockSource(block); err != nil {
			return partialAnthropicSourceFailure(out, states, annotations, err)
		}
		switch block.Type {
		case wireText:
			index := len(out)
			out = append(out, prompty.TextPart{CachePolicy: nil, Text: block.Text})
			for _, citation := range block.Citations {
				value := prompty.JSONDocument(citation.RawJSON())
				if len(value) == 0 {
					var err error
					value, err = prompty.MarshalJSONDocument(citation)
					if err != nil {
						return nil, adapter.ErrInvalidResponse
					}
				}
				annotations = append(annotations, prompty.PartAnnotation{
					PartIndex: index, Kind: wireAnthropicCitation, Value: value,
				})
			}
		case wireThinking, wireRedactedThinking:
			state, part, err := thinkingState(block, len(out), destination)
			if err != nil {
				return nil, err
			}
			states = append(states, state)
			out = append(out, part)
		case wireToolUse:
			args := string(block.Input)
			if args == "" {
				return nil, fmt.Errorf(
					"%w: empty tool_use args for %q",
					adapter.ErrMalformedArgs,
					block.Name,
				)
			}
			out = append(
				out,
				prompty.ToolCallPart{ArgsChunk: "", CachePolicy: nil, ID: block.ID, Name: block.Name, Args: args},
			)
		case wireServerToolUse, wireWebSearchToolResult, wireWebFetchToolResult:
			// Hosted execution remains provider metadata; source validation below
			// checks types and call/result linkage without authorizing a handler.
		default:
			blockType := block.Type
			if blockType == "" {
				blockType = "<empty>"
			}
			return nil, fmt.Errorf(
				"%w: unknown content block type %q",
				adapter.ErrUnsupportedContentType,
				blockType,
			)
		}
	}
	outcome := anthropicOutcome(msg.StopReason)
	if len(out) == 0 && (msg.StopReason == "" || outcome == prompty.OutcomeToolCalls) {
		return nil, adapter.ErrEmptyResponse
	}
	resp := prompty.NewResponse(out)
	model, modelErr := observedMessageModel(msg)
	resp.Model = model
	if modelErr != nil {
		resp.ContinuationUnavailable = true
	}
	resp.ProviderState = states
	resp.Annotations = annotations
	if err := preserveHostedHistory(msg, resp, destination); err != nil {
		return nil, err
	}
	resp.Outcome = outcome
	usage, usageErr := usageFromAnthropic(msg.Usage)
	resp.Usage = usage
	resp.ContinuationUnavailable = resp.ContinuationUnavailable || usageErr != nil
	if msg.StopReason != "" {
		resp.FinishReason = string(msg.StopReason)
	}
	if err := prompty.ValidatePartAssociations(resp.Content, resp.ProviderState, resp.Annotations); err != nil {
		return nil, err
	}
	return resp, errors.Join(modelErr, usageErr)
}

var _ adapter.ProviderAdapter[*anthropic.MessageNewParams, *anthropic.Message] = (*Adapter)(nil)

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func usageFromAnthropic(usage anthropic.Usage) (prompty.Usage, error) {
	invalid, sourceErr := sanitizeObservedUsage(&usage)
	if invalid == nil {
		return prompty.Usage{}, sourceErr
	}
	_, inputErr := adapter.SumUsageCounters(usage.InputTokens)
	output, outputErr := adapter.SumUsageCounters(usage.OutputTokens)
	read, readErr := adapter.SumUsageCounters(usage.CacheReadInputTokens)
	write, writeErr := adapter.SumUsageCounters(usage.CacheCreationInputTokens)
	var promptErr, totalErr error
	out := prompty.Usage{PromptTokens: 0, TotalTokens: 0, CompletionTokensReasoning: 0, Known: nil,
		Mode:                      prompty.UsageCumulative,
		CompletionTokens:          output,
		PromptTokensCached:        read,
		PromptTokensCacheCreation: write,
	}
	inputKnown := !invalid["input_tokens"] && (usage.JSON.InputTokens.Valid() || usage.InputTokens != 0)
	readKnown := !invalid["cache_read_input_tokens"] &&
		(usage.JSON.CacheReadInputTokens.Valid() || usage.CacheReadInputTokens != 0)
	writeKnown := !invalid["cache_creation_input_tokens"] &&
		(usage.JSON.CacheCreationInputTokens.Valid() || usage.CacheCreationInputTokens != 0)
	outputKnown := !invalid["output_tokens"] && (usage.JSON.OutputTokens.Valid() || usage.OutputTokens != 0)
	if inputKnown && readKnown && writeKnown {
		out.PromptTokens, promptErr = adapter.SumUsageCounters(
			usage.InputTokens,
			usage.CacheCreationInputTokens,
			usage.CacheReadInputTokens,
		)
		if promptErr == nil {
			out.Known = append(out.Known, prompty.UsagePrompt)
		}
	}
	if outputKnown && outputErr == nil {
		out.Known = append(out.Known, prompty.UsageCompletion)
	}
	if inputKnown && readKnown && writeKnown && outputKnown {
		out.TotalTokens, totalErr = adapter.SumUsageCounters(
			usage.InputTokens,
			usage.CacheCreationInputTokens,
			usage.CacheReadInputTokens,
			usage.OutputTokens,
		)
		if totalErr == nil {
			out.Known = append(out.Known, prompty.UsageTotal)
		}
	}
	if readKnown && readErr == nil {
		out.Known = append(out.Known, prompty.UsageCacheRead)
	}
	if writeKnown && writeErr == nil {
		out.Known = append(out.Known, prompty.UsageCacheWrite)
	}
	return out, errors.Join(sourceErr, inputErr, outputErr, readErr, writeErr, promptErr, totalErr)
}

func anthropicProviderSettingKeys() []string {
	return []string{
		"top_k",
		"stop_sequences",
		"thinking_mode",
		"thinking_budget",
		"thinking_visibility",
		"reasoning_effort",
	}
}

func applyAnthropicProviderSettings(params *anthropic.MessageNewParams, settings map[string]any) error {
	if len(settings) == 0 {
		return nil
	}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, anthropicProviderSettingKeys()); err != nil {
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
		params.TopK = anthropic.Int(int64(topK))
	}
	if raw, ok := settings["stop_sequences"]; ok {
		stops, err := cast.ToStringSlice(raw)
		if err != nil {
			return adapter.ProviderSettingError("stop_sequences", err)
		}
		params.StopSequences = stops
	}
	return applyThinkingSettings(params, settings)
}

func applyModelOptions(params *anthropic.MessageNewParams, exec *prompty.PromptExecution) error {
	if exec.ModelOptions == nil {
		return nil
	}
	if exec.ModelOptions.Model != "" {
		params.Model = exec.ModelOptions.Model
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
	providerSettings, settingsErr := prompty.ProviderSettingsMap(exec.ModelOptions)
	if settingsErr != nil {
		return settingsErr
	}
	if err := applyAnthropicProviderSettings(params, providerSettings); err != nil {
		return err
	}

	return nil
}

func applyExecutionTools(params *anthropic.MessageNewParams, exec *prompty.PromptExecution) error {
	// Native output format is independent of local tools and tool choice.
	if exec.ResponseFormat != nil && len(exec.ResponseFormat.Schema) > 0 {
		schemaMap, schemaErr := prompty.SchemaMap(exec.ResponseFormat)
		if schemaErr != nil {
			return fmt.Errorf("response_format schema: %w", schemaErr)
		}
		//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
		params.OutputConfig.Format = anthropic.JSONOutputFormatParam{
			Schema: sdkSchemaMapNumbers(schemaMap),
		}
	}
	if len(exec.Tools) > 0 && params.Tools == nil {
		params.Tools = make([]anthropic.ToolUnionParam, 0, len(exec.Tools))
	}
	for _, t := range exec.Tools {
		toolParams := map[string]any{wireType: "object"}
		if len(t.Parameters) != 0 {
			var toolParamsErr error
			toolParams, toolParamsErr = prompty.SchemaDocumentAsMap(t.Parameters)
			if toolParamsErr != nil {
				return fmt.Errorf("tool %q parameters: %w", t.Name, toolParamsErr)
			}
		}
		schema := toolSchemaFromParameters(sdkSchemaMapNumbers(toolParams))
		tool := anthropic.ToolUnionParamOfTool(schema, t.Name)
		if t.Description != "" {
			tool.OfTool.Description = anthropic.String(t.Description)
		}
		params.Tools = append(params.Tools, tool)
	}
	if exec.ForcedTool != "" && len(params.Tools) > 0 {
		params.ToolChoice = anthropic.ToolChoiceParamOfTool(exec.ForcedTool)
	}

	return nil
}

func (a *Adapter) translateMessages(
	input []prompty.ChatMessage,
) ([]anthropic.TextBlockParam, []anthropic.MessageParam, error) {
	var systemBlocks []anthropic.TextBlockParam
	var messages []anthropic.MessageParam
	for _, msg := range input {
		switch msg.Role {
		case prompty.RoleSystem, prompty.RoleDeveloper:
			blocks, err := a.systemMessageBlocks(msg.Content, msg.CachePolicy)
			if err != nil {
				return nil, nil, err
			}
			systemBlocks = append(systemBlocks, blocks...)
		case prompty.RoleUser:
			m, err := a.userMessage(msg.Content, msg.CachePolicy)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, m)
		case prompty.RoleAssistant:
			m, err := a.assistantMessageWithState(msg)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, m)
		case prompty.RoleTool:
			m, err := a.toolResultMessage(msg.Content, msg.CachePolicy)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, m)
		default:
			return nil, nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
		}
	}

	return systemBlocks, messages, nil
}
