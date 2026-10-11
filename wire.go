package prompty

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ExecutionWireFormat       = "prompty.execution.v2"
	ContentPartWireText       = "text"
	ContentPartWireMedia      = "media"
	ContentPartWireReasoning  = "reasoning"
	ContentPartWireToolCall   = "tool_call"
	ContentPartWireToolResult = "tool_result"
)

// PromptExecutionWire is the stable JSON-safe wire representation of PromptExecution.
type PromptExecutionWire struct {
	ToolScope      *ToolScope            `json:"tool_scope,omitempty"`
	RequestCache   *CachePolicy          `json:"request_cache,omitempty"`
	CacheResource  *CacheResourceHandle  `json:"cache_resource,omitempty"`
	Format         string                `json:"format"`
	Messages       []MessageWire         `json:"messages"`
	Tools          []ToolWire            `json:"tools,omitempty"`
	HostedTools    []HostedToolRequest   `json:"hosted_tools,omitempty"`
	RequiredTools  []string              `json:"required_tools,omitempty"`
	ForcedTool     string                `json:"forced_tool,omitempty"`
	ModelOptions   *ModelOptionsWire     `json:"model_options,omitempty"`
	Metadata       PromptMetadata        `json:"metadata"`
	ResponseFormat *ResponseFormatWire   `json:"response_format,omitempty"`
	Provider       JSONDocument          `json:"provider,omitempty"`
	Requirements   ExecutionRequirements `json:"requirements,omitzero"`
}

// MessageWire is the JSON-safe representation of ChatMessage.
type MessageWire struct {
	ProviderState           []PartState         `json:"provider_state,omitempty"`
	Annotations             []PartAnnotation    `json:"annotations,omitempty"`
	MessageAnnotations      []MessageAnnotation `json:"message_annotations,omitempty"`
	ContinuationUnavailable bool                `json:"continuation_unavailable,omitempty"`
	Role                    Role                `json:"role"`
	Content                 []ContentPartWire   `json:"content"`
	CachePolicy             *CachePolicy        `json:"cache_policy,omitempty"`
	Provenance              *MessageProvenance  `json:"provenance,omitempty"`
	Metadata                JSONDocument        `json:"metadata,omitempty"`
	LayerKind               LayerKind           `json:"layer_kind,omitempty"`
}

// ContentPartWire is a tagged union for all public content part variants.
type ContentPartWire struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	MediaType   string            `json:"media_type,omitempty"`
	MIMEType    string            `json:"mime_type,omitempty"`
	URL         string            `json:"url,omitempty"`
	Data        []byte            `json:"data,omitempty"`
	ToolCallID  string            `json:"tool_call_id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Args        string            `json:"args,omitempty"`
	ArgsChunk   string            `json:"args_chunk,omitempty"`
	Content     []ContentPartWire `json:"content,omitempty"`
	IsError     bool              `json:"is_error,omitempty"`
	CachePolicy *CachePolicy      `json:"cache_policy,omitempty"`
}

// ToolWire is the JSON-safe representation of ToolDefinition.
type ToolWire struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Dialect      string       `json:"dialect,omitempty"`
	Parameters   JSONDocument `json:"parameters,omitempty"`
	Capabilities []string     `json:"capabilities,omitempty"`
}

// ResponseFormatWire is the JSON-safe representation of SchemaDefinition.
type ResponseFormatWire struct {
	Name        string       `json:"name,omitempty"`
	Description string       `json:"description,omitempty"`
	Dialect     string       `json:"dialect,omitempty"`
	Schema      JSONDocument `json:"schema"`
}

// ModelOptionsWire carries cross-provider model options; provider-specific payload is PromptExecutionWire.Provider.
type ModelOptionsWire struct {
	Model       string   `json:"model,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int64   `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// MarshalExecution converts PromptExecution into a stable JSON-safe wire DTO.
