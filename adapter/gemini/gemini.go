package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/cast"

	"google.golang.org/genai"
)

// geminiSyntheticUserTrigger is appended as a user Content when the prompt is system-only.
// Gemini requires non-empty Contents; system text lives in SystemInstruction.
const geminiSyntheticUserTrigger = "Proceed according to the system instructions."

// Request wraps Contents, Config and Model for Gemini GenerateContent API.
type Request struct {
	Model    string
	Contents []*genai.Content
	Config   *genai.GenerateContentConfig
}

// Adapter implements adapter.ProviderAdapter for the Google Gemini (genai) API.
// Req = *Request, Resp = *genai.GenerateContentResponse.
type Adapter struct {
	defaultModel        string
	client              *genai.Client
	responseClient      *genai.Client
	responseLimit       int
	endpoint            string
	profiles            []prompty.CapabilityProfile
	cacheAccountBinding string
	nativeJSONSchema    bool
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithNativeJSONSchema selects exact JSON Schema transport for the documented subset.
// Endpoint/model support must be declared through the existing capability profiles.
func WithNativeJSONSchema() Option {
	return func(a *Adapter) { a.nativeJSONSchema = true }
}

// WithModel sets the default model used when exec.ModelOptions does not contain Model.
func WithModel(m string) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the genai client for Execute. Required for Execute/Invoker flow.
func WithClient(c *genai.Client) Option {
	return func(a *Adapter) {
		a.client = c
		a.responseClient = isolatedResponseClient(c)
	}
}

// WithResponsePayloadLimit bounds original response observation. Nonpositive
// limits fail before dispatch. Signed state cannot use a truncated payload.
// Sync SDK body reads are not capped by this retention limit. Native observation
// bounds each SSE line including framing, not the whole response.
func WithResponsePayloadLimit(limit int) Option {
	return func(a *Adapter) { a.responseLimit = limit }
}

// New returns an Adapter with default model "gemini-2.0-flash".
func New(opts ...Option) *Adapter {
	a := &Adapter{
		client:              nil,
		responseClient:      nil,
		endpoint:            "",
		profiles:            nil,
		cacheAccountBinding: "",
		defaultModel:        "gemini-2.0-flash",
		responseLimit:       defaultResponsePayloadLimit,
		nativeJSONSchema:    false,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// RuntimeDescriptor exposes adapter runtime metadata.
func (a *Adapter) RuntimeDescriptor() adapter.RuntimeDescriptor {
	endpoint := a.endpoint
	if endpoint == "" && a.client == nil {
		endpoint = "https://generativelanguage.googleapis.com"
	}
	return adapter.CloneRuntimeDescriptor(
		adapter.RuntimeDescriptor{HostedToolCapabilities: nil, MessageAnnotationCapabilities: nil,
			Name: wireGemini, Endpoint: endpoint, DefaultModel: a.defaultModel,
			Profiles:          a.profiles,
			ReasoningSettings: []string{"thinking", "thinking_budget", "thinking_level"},
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

// Translate converts PromptExecution into *Request (Contents + Config).
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*Request, error) {
	if err := a.validateTranslation(exec); err != nil {
		return nil, err
	}
	working := adapter.PrepareTranslateExecution(exec)
	if err := validateGeminiCorrelation(working.Messages); err != nil {
		return nil, err
	}
	if err := a.validateFunctionResultMedia(working); err != nil {
		return nil, err
	}
	if stateErr := a.ValidateProviderState(working, a.destinationIdentity(working)); stateErr != nil {
		return nil, stateErr
	}
	config := &genai.GenerateContentConfig{}
	if err := a.applyCacheResource(config, exec); err != nil {
		return nil, err
	}
	// Model is set on the genai request, not inside Config.
	applyModelOptions(config, exec)
	providerSettings, err := modelProviderSettings(exec.ModelOptions)
	if err != nil {
		return nil, err
	}
	wantGoogleSearch, err := applyGeminiProviderSettings(config, providerSettings)
	if err != nil {
		return nil, err
	}
	systemParts, contents, err := a.translateMessages(working.Messages)
	if err != nil {
		return nil, err
	}
	if exec.ForcedTool != "" {
		config.ToolConfig = &genai.ToolConfig{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
			RetrievalConfig: nil,
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAny, AllowedFunctionNames: []string{exec.ForcedTool},
				StreamFunctionCallArguments: nil,
			},
		}
	}
	if len(systemParts) > 0 {
		config.SystemInstruction = genai.NewContentFromText(strings.Join(systemParts, "\n\n"), genai.RoleUser)
	}
	if err := a.applyToolsAndResponseFormat(config, exec, wantGoogleSearch); err != nil {
		return nil, err
	}
	if controlErr := a.applyPartialControl(config, exec, providerSettings); controlErr != nil {
		return nil, controlErr
	}
	model := a.defaultModel
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		model = exec.ModelOptions.Model
	}
	// Gemini rejects requests with empty Contents; system-only prompts need a synthetic user turn.
	if len(contents) == 0 {
		if len(systemParts) > 0 {
			contents = append(contents, genai.NewContentFromText(geminiSyntheticUserTrigger, genai.RoleUser))
		} else {
			return nil, errors.New("prompty/adapter/gemini: empty contents and no system instructions")
		}
	}
	if err := a.preserveSchemaNumbers(config); err != nil {
		return nil, err
	}
	return &Request{Model: model, Contents: contents, Config: config}, nil
}

// Execute performs the API call. Requires WithClient.
func (a *Adapter) Execute(ctx context.Context, req *Request) (*genai.GenerateContentResponse, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	if req == nil {
		return nil, adapter.ErrNilExecution
	}
	if requestsPartialArguments(req) {
		return nil, adapter.ErrUnsupportedCapability
	}
	return a.executeObservedResponse(ctx, req)
}

func (a *Adapter) userContent(parts []prompty.ContentPart) (*genai.Content, error) {
	var genParts []*genai.Part
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			genParts = append(genParts, genai.NewPartFromText(x.Text))
		case prompty.MediaPart:
			switch {
			case len(x.Data) > 0:
				mime := x.MIMEType
				if mime == "" {
					mime = "application/octet-stream"
				}
				genParts = append(genParts, genai.NewPartFromBytes(x.Data, mime))
			case x.URL != "":
				mime := x.MIMEType
				if mime == "" {
					mime = "application/octet-stream"
				}
				genParts = append(genParts, genai.NewPartFromURI(x.URL, mime))
			default:
				return nil, fmt.Errorf("%w: MediaPart has neither Data nor URL", adapter.ErrUnsupportedContentType)
			}
		default:
			return nil, adapter.ErrUnsupportedContentType
		}
	}
	if len(genParts) == 0 {
		return genai.NewContentFromText("", genai.RoleUser), nil
	}
	return genai.NewContentFromParts(genParts, genai.RoleUser), nil
}

func (a *Adapter) assistantContent(parts []prompty.ContentPart) (*genai.Content, error) {
	var genParts []*genai.Part
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			genParts = append(genParts, genai.NewPartFromText(x.Text))
		case prompty.ReasoningPart:
			genParts = append(
				genParts,
				//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
				&genai.Part{Text: x.Text, Thought: true},
			)
		case prompty.ToolCallPart:
			argsJSON, argsErr := prompty.ToolCallArgsForTranslate(x)
			if argsErr != nil {
				return nil, argsErr
			}
			var args map[string]any
			decoder := json.NewDecoder(strings.NewReader(argsJSON))
			decoder.UseNumber()
			if err := decoder.Decode(&args); err != nil {
				return nil, fmt.Errorf("%w: invalid tool call args JSON: %w", adapter.ErrMalformedArgs, err)
			}
			if args == nil {
				args = make(map[string]any)
			}
			functionPart := genai.NewPartFromFunctionCall(x.Name, args)
			functionPart.FunctionCall.ID = x.ID
			genParts = append(genParts, functionPart)
		case prompty.MediaPart:
			media, err := a.userContent([]prompty.ContentPart{x})
			if err != nil {
				return nil, err
			}
			genParts = append(genParts, media.Parts...)
		default:
			return nil, adapter.ErrUnsupportedContentType
		}
	}
	if len(genParts) == 0 {
		return genai.NewContentFromText("", genai.RoleModel), nil
	}
	return genai.NewContentFromParts(genParts, genai.RoleModel), nil
}

