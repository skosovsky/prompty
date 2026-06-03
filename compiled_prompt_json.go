package prompty

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const compiledPromptFormatVersion = 1

type compiledPromptWire struct {
	FormatVersion  int           `json:"format_version"`
	ManifestID     string        `json:"manifest_id"`
	ManifestDigest string        `json:"manifest_digest"`
	DigestSource   DigestSource  `json:"digest_source"`
	Execution      executionWire `json:"execution"`
}

type executionWire struct {
	Messages       []messageWire     `json:"messages"`
	Tools          []ToolDefinition  `json:"tools,omitempty"`
	RequiredTools  []string          `json:"required_tools,omitempty"`
	ForcedTool     string            `json:"forced_tool,omitempty"`
	ModelOptions   *ModelOptions     `json:"model_options,omitempty"`
	Metadata       PromptMetadata    `json:"metadata"`
	ResponseFormat *SchemaDefinition `json:"response_format,omitempty"`
}

type messageWire struct {
	Role         Role              `json:"role"`
	Content      []contentPartWire `json:"content"`
	CacheControl *CacheControl     `json:"cache_control,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
	LayerID      string            `json:"layer_id,omitempty"`
	LayerKind    LayerKind         `json:"layer_kind,omitempty"`
	LayerRef     LayerRef          `json:"layer_ref,omitzero"`
	ManifestID   string            `json:"manifest_id,omitempty"`
}

type contentPartWire struct {
	Type         string            `json:"type"`
	Text         string            `json:"text,omitempty"`
	MediaType    string            `json:"media_type,omitempty"`
	MIMEType     string            `json:"mime_type,omitempty"`
	URL          string            `json:"url,omitempty"`
	Data         string            `json:"data,omitempty"` // base64
	ID           string            `json:"id,omitempty"`
	Name         string            `json:"name,omitempty"`
	Args         string            `json:"args,omitempty"`
	ArgsChunk    string            `json:"args_chunk,omitempty"`
	ToolCallID   string            `json:"tool_call_id,omitempty"`
	IsError      bool              `json:"is_error,omitempty"`
	Nested       []contentPartWire `json:"nested,omitempty"`
	CacheControl *CacheControl     `json:"cache_control,omitempty"`
}

//nolint:exhaustruct,gocognit // wire encoding uses partial field sets per content part type
func encodeContentPart(p ContentPart) (contentPartWire, error) {
	switch x := p.(type) {
	case TextPart:
		return contentPartWire{Type: "text", Text: x.Text, CacheControl: x.CacheControl}, nil
	case *TextPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil text content part")
		}
		return encodeContentPart(*x)
	case MediaPart:
		w := contentPartWire{
			Type: "media", MediaType: x.MediaType, MIMEType: x.MIMEType, URL: x.URL,
			CacheControl: x.CacheControl,
		}
		if len(x.Data) > 0 {
			w.Data = base64.StdEncoding.EncodeToString(x.Data)
		}
		return w, nil
	case *MediaPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil media content part")
		}
		return encodeContentPart(*x)
	case ReasoningPart:
		return contentPartWire{Type: "reasoning", Text: x.Text, CacheControl: x.CacheControl}, nil
	case *ReasoningPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil reasoning content part")
		}
		return encodeContentPart(*x)
	case ToolCallPart:
		return contentPartWire{
			Type: "tool_call", ID: x.ID, Name: x.Name, Args: x.Args, ArgsChunk: x.ArgsChunk,
			CacheControl: x.CacheControl,
		}, nil
	case *ToolCallPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil tool_call content part")
		}
		return encodeContentPart(*x)
	case ToolResultPart:
		nested := make([]contentPartWire, len(x.Content))
		for i, c := range x.Content {
			wire, err := encodeContentPart(c)
			if err != nil {
				return contentPartWire{}, err
			}
			nested[i] = wire
		}
		return contentPartWire{
			Type: "tool_result", ToolCallID: x.ToolCallID, Name: x.Name, Nested: nested,
			IsError: x.IsError, CacheControl: x.CacheControl,
		}, nil
	case *ToolResultPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil tool_result content part")
		}
		return encodeContentPart(*x)
	default:
		if p == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil content part")
		}
		return contentPartWire{}, fmt.Errorf("compiled prompt: unsupported content part type %T", p)
	}
}

func decodeContentPart(w contentPartWire) (ContentPart, error) {
	switch w.Type {
	case "text":
		return TextPart{Text: w.Text, CacheControl: w.CacheControl}, nil
	case "media":
		var data []byte
		if w.Data != "" {
			decoded, err := base64.StdEncoding.DecodeString(w.Data)
			if err != nil {
				return nil, fmt.Errorf("compiled prompt: invalid base64 media data: %w", err)
			}
			data = decoded
		}
		return MediaPart{
			MediaType: w.MediaType, MIMEType: w.MIMEType, URL: w.URL, Data: data,
			CacheControl: w.CacheControl,
		}, nil
	case "reasoning":
		return ReasoningPart{Text: w.Text, CacheControl: w.CacheControl}, nil
	case "tool_call":
		return ToolCallPart{
			ID: w.ID, Name: w.Name, Args: w.Args, ArgsChunk: w.ArgsChunk, CacheControl: w.CacheControl,
		}, nil
	case "tool_result":
		nested := make([]ContentPart, len(w.Nested))
		for i, c := range w.Nested {
			part, err := decodeContentPart(c)
			if err != nil {
				return nil, err
			}
			nested[i] = part
		}
		return ToolResultPart{
			ToolCallID: w.ToolCallID, Name: w.Name, Content: nested, IsError: w.IsError,
			CacheControl: w.CacheControl,
		}, nil
	default:
		if w.Type == "" {
			return nil, errors.New("compiled prompt: content part type is required")
		}
		return nil, fmt.Errorf("compiled prompt: unknown content part type %q", w.Type)
	}
}

func compiledPromptToWire(c *CompiledPrompt) (compiledPromptWire, error) {
	msgs := make([]messageWire, len(c.execution.Messages))
	for i, m := range c.execution.Messages {
		parts := make([]contentPartWire, len(m.Content))
		for j, p := range m.Content {
			wire, err := encodeContentPart(p)
			if err != nil {
				return compiledPromptWire{}, err
			}
			parts[j] = wire
		}
		msgMeta, err := JSONDocumentAsMap(m.Metadata)
		if err != nil {
			return compiledPromptWire{}, err
		}
		msgs[i] = messageWire{
			Role: m.Role, Content: parts, CacheControl: m.CacheControl, Metadata: cloneMapAny(msgMeta),
			LayerID: m.LayerID, LayerKind: m.LayerKind, LayerRef: m.LayerRef, ManifestID: m.ManifestID,
		}
	}
	return compiledPromptWire{
		FormatVersion:  compiledPromptFormatVersion,
		ManifestID:     c.manifestID,
		ManifestDigest: c.manifestDigest,
		DigestSource:   c.digestSource,
		Execution: executionWire{
			Messages: msgs, Tools: append([]ToolDefinition(nil), c.execution.Tools...),
			RequiredTools: append([]string(nil), c.execution.RequiredTools...),
			ForcedTool:    c.execution.ForcedTool, ModelOptions: c.execution.ModelOptions,
			Metadata: c.execution.Metadata, ResponseFormat: c.execution.ResponseFormat,
		},
	}, nil
}

func validateCompiledPromptWire(w compiledPromptWire) error {
	if w.FormatVersion != compiledPromptFormatVersion {
		return fmt.Errorf(
			"compiled prompt: unsupported format_version %d (supported: %d)",
			w.FormatVersion, compiledPromptFormatVersion,
		)
	}
	if w.ManifestDigest == "" {
		return errors.New("compiled prompt: manifest_digest is required")
	}
	switch w.DigestSource {
	case DigestSourceManifestBytes, DigestSourceCanonicalSnapshot:
	default:
		return fmt.Errorf("compiled prompt: invalid digest_source %q", w.DigestSource)
	}
	return nil
}

func wireToCompiledPrompt(w compiledPromptWire) (*CompiledPrompt, error) {
	if err := validateCompiledPromptWire(w); err != nil {
		return nil, err
	}
	msgs := make([]ChatMessage, len(w.Execution.Messages))
	for i, m := range w.Execution.Messages {
		parts := make([]ContentPart, len(m.Content))
		for j, p := range m.Content {
			part, err := decodeContentPart(p)
			if err != nil {
				return nil, err
			}
			parts[j] = part
		}
		msgMeta, err := MapToJSONDocument(m.Metadata)
		if err != nil {
			return nil, err
		}
		msgs[i] = ChatMessage{
			Role: m.Role, Content: parts, CacheControl: m.CacheControl, Metadata: msgMeta,
			LayerID: m.LayerID, LayerKind: m.LayerKind, LayerRef: m.LayerRef, ManifestID: m.ManifestID,
		}
	}
	return &CompiledPrompt{
		manifestID: w.ManifestID, manifestDigest: w.ManifestDigest, digestSource: w.DigestSource,
		execution: PromptExecution{
			Messages: msgs, Tools: w.Execution.Tools, RequiredTools: w.Execution.RequiredTools,
			ForcedTool: w.Execution.ForcedTool, ModelOptions: w.Execution.ModelOptions,
			Metadata: w.Execution.Metadata, ResponseFormat: w.Execution.ResponseFormat,
		},
	}, nil
}

func marshalCompiledPromptJSON(c *CompiledPrompt) ([]byte, error) {
	wire, err := compiledPromptToWire(c)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wire)
}

func unmarshalCompiledPromptJSON(data []byte) (*CompiledPrompt, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w compiledPromptWire
	if err := dec.Decode(&w); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("compiled prompt: trailing JSON after document")
	}
	return wireToCompiledPrompt(w)
}