func MarshalExecution(exec *PromptExecution, policy WirePolicy) (PromptExecutionWire, error) {
	var zero PromptExecutionWire
	if exec == nil {
		return zero, errors.New("prompt execution wire: execution is nil")
	}
	if err := ValidateExecutionToolScope(exec); err != nil {
		return zero, err
	}
	if err := ValidateHostedToolRequests(exec.HostedTools); err != nil {
		return zero, err
	}
	if err := ValidateRequestCachePolicy(exec.RequestCache); err != nil {
		return zero, err
	}
	if exec.CacheResource != nil {
		if err := ValidateCacheResourceHandle(*exec.CacheResource); err != nil {
			return zero, err
		}
	}
	if err := ValidateOutputMode(exec.Requirements.OutputMode, exec.ResponseFormat); err != nil {
		return zero, err
	}
	if exec.Requirements.Override != nil {
		if profileErr := ValidateCapabilityProfile(*exec.Requirements.Override); profileErr != nil {
			return zero, profileErr
		}
	}
	if err := ValidateSchemaPolicy(exec.Requirements.Schema); err != nil {
		return zero, err
	}
	if captureErr := ValidateCaptureRequirement(exec.Requirements.Capture); captureErr != nil {
		return zero, captureErr
	}
	if cacheErr := ValidateCachePolicies(exec.Messages); cacheErr != nil {
		return zero, cacheErr
	}
	projected, err := projectMessages(exec.Messages, policy)
	if err != nil {
		return zero, err
	}
	messages, err := messagesToWire(projected)
	if err != nil {
		return zero, err
	}
	wire := PromptExecutionWire{
		ToolScope:      cloneToolScopePtr(exec.toolScope),
		RequestCache:   cloneCachePolicy(exec.RequestCache),
		CacheResource:  cloneCacheResource(exec.CacheResource),
		Format:         ExecutionWireFormat,
		Requirements:   cloneRequirements(exec.Requirements),
		Messages:       messages,
		Tools:          toolsToWire(exec.Tools),
		HostedTools:    CloneHostedToolRequests(exec.HostedTools),
		RequiredTools:  cloneStringSlice(exec.RequiredTools),
		ForcedTool:     exec.ForcedTool,
		ModelOptions:   modelOptionsToWire(exec.ModelOptions),
		Metadata:       clonePromptMetadata(exec.Metadata),
		ResponseFormat: responseFormatToWire(exec.ResponseFormat),
		Provider:       nil,
	}
	if exec.ModelOptions != nil {
		wire.Provider = CloneJSONDocument(exec.ModelOptions.ProviderSettings)
	}
	return wire, nil
}

// UnmarshalExecution converts a stable wire DTO into PromptExecution.
func UnmarshalExecution(wire PromptExecutionWire) (*PromptExecution, error) {
	if wire.Format != ExecutionWireFormat {
		return nil, ErrWireFormat
	}
	if err := ValidateRequestCachePolicy(wire.RequestCache); err != nil {
		return nil, err
	}
	if wire.Requirements.Override != nil {
		if profileErr := ValidateCapabilityProfile(*wire.Requirements.Override); profileErr != nil {
			return nil, profileErr
		}
	}
	serialized, encodeErr := json.Marshal(wire)
	if encodeErr != nil {
		return nil, encodeErr
	}
	if schemaErr := ValidateWireJSON(serialized, ExecutionWireFormat); schemaErr != nil {
		return nil, schemaErr
	}
	if err := ValidateHostedToolRequests(wire.HostedTools); err != nil {
		return nil, err
	}
	if policyErr := ValidateSchemaPolicy(wire.Requirements.Schema); policyErr != nil {
		return nil, policyErr
	}
	if wire.CacheResource != nil {
		if err := ValidateCacheResourceHandle(*wire.CacheResource); err != nil {
			return nil, err
		}
	}
	format := responseFormatFromWire(wire.ResponseFormat)
	if err := ValidateOutputMode(wire.Requirements.OutputMode, format); err != nil {
		return nil, err
	}
	if captureErr := ValidateCaptureRequirement(wire.Requirements.Capture); captureErr != nil {
		return nil, captureErr
	}
	messages, err := messagesFromWire(wire.Messages)
	if err != nil {
		return nil, err
	}
	if cacheErr := ValidateCachePolicies(messages); cacheErr != nil {
		return nil, cacheErr
	}
	exec := &PromptExecution{Validator: nil, Capture: nil,
		toolScope:      cloneToolScopePtr(wire.ToolScope),
		RequestCache:   cloneCachePolicy(wire.RequestCache),
		CacheResource:  cloneCacheResource(wire.CacheResource),
		Requirements:   cloneRequirements(wire.Requirements),
		Messages:       messages,
		Tools:          toolsFromWire(wire.Tools),
		HostedTools:    CloneHostedToolRequests(wire.HostedTools),
		RequiredTools:  cloneStringSlice(wire.RequiredTools),
		ForcedTool:     wire.ForcedTool,
		ModelOptions:   modelOptionsFromWire(wire.ModelOptions, wire.Provider),
		Metadata:       clonePromptMetadata(wire.Metadata),
		ResponseFormat: responseFormatFromWire(wire.ResponseFormat),
	}
	if err := ValidateExecutionToolScope(exec); err != nil {
		return nil, err
	}
	return exec, nil
}

