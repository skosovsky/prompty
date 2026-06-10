package compiled

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/skosovsky/prompty"
)

const formatVersion = 2

type promptWire struct {
	FormatVersion  int           `json:"format_version"`
	ManifestID     string        `json:"manifest_id"`
	ManifestDigest string        `json:"manifest_digest"`
	DigestSource   DigestSource  `json:"digest_source"`
	Execution      executionWire `json:"execution"`
}

type executionWire struct {
	Messages       []messageWire             `json:"messages"`
	Tools          []prompty.ToolDefinition  `json:"tools,omitempty"`
	RequiredTools  []string                  `json:"required_tools,omitempty"`
	ForcedTool     string                    `json:"forced_tool,omitempty"`
	ModelOptions   *prompty.ModelOptions     `json:"model_options,omitempty"`
	Metadata       prompty.PromptMetadata    `json:"metadata"`
	ResponseFormat *prompty.SchemaDefinition `json:"response_format,omitempty"`
}

type messageWire struct {
	Role        prompty.Role               `json:"role"`
	Content     []contentPartWire          `json:"content"`
	CachePolicy *prompty.CachePolicy       `json:"cache_policy,omitempty"`
	Provenance  *prompty.MessageProvenance `json:"provenance,omitempty"`
	Metadata    map[string]any             `json:"metadata,omitempty"`
	LayerKind   prompty.LayerKind          `json:"layer_kind,omitempty"`
}

