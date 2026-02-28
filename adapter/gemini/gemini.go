package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"

	"google.golang.org/genai"
)

// Request wraps Contents and Config for Gemini GenerateContent API.
type Request struct {
	Contents []*genai.Content
	Config   *genai.GenerateContentConfig
}

// Adapter implements adapter.ProviderAdapter for the Google Gemini (genai) API.
// Translate returns *gemini.Request; ParseResponse expects *genai.GenerateContentResponse.
type Adapter struct{}

// New returns an Adapter.
func New() *Adapter {
	return &Adapter{}
}

// Translate converts PromptExecution into *Request (Contents + Config).
func (a *Adapter) Translate(ctx context.Context, exec *prompty.PromptExecution) (any, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	return a.TranslateTyped(ctx, exec)
}

// TranslateTyped returns the concrete type so callers avoid type assertion.
func (a *Adapter) TranslateTyped(ctx context.Context, exec *prompty.PromptExecution) (*Request, error) {
	if exec == nil {
		return nil, adapter.ErrNilExecution
	}
	_ = ctx
	config := &genai.GenerateContentConfig{}
	// Model is set on the genai client, not in Config; we do not read "model" from ModelConfig.
	if exec.ModelConfig != nil {
		mp := adapter.ExtractModelConfig(exec.ModelConfig)
		if mp.Temperature != nil {
			t := float32(*mp.Temperature)
			config.Temperature = &t
		}
		if mp.MaxTokens != nil {
			if *mp.MaxTokens > math.MaxInt32 {
				config.MaxOutputTokens = math.MaxInt32
			} else {
				config.MaxOutputTokens = int32(*mp.MaxTokens)
			}
		}
		if mp.TopP != nil {
			p := float32(*mp.TopP)
			config.TopP = &p
		}
		if len(mp.Stop) > 0 {
			config.StopSequences = mp.Stop
		}
	}
	var systemParts []string
	var contents []*genai.Content
	for _, msg := range exec.Messages {
		switch msg.Role {
		case prompty.RoleSystem, prompty.RoleDeveloper:
			systemParts = append(systemParts, adapter.TextFromParts(msg.Content))
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
	if len(systemParts) > 0 {
		config.SystemInstruction = genai.NewContentFromText(strings.Join(systemParts, "\n\n"), genai.RoleUser)
	}
	if len(exec.Tools) > 0 {
		config.Tools = []*genai.Tool{{
			FunctionDeclarations: make([]*genai.FunctionDeclaration, 0, len(exec.Tools)),
		}}
		for _, t := range exec.Tools {
			config.Tools[0].FunctionDeclarations = append(config.Tools[0].FunctionDeclarations, &genai.FunctionDeclaration{
				Name:                 t.Name,
				Description:          t.Description,
				Parameters:           nil,
				ParametersJsonSchema: t.Parameters,
			})
		}
	}
	if exec.ResponseFormat != nil && len(exec.ResponseFormat.Schema) > 0 {
		config.ResponseMIMEType = "application/json"
		schema, err := mapToGenaiSchema(exec.ResponseFormat.Schema)
		if err != nil {
			return nil, fmt.Errorf("response_format schema: %w", err)
		}
		if schema != nil {
			config.ResponseSchema = schema
		}
	}
	return &Request{Contents: contents, Config: config}, nil
}

func (a *Adapter) userContent(parts []prompty.ContentPart) (*genai.Content, error) {
	var genParts []*genai.Part
	for _, p := range parts {
		switch x := p.(type) {
		case prompty.TextPart:
			genParts = append(genParts, genai.NewPartFromText(x.Text))
		case prompty.MediaPart:
			if x.MediaType != "image" {
				return nil, adapter.ErrUnsupportedContentType
			}
			switch {
			case len(x.Data) > 0:
				mime := x.MIMEType
				if mime == "" {
					mime = "image/png"
				}
				genParts = append(genParts, genai.NewPartFromBytes(x.Data, mime))
			case x.URL != "":
				mime := x.MIMEType
				if mime == "" {
					mime = "image/png"
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
			var args map[string]any
			if x.Args != "" {
				if err := json.Unmarshal([]byte(x.Args), &args); err != nil {
					return nil, fmt.Errorf("%w: invalid tool call args JSON: %w", adapter.ErrMalformedArgs, err)
				}
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
	for _, p := range parts {
		if tr, ok := p.(prompty.ToolResultPart); ok {
			part := genai.NewPartFromFunctionResponse(tr.Name, map[string]any{"result": tr.Content})
			return genai.NewContentFromParts([]*genai.Part{part}, genai.RoleUser), nil
		}
	}
	return nil, fmt.Errorf("%w: tool message missing ToolResultPart", adapter.ErrUnsupportedContentType)
}

// ParseResponse converts *genai.GenerateContentResponse into []prompty.ContentPart.
func (a *Adapter) ParseResponse(ctx context.Context, raw any) ([]prompty.ContentPart, error) {
	_ = ctx
	resp, ok := raw.(*genai.GenerateContentResponse)
	if !ok {
		return nil, adapter.ErrInvalidResponse
	}
	var out []prompty.ContentPart
	if text := resp.Text(); text != "" {
		out = append(out, prompty.TextPart{Text: text})
	}
	for _, fc := range resp.FunctionCalls() {
		var args string
		if len(fc.Args) > 0 {
			b, err := json.Marshal(fc.Args)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal function call args: %w", adapter.ErrMalformedArgs, err)
			}
			args = string(b)
		} else {
			args = "{}"
		}
		out = append(out, prompty.ToolCallPart{ID: fc.ID, Name: fc.Name, Args: args})
	}
	if len(out) == 0 {
		return nil, adapter.ErrEmptyResponse
	}
	return out, nil
}

// ParseStreamChunk parses a single Gemini stream chunk (*genai.GenerateContentResponse).
// Emits one ContentPart per chunk; client glues ArgsChunk for tool calls.
func (a *Adapter) ParseStreamChunk(ctx context.Context, rawChunk any) ([]prompty.ContentPart, error) {
	_ = ctx
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
				continue
			}
			argsChunk = string(b)
		}
		out = append(out, prompty.ToolCallPart{ID: fc.ID, Name: fc.Name, ArgsChunk: argsChunk})
	}
	return out, nil
}

var _ adapter.ProviderAdapter = (*Adapter)(nil)