func messagesToWire(messages []ChatMessage) ([]MessageWire, error) {
	out := make([]MessageWire, len(messages))
	for i, msg := range messages {
		parts, err := contentPartsToWire(msg.Content)
		if err != nil {
			return nil, fmt.Errorf("prompt execution wire: messages[%d]: %w", i, err)
		}
		out[i] = MessageWire{
			ProviderState:           clonePartStates(msg.ProviderState),
			Annotations:             clonePartAnnotations(msg.Annotations),
			MessageAnnotations:      cloneMessageAnnotations(msg.MessageAnnotations),
			ContinuationUnavailable: msg.ContinuationUnavailable,
			Role:                    msg.Role,
			Content:                 parts,
			CachePolicy:             cloneCachePolicy(msg.CachePolicy),
			Provenance:              cloneMessageProvenance(msg.Provenance),
			Metadata:                CloneJSONDocument(msg.Metadata),
			LayerKind:               msg.LayerKind,
		}
	}
	return out, nil
}

func messagesFromWire(messages []MessageWire) ([]ChatMessage, error) {
	out := make([]ChatMessage, len(messages))
	for i, msg := range messages {
		parts, err := contentPartsFromWire(msg.Content)
		if err != nil {
			return nil, fmt.Errorf("prompt execution wire: messages[%d]: %w", i, err)
		}
		out[i] = ChatMessage{
			ProviderState:           clonePartStates(msg.ProviderState),
			Annotations:             clonePartAnnotations(msg.Annotations),
			MessageAnnotations:      cloneMessageAnnotations(msg.MessageAnnotations),
			ContinuationUnavailable: msg.ContinuationUnavailable,
			Role:                    msg.Role,
			Content:                 parts,
			CachePolicy:             cloneCachePolicy(msg.CachePolicy),
			Provenance:              cloneMessageProvenance(msg.Provenance),
			Metadata:                CloneJSONDocument(msg.Metadata),
			LayerKind:               msg.LayerKind,
		}
		if associationErr := ValidatePartAssociations(
			parts,
			msg.ProviderState,
			msg.Annotations,
		); associationErr != nil {
			return nil, fmt.Errorf("prompt execution wire: messages[%d]: %w", i, associationErr)
		}
		if annotationErr := ValidateMessageAnnotations(msg.MessageAnnotations); annotationErr != nil {
			return nil, fmt.Errorf("prompt execution wire: messages[%d]: %w", i, annotationErr)
		}
	}
	return out, nil
}

func contentPartsToWire(parts []ContentPart) ([]ContentPartWire, error) {
	out := make([]ContentPartWire, len(parts))
	for i, part := range parts {
		wire, err := contentPartToWire(part)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
		out[i] = wire
	}
	return out, nil
}