func (a *Adapter) toolResultContent(parts []prompty.ContentPart) (*genai.Content, error) {
	genParts := make([]*genai.Part, 0, len(parts))
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
		response, media, err := a.functionResultParts(tr.Content)
		if err != nil {
			return nil, err
		}
		key := wireOutput
		if tr.IsError {
			key = "error"
		}
		part := genai.NewPartFromFunctionResponseWithParts(tr.Name, map[string]any{key: response}, media)
		part.FunctionResponse.ID = tr.ToolCallID
		genParts = append(genParts, part)
	}
	if len(genParts) == 0 {
		return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	}
	return genai.NewContentFromParts(genParts, genai.RoleUser), nil
}

// SupportsToolNameCorrelation reports the name-preserving FunctionResponse mapping.
func (*Adapter) SupportsToolNameCorrelation() bool { return true }

// ParseResponse converts *genai.GenerateContentResponse into *prompty.Response.
func (a *Adapter) ParseResponse(resp *genai.GenerateContentResponse) (*prompty.Response, error) {
	desc := a.RuntimeDescriptor()
	return a.ParseResponseForProfile(
		resp,
		prompty.ProfileIdentity{
			ModelFamily: "",
			Provider:    desc.Name,
			Endpoint:    desc.Endpoint,
			Model:       desc.DefaultModel,
		},
	)
}

