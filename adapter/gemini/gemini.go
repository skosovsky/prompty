package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

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
	defaultModel string
	client       *genai.Client
}

// Option configures an Adapter (e.g. WithModel, WithClient).
type Option func(*Adapter)

// WithModel sets the default model used when exec.ModelOptions does not contain Model.
func WithModel(m string) Option {
	return func(a *Adapter) { a.defaultModel = m }
}

// WithClient injects the genai client for Execute. Required for Execute/Invoker flow.
func WithClient(c *genai.Client) Option {
	return func(a *Adapter) { a.client = c }
}

// New returns an Adapter with default model "gemini-2.0-flash".
func New(opts ...Option) *Adapter {
	a := &Adapter{defaultModel: "gemini-2.0-flash"}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Translate converts PromptExecution into *Request (Contents + Config).
func (a *Adapter) Translate(exec *prompty.PromptExecution) (*Request, error) {
	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, err
	}
	working := adapter.PrepareTranslateExecution(exec)
	config := &genai.GenerateContentConfig{}
	// Model is set on the genai request, not inside Config.
	if exec.ModelOptions != nil {
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
	providerSettings, err := modelProviderSettings(exec.ModelOptions)
	if err != nil {
		return nil, err
	}
	wantGoogleSearch, err := applyGeminiProviderSettings(config, providerSettings)
	if err != nil {
		return nil, err
	}
	var systemParts []string
	var contents []*genai.Content
	for _, msg := range working.Messages {
		switch msg.Role {
		case prompty.RoleSystem, prompty.RoleDeveloper:
			text, err := prompty.StrictTextFromParts(msg.Content)
			if err != nil {
				return nil, err
			}
			systemParts = append(systemParts, text)
		case prompty.RoleUser:
			c, err := a.userContent(msg.Content)
			if err != nil {
				return nil, err
			}
			contents = append(contents, c)
		case prompty.RoleAssistant:
			c, err := a.assistantContent(msg.Content)
			if err != nil {
				return nil, err
			}
			contents = append(contents, c)
		case prompty.RoleTool:
			c, err := a.toolResultContent(msg.Content)
			if err != nil {
				return nil, err
			}
			contents = append(contents, c)
		default:
			return nil, fmt.Errorf("%w: %q", adapter.ErrUnsupportedRole, msg.Role)
		}
	}
	if exec.ForcedTool != "" {
		systemParts = append(
			systemParts,
			fmt.Sprintf("You must call tool %q for the next tool-use step.", exec.ForcedTool),
		)
	}
	if len(systemParts) > 0 {
		config.SystemInstruction = genai.NewContentFromText(strings.Join(systemParts, "\n\n"), genai.RoleUser)
	}
	// CachePolicy is ignored: Context Caching requires out-of-band orchestration (Context Caching API).
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
		searchTool := &genai.Tool{GoogleSearch: &genai.GoogleSearch{}}
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
			return nil, fmt.Errorf("response_format schema: %w", err)
		}
		schema, err := mapToGenaiSchema(schemaMap)
		if err != nil {
			return nil, fmt.Errorf("response_format schema: %w", err)
		}
		if schema != nil {
			config.ResponseSchema = schema
		}
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
	return &Request{Model: model, Contents: contents, Config: config}, nil
}

// Execute performs the API call. Requires WithClient.
func (a *Adapter) Execute(ctx context.Context, req *Request) (*genai.GenerateContentResponse, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	return a.client.Models.GenerateContent(ctx, req.Model, req.Contents, req.Config)
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
		case prompty.ToolCallPart:
			argsJSON, argsErr := prompty.ToolCallArgsForTranslate(x)
			if argsErr != nil {
				return nil, argsErr
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return nil, fmt.Errorf("%w: invalid tool call args JSON: %w", adapter.ErrMalformedArgs, err)
			}
			if args == nil {
				args = make(map[string]any)
			}
			genParts = append(genParts, genai.NewPartFromFunctionCall(x.Name, args))
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
		// Fail-fast on MediaPart: FunctionResponse expects map[string]any (JSON), no native image support
		for _, cp := range tr.Content {
			if _, ok := cp.(prompty.MediaPart); ok {
				return nil, adapter.ErrUnsupportedContentType
			}
		}
		text, err := prompty.StrictTextFromParts(tr.Content)
		if err != nil {
			return nil, err
		}
		genParts = append(genParts, genai.NewPartFromFunctionResponse(tr.Name, map[string]any{"result": text}))
	}
	if len(genParts) == 0 {
		return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
	}
	return genai.NewContentFromParts(genParts, genai.RoleUser), nil
}

// ParseResponse converts *genai.GenerateContentResponse into *prompty.Response.
func (a *Adapter) ParseResponse(resp *genai.GenerateContentResponse) (*prompty.Response, error) {
	if resp == nil {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	if text := resp.Text(); text != "" {
		out = append(out, prompty.TextPart{Text: text})
	}
	for _, fc := range resp.FunctionCalls() {
		if len(fc.Args) == 0 {
			return nil, fmt.Errorf(
				"%w: empty function call args for %q",
				adapter.ErrMalformedArgs,
				fc.Name,
			)
		}
		b, err := json.Marshal(fc.Args)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to marshal function call args: %w", adapter.ErrMalformedArgs, err)
		}
		out = append(out, prompty.ToolCallPart{ID: fc.ID, Name: fc.Name, Args: string(b)})
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	return prompty.NewResponse(out), nil
}

// ParseStreamChunk parses a single Gemini stream chunk (*genai.GenerateContentResponse).
// Emits one ContentPart per chunk; client glues ArgsChunk for tool calls.
func (a *Adapter) ParseStreamChunk(rawChunk any) ([]prompty.ContentPart, error) {
	chunk, ok := rawChunk.(*genai.GenerateContentResponse)
	if !ok {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	if text := chunk.Text(); text != "" {
		out = append(out, prompty.TextPart{Text: text})
	}
	for _, fc := range chunk.FunctionCalls() {
		var argsChunk string
		if len(fc.Args) > 0 {
			b, err := json.Marshal(fc.Args)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal function call args: %w", adapter.ErrMalformedArgs, err)
			}
			argsChunk = string(b)
		}
		out = append(out, prompty.ToolCallPart{ID: fc.ID, Name: fc.Name, ArgsChunk: argsChunk})
	}
	return out, nil
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
		"gemini_search_grounding",
	}
}

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
		budget, err := cast.ToInt32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("thinking_budget", err)
		}
		if err := adapter.ValidateInt32Min("thinking_budget", budget, 0); err != nil {
			return false, err
		}
		if thinkingCfg == nil {
			thinkingCfg = &genai.ThinkingConfig{}
		}
		thinkingCfg.ThinkingBudget = &budget
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