func contentPartToWire(part ContentPart) (ContentPartWire, error) {
	switch x := part.(type) {
	case TextPart:
		return textPartToWire(x), nil
	case *TextPart:
		if x == nil {
			return ContentPartWire{}, errors.New("nil text content part")
		}
		return textPartToWire(*x), nil
	case MediaPart:
		return mediaPartToWire(x), nil
	case *MediaPart:
		if x == nil {
			return ContentPartWire{}, errors.New("nil media content part")
		}
		return mediaPartToWire(*x), nil
	case ReasoningPart:
		return reasoningPartToWire(x), nil
	case *ReasoningPart:
		if x == nil {
			return ContentPartWire{}, errors.New("nil reasoning content part")
		}
		return reasoningPartToWire(*x), nil
	case ToolCallPart:
		return toolCallPartToWire(x), nil
	case *ToolCallPart:
		if x == nil {
			return ContentPartWire{}, errors.New("nil tool call content part")
		}
		return toolCallPartToWire(*x), nil
	case ToolResultPart:
		return toolResultPartToWire(x)
	case *ToolResultPart:
		if x == nil {
			return ContentPartWire{}, errors.New("nil tool result content part")
		}
		return toolResultPartToWire(*x)
	default:
		var zero ContentPartWire
		return zero, fmt.Errorf("unsupported content part %T", part)
	}
}

func textPartToWire(part TextPart) ContentPartWire {
	wire := contentPartWireOfType(ContentPartWireText)
	wire.Text = part.Text
	wire.CachePolicy = cloneCachePolicy(part.CachePolicy)
	return wire
}

func mediaPartToWire(part MediaPart) ContentPartWire {
	wire := contentPartWireOfType(ContentPartWireMedia)
	wire.MediaType = part.MediaType
	wire.MIMEType = part.MIMEType
	wire.URL = part.URL
	wire.Data = append([]byte(nil), part.Data...)
	wire.CachePolicy = cloneCachePolicy(part.CachePolicy)
	return wire
}

func reasoningPartToWire(part ReasoningPart) ContentPartWire {
	wire := contentPartWireOfType(ContentPartWireReasoning)
	wire.Text = part.Text
	wire.CachePolicy = cloneCachePolicy(part.CachePolicy)
	return wire
}

func toolCallPartToWire(part ToolCallPart) ContentPartWire {
	wire := contentPartWireOfType(ContentPartWireToolCall)
	wire.ToolCallID = part.ID
	wire.Name = part.Name
	wire.Args = part.Args
	wire.ArgsChunk = part.ArgsChunk
	wire.CachePolicy = cloneCachePolicy(part.CachePolicy)
	return wire
}

func toolResultPartToWire(part ToolResultPart) (ContentPartWire, error) {
	content, err := contentPartsToWire(part.Content)
	if err != nil {
		var zero ContentPartWire
		return zero, err
	}
	wire := contentPartWireOfType(ContentPartWireToolResult)
	wire.ToolCallID = part.ToolCallID
	wire.Name = part.Name
	wire.Content = content
	wire.IsError = part.IsError
	wire.CachePolicy = cloneCachePolicy(part.CachePolicy)
	return wire, nil
}

func contentPartWireOfType(kind string) ContentPartWire {
	return ContentPartWire{
		Type:        kind,
		Text:        "",
		MediaType:   "",
		MIMEType:    "",
		URL:         "",
		Data:        nil,
		ToolCallID:  "",
		Name:        "",
		Args:        "",
		ArgsChunk:   "",
		Content:     nil,
		IsError:     false,
		CachePolicy: nil,
	}
}

func contentPartsFromWire(parts []ContentPartWire) ([]ContentPart, error) {
	out := make([]ContentPart, len(parts))
	for i, wire := range parts {
		part, err := contentPartFromWire(wire)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
		out[i] = part
	}
	return out, nil
}