// ValidateProviderState rejects foreign, expired or modified signed parts.
func (a *Adapter) ValidateProviderState(exec *prompty.PromptExecution, destination prompty.ProfileIdentity) error {
	if err := prompty.ValidateContinuation(exec.Messages, destination, time.Now()); err != nil {
		return err
	}
	for _, message := range exec.Messages {
		for _, state := range message.ProviderState {
			if state.Envelope.Codec == geminiPartialPartsCodec &&
				(a.client == nil || a.client.ClientConfig().Backend != genai.BackendVertexAI) {
				return prompty.ErrProviderState
			}
		}
	}
	return validateGeminiPayloads(exec.Messages)
}

func modelProviderSettings(opts *prompty.ModelOptions) (map[string]any, error) {
	return prompty.ProviderSettingsMap(opts)
}

func geminiProviderSettingKeys() []string {
	return []string{
		"top_k",
		"presence_penalty",
		"frequency_penalty",
		"stop_sequences",
		"thinking",
		"thinking_budget",
		"thinking_level",
		"gemini_search_grounding",
		"stream_function_call_arguments",
	}
}

//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func applyGeminiProviderSettings(config *genai.GenerateContentConfig, settings map[string]any) (bool, error) {
	if len(settings) == 0 {
		return false, nil
	}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, geminiProviderSettingKeys()); err != nil {
		return false, err
	}
	if raw, ok := settings["top_k"]; ok {
		topK, err := cast.ToFloat32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("top_k", err)
		}
		if topK < 1 {
			return false, adapter.ProviderSettingError("top_k", fmt.Errorf("value %v must be >= 1", topK))
		}
		config.TopK = &topK
	}
	if raw, ok := settings["presence_penalty"]; ok {
		penalty, err := cast.ToFloat32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("presence_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("presence_penalty", penalty, -2, 2); err != nil {
			return false, err
		}
		config.PresencePenalty = &penalty
	}
	if raw, ok := settings["frequency_penalty"]; ok {
		penalty, err := cast.ToFloat32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("frequency_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("frequency_penalty", penalty, -2, 2); err != nil {
			return false, err
		}
		config.FrequencyPenalty = &penalty
	}
	if raw, ok := settings["stop_sequences"]; ok {
		stops, err := cast.ToStringSlice(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("stop_sequences", err)
		}
		config.StopSequences = stops
	}

	thinkingCfg := config.ThinkingConfig
	var hasThinkingSettings bool
	if raw, ok := settings["thinking"]; ok {
		includeThoughts, err := cast.ToBool(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("thinking", err)
		}
		if thinkingCfg == nil {
			thinkingCfg = &genai.ThinkingConfig{}
		}
		thinkingCfg.IncludeThoughts = includeThoughts
		hasThinkingSettings = true
	}
	if raw, ok := settings["thinking_budget"]; ok {
		if _, levelSet := settings["thinking_level"]; levelSet {
			return false, adapter.ProviderSettingError(
				"thinking_budget",
				errors.New("budget and level are separate modes"),
			)
		}
		budget, err := cast.ToInt32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("thinking_budget", err)
		}
		if err := adapter.ValidateInt32Min("thinking_budget", budget, -1); err != nil {
			return false, err
		}
		if thinkingCfg == nil {
			thinkingCfg = &genai.ThinkingConfig{}
		}
		thinkingCfg.ThinkingBudget = &budget
		hasThinkingSettings = true
	}
	if raw, ok := settings["thinking_level"]; ok {
		level, err := geminiThinkingLevel(raw)
		if err != nil {
			return false, err
		}
		if thinkingCfg == nil {
			thinkingCfg = &genai.ThinkingConfig{}
		}
		thinkingCfg.ThinkingLevel = level
		hasThinkingSettings = true
	}
	if hasThinkingSettings {
		config.ThinkingConfig = thinkingCfg
	}

	if raw, ok := settings["gemini_search_grounding"]; ok {
		enabled, err := cast.ToBool(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("gemini_search_grounding", err)
		}
		return enabled, nil
	}
	return false, nil
}

var _ adapter.ProviderAdapter[*Request, *genai.GenerateContentResponse] = (*Adapter)(nil)

func (a *Adapter) applyCacheResource(config *genai.GenerateContentConfig, exec *prompty.PromptExecution) error {
	if exec.CacheResource != nil {
		if err := a.ValidateCacheResource(*exec.CacheResource, a.destinationIdentity(exec)); err != nil {
			return err
		}
		if len(exec.Tools) != 0 || exec.ForcedTool != "" {
			return prompty.ErrCacheRequirement
		}
		for _, message := range exec.Messages {
			if message.Role == prompty.RoleSystem || message.Role == prompty.RoleDeveloper {
				return prompty.ErrCacheRequirement
			}
		}
		config.CachedContent = exec.CacheResource.ProviderID
	}

	return nil
}