type contentPartWire struct {
	Type        string               `json:"type"`
	Text        string               `json:"text,omitempty"`
	MediaType   string               `json:"media_type,omitempty"`
	MIMEType    string               `json:"mime_type,omitempty"`
	URL         string               `json:"url,omitempty"`
	Data        string               `json:"data,omitempty"`
	ID          string               `json:"id,omitempty"`
	Name        string               `json:"name,omitempty"`
	Args        string               `json:"args,omitempty"`
	ArgsChunk   string               `json:"args_chunk,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	IsError     bool                 `json:"is_error,omitempty"`
	Nested      []contentPartWire    `json:"nested,omitempty"`
	CachePolicy *prompty.CachePolicy `json:"cache_policy,omitempty"`
}

//nolint:exhaustruct,gocognit // wire encoding uses partial field sets per content part type
func encodeContentPart(p prompty.ContentPart) (contentPartWire, error) {
	switch x := p.(type) {
	case prompty.TextPart:
		return contentPartWire{Type: "text", Text: x.Text, CachePolicy: x.CachePolicy}, nil
	case *prompty.TextPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil text content part")
		}
		return encodeContentPart(*x)
	case prompty.MediaPart:
		w := contentPartWire{
			Type: "media", MediaType: x.MediaType, MIMEType: x.MIMEType, URL: x.URL,
			CachePolicy: x.CachePolicy,
		}
		if len(x.Data) > 0 {
			w.Data = base64.StdEncoding.EncodeToString(x.Data)
		}
		return w, nil
	case *prompty.MediaPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil media content part")
		}
		return encodeContentPart(*x)
	case prompty.ReasoningPart:
		return contentPartWire{Type: "reasoning", Text: x.Text, CachePolicy: x.CachePolicy}, nil
	case *prompty.ReasoningPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil reasoning content part")
		}
		return encodeContentPart(*x)
	case prompty.ToolCallPart:
		return contentPartWire{
			Type: "tool_call", ID: x.ID, Name: x.Name, Args: x.Args, ArgsChunk: x.ArgsChunk,
			CachePolicy: x.CachePolicy,
		}, nil
	case *prompty.ToolCallPart:
		if x == nil {
			return contentPartWire{}, errors.New("compiled prompt: nil tool_call content part")
		}
		return encodeContentPart(*x)
	case prompty.ToolResultPart:
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
			IsError: x.IsError, CachePolicy: x.CachePolicy,
		}, nil
	case *prompty.ToolResultPart:
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

func decodeContentPart(w contentPartWire) (prompty.ContentPart, error) {
	switch w.Type {
	case "text":
		return prompty.TextPart{Text: w.Text, CachePolicy: w.CachePolicy}, nil
	case "media":
		var data []byte
		if w.Data != "" {
			decoded, err := base64.StdEncoding.DecodeString(w.Data)
			if err != nil {
				return nil, fmt.Errorf("compiled prompt: invalid base64 media data: %w", err)
			}
			data = decoded
		}
		return prompty.MediaPart{
			MediaType: w.MediaType, MIMEType: w.MIMEType, URL: w.URL, Data: data,
			CachePolicy: w.CachePolicy,
		}, nil
	case "reasoning":
		return prompty.ReasoningPart{Text: w.Text, CachePolicy: w.CachePolicy}, nil
	case "tool_call":
		return prompty.ToolCallPart{
			ID: w.ID, Name: w.Name, Args: w.Args, ArgsChunk: w.ArgsChunk, CachePolicy: w.CachePolicy,
		}, nil
	case "tool_result":
		nested := make([]prompty.ContentPart, len(w.Nested))
		for i, c := range w.Nested {
			part, err := decodeContentPart(c)
			if err != nil {
				return nil, err
			}
			nested[i] = part
		}
		return prompty.ToolResultPart{
			ToolCallID: w.ToolCallID, Name: w.Name, Content: nested, IsError: w.IsError,
			CachePolicy: w.CachePolicy,
		}, nil
	default:
		if w.Type == "" {
			return nil, errors.New("compiled prompt: content part type is required")
		}
		return nil, fmt.Errorf("compiled prompt: unknown content part type %q", w.Type)
	}
}

func cloneProvenance(p *prompty.MessageProvenance) *prompty.MessageProvenance {
	if p == nil {
		return nil
	}
	out := *p
	return &out
}

func promptToWire(c *Prompt) (promptWire, error) {
	msgs := make([]messageWire, len(c.execution.Messages))
	for i, m := range c.execution.Messages {
		parts := make([]contentPartWire, len(m.Content))
		for j, p := range m.Content {
			wire, err := encodeContentPart(p)
			if err != nil {
				return promptWire{}, err
			}
			parts[j] = wire
		}
		msgMeta, err := prompty.JSONDocumentAsMap(m.Metadata)
		if err != nil {
			return promptWire{}, err
		}
		meta := maps.Clone(msgMeta)
		msgs[i] = messageWire{
			Role: m.Role, Content: parts, CachePolicy: m.CachePolicy,
			Provenance: cloneProvenance(m.Provenance), Metadata: meta, LayerKind: m.LayerKind,
		}
	}
	return promptWire{
		FormatVersion:  formatVersion,
		ManifestID:     c.manifestID,
		ManifestDigest: c.manifestDigest,
		DigestSource:   c.digestSource,
		Execution: executionWire{
			Messages: msgs, Tools: append([]prompty.ToolDefinition(nil), c.execution.Tools...),
			RequiredTools: append([]string(nil), c.execution.RequiredTools...),
			ForcedTool:    c.execution.ForcedTool, ModelOptions: c.execution.ModelOptions,
			Metadata: c.execution.Metadata, ResponseFormat: c.execution.ResponseFormat,
		},
	}, nil
}

func validateWire(w promptWire) error {
	if w.FormatVersion != formatVersion {
		return fmt.Errorf(
			"compiled prompt: unsupported format_version %d (supported: %d)",
			w.FormatVersion, formatVersion,
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

func wireToPrompt(w promptWire) (*Prompt, error) {
	if err := validateWire(w); err != nil {
		return nil, err
	}
	msgs := make([]prompty.ChatMessage, len(w.Execution.Messages))
	for i, m := range w.Execution.Messages {
		parts := make([]prompty.ContentPart, len(m.Content))
		for j, p := range m.Content {
			part, err := decodeContentPart(p)
			if err != nil {
				return nil, err
			}
			parts[j] = part
		}
		msgMeta, err := prompty.MapToJSONDocument(m.Metadata)
		if err != nil {
			return nil, err
		}
		msgs[i] = prompty.ChatMessage{
			Role: m.Role, Content: parts, CachePolicy: m.CachePolicy,
			Provenance: cloneProvenance(m.Provenance), Metadata: msgMeta, LayerKind: m.LayerKind,
		}
	}
	return &Prompt{
		manifestID: w.ManifestID, manifestDigest: w.ManifestDigest, digestSource: w.DigestSource,
		execution: prompty.PromptExecution{
			Messages: msgs, Tools: w.Execution.Tools, RequiredTools: w.Execution.RequiredTools,
			ForcedTool: w.Execution.ForcedTool, ModelOptions: w.Execution.ModelOptions,
			Metadata: w.Execution.Metadata, ResponseFormat: w.Execution.ResponseFormat,
		},
	}, nil
}

func marshalJSON(c *Prompt) ([]byte, error) {
	wire, err := promptToWire(c)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wire)
}

func unmarshalJSON(data []byte) (*Prompt, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w promptWire
	if err := dec.Decode(&w); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("compiled prompt: trailing JSON after document")
	}
	return wireToPrompt(w)
}