func contentPartFromWire(wire ContentPartWire) (ContentPart, error) {
	switch wire.Type {
	case ContentPartWireText:
		return TextPart{Text: wire.Text, CachePolicy: cloneCachePolicy(wire.CachePolicy)}, nil
	case ContentPartWireMedia:
		return MediaPart{
			MediaType:   wire.MediaType,
			MIMEType:    wire.MIMEType,
			URL:         wire.URL,
			Data:        append([]byte(nil), wire.Data...),
			CachePolicy: cloneCachePolicy(wire.CachePolicy),
		}, nil
	case ContentPartWireReasoning:
		return ReasoningPart{Text: wire.Text, CachePolicy: cloneCachePolicy(wire.CachePolicy)}, nil
	case ContentPartWireToolCall:
		return ToolCallPart{
			ID:          wire.ToolCallID,
			Name:        wire.Name,
			Args:        wire.Args,
			ArgsChunk:   wire.ArgsChunk,
			CachePolicy: cloneCachePolicy(wire.CachePolicy),
		}, nil
	case ContentPartWireToolResult:
		content, err := contentPartsFromWire(wire.Content)
		if err != nil {
			return nil, err
		}
		return ToolResultPart{
			ToolCallID:  wire.ToolCallID,
			Name:        wire.Name,
			Content:     content,
			IsError:     wire.IsError,
			CachePolicy: cloneCachePolicy(wire.CachePolicy),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported content part wire type %q", wire.Type)
	}
}

func toolsToWire(tools []ToolDefinition) []ToolWire {
	out := make([]ToolWire, len(tools))
	for i, tool := range tools {
		out[i] = ToolWire{
			Name:         tool.Name,
			Description:  tool.Description,
			Dialect:      tool.Dialect,
			Parameters:   CloneJSONDocument(tool.Parameters),
			Capabilities: cloneStringSlice(tool.Capabilities),
		}
	}
	return out
}

func toolsFromWire(tools []ToolWire) []ToolDefinition {
	out := make([]ToolDefinition, len(tools))
	for i, tool := range tools {
		out[i] = ToolDefinition{
			Name:         tool.Name,
			Description:  tool.Description,
			Dialect:      tool.Dialect,
			Parameters:   CloneJSONDocument(tool.Parameters),
			Capabilities: cloneStringSlice(tool.Capabilities),
		}
	}
	return out
}

func responseFormatToWire(format *SchemaDefinition) *ResponseFormatWire {
	if format == nil {
		return nil
	}
	return &ResponseFormatWire{
		Name:        format.Name,
		Description: format.Description,
		Dialect:     format.Dialect,
		Schema:      CloneJSONDocument(format.Schema),
	}
}

func responseFormatFromWire(format *ResponseFormatWire) *SchemaDefinition {
	if format == nil {
		return nil
	}
	return &SchemaDefinition{
		Name:        format.Name,
		Description: format.Description,
		Dialect:     format.Dialect,
		Schema:      CloneJSONDocument(format.Schema),
	}
}

func modelOptionsToWire(opts *ModelOptions) *ModelOptionsWire {
	if opts == nil {
		return nil
	}
	out := &ModelOptionsWire{
		Model:       opts.Model,
		Temperature: nil,
		MaxTokens:   nil,
		TopP:        nil,
		Stop:        cloneStringSlice(opts.Stop),
	}
	if opts.Temperature != nil {
		v := *opts.Temperature
		out.Temperature = &v
	}
	if opts.MaxTokens != nil {
		v := *opts.MaxTokens
		out.MaxTokens = &v
	}
	if opts.TopP != nil {
		v := *opts.TopP
		out.TopP = &v
	}
	return out
}

func modelOptionsFromWire(wire *ModelOptionsWire, provider JSONDocument) *ModelOptions {
	if wire == nil && len(provider) == 0 {
		return nil
	}
	out := &ModelOptions{
		Model:            "",
		Temperature:      nil,
		MaxTokens:        nil,
		TopP:             nil,
		Stop:             nil,
		ProviderSettings: CloneJSONDocument(provider),
	}
	if wire == nil {
		return out
	}
	out.Model = wire.Model
	out.Stop = cloneStringSlice(wire.Stop)
	if wire.Temperature != nil {
		v := *wire.Temperature
		out.Temperature = &v
	}
	if wire.MaxTokens != nil {
		v := *wire.MaxTokens
		out.MaxTokens = &v
	}
	if wire.TopP != nil {
		v := *wire.TopP
		out.TopP = &v
	}
	return out
}