func applyModelOptions(config *genai.GenerateContentConfig, exec *prompty.PromptExecution) {
	if exec.ModelOptions == nil {
		return
	}
	if exec.ModelOptions.Temperature != nil {
		t := float32(*exec.ModelOptions.Temperature)
		config.Temperature = &t
	}
	if exec.ModelOptions.MaxTokens != nil {
		if *exec.ModelOptions.MaxTokens > math.MaxInt32 {
			config.MaxOutputTokens = math.MaxInt32
		} else {
			//nolint:gosec // G115: value is <= math.MaxInt32 per branch above.
			config.MaxOutputTokens = int32(*exec.ModelOptions.MaxTokens)
		}
	}
	if exec.ModelOptions.TopP != nil {
		p := float32(*exec.ModelOptions.TopP)
		config.TopP = &p
	}
	if len(exec.ModelOptions.Stop) > 0 {
		config.StopSequences = exec.ModelOptions.Stop
	}
}

func (a *Adapter) translateMessages(input []prompty.ChatMessage) ([]string, []*genai.Content, error) {
	var systemParts []string
	var contents []*genai.Content
	for _, msg := range input {
		switch msg.Role {
		case prompty.RoleSystem, prompty.RoleDeveloper:
			text, err := prompty.StrictTextFromParts(msg.Content)
			if err != nil {
				return nil, nil, err
			}
			systemParts = append(systemParts, text)
		case prompty.RoleUser:
			c, err := a.userContent(msg.Content)
			if err != nil {
				return nil, nil, err
			}
			contents = append(contents, c)
		case prompty.RoleAssistant:
			c, err := a.assistantMessageContent(msg)
			if err != nil {
				return nil, nil, err
			}
			contents = append(contents, c)
		case prompty.RoleTool:
			c, err := a.toolResultContent(msg.Content)
			if err != nil {
				return nil, nil, err
			}
			contents = append(contents, c)
		default:
			return nil, nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
		}
	}

	return systemParts, contents, nil
}

func (a *Adapter) applyToolsAndResponseFormat(
	config *genai.GenerateContentConfig,
	exec *prompty.PromptExecution,
	wantGoogleSearch bool,
) error {
	// Request cache hints and caller-selected remote resources are distinct contracts.
	if a.nativeJSONSchema {
		if err := a.validateNativeExecution(exec); err != nil {
			return err
		}
	}
	if len(exec.Tools) > 0 {
		config.Tools = []*genai.Tool{{
			FunctionDeclarations: make([]*genai.FunctionDeclaration, 0, len(exec.Tools)),
		}}
		for _, t := range exec.Tools {
			config.Tools[0].FunctionDeclarations = append(
				config.Tools[0].FunctionDeclarations,
				&genai.FunctionDeclaration{
					Name:                 t.Name,
					Description:          t.Description,
					Parameters:           nil,
					ParametersJsonSchema: t.Parameters,
				},
			)
		}
	}
	if wantGoogleSearch {
		searchTool := &genai.Tool{
			//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
			GoogleSearch: &genai.GoogleSearch{},
		}
		if config.Tools == nil {
			config.Tools = []*genai.Tool{searchTool}
		} else {
			config.Tools = append(config.Tools, searchTool)
		}
	}
	if exec.ResponseFormat != nil && len(exec.ResponseFormat.Schema) > 0 {
		config.ResponseMIMEType = "application/json"
		schemaMap, err := prompty.SchemaMap(exec.ResponseFormat)
		if err != nil {
			return fmt.Errorf("response_format schema: %w", err)
		}
		if a.nativeJSONSchema {
			config.ResponseJsonSchema = schemaMap
			config.ResponseSchema = nil
			return nil
		}
		schema, err := mapToGenaiSchema(schemaMap)
		if err != nil {
			return fmt.Errorf("response_format schema: %w", err)
		}
		if schema != nil {
			config.ResponseSchema = schema
		}
	}

	return nil
}

func (a *Adapter) validateFunctionResultMedia(working *prompty.PromptExecution) error {
	if hasFunctionResultMedia(working.Messages) {
		checked := working.Clone()
		checked.Requirements.Capabilities = append(checked.Requirements.Capabilities, CapabilityFunctionResultMedia)
		if _, err := adapter.Preflight(a.RuntimeDescriptor(), checked, "", true); err != nil {
			return err
		}
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
	if cacheErr := adapter.RejectUnmappedCache(exec); cacheErr != nil {
		return cacheErr
	}

	return nil
}
