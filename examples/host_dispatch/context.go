package hostdispatch

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/contexty"

	"github.com/skosovsky/prompty"
)

const imageKind = "image"

// FromContext implements a narrow semantic subset. Richer fields require the
// separate opaque storage profile; unknown data is never silently dropped.
func FromContext(messages []contexty.Message) ([]prompty.ChatMessage, error) {
	out := make([]prompty.ChatMessage, len(messages))
	for i, message := range messages {
		if !supportedRole(message.Role) || !utf8.ValidString(message.ID) || message.Actor != nil ||
			message.Origin != nil ||
			message.LLMCache != nil ||
			message.Provenance != nil ||
			len(message.SourceRefs) != 0 ||
			!contextAnnotationsEmpty(message.Annotations) {
			return nil, ErrProjection
		}
		rawCalls, err := contextRawArguments(message)
		if err != nil {
			return nil, err
		}
		out[i].Role = prompty.Role(message.Role)
		for partIndex, part := range message.Parts {
			mapped, err := fromContextPart(part)
			if err != nil {
				return nil, err
			}
			if original, exists := rawCalls[partIndex]; exists {
				call, ok := mapped.(prompty.ToolCallPart)
				if !ok {
					return nil, ErrProjection
				}
				call.Args = original.Args
				mapped = call
			}
			out[i].Content = append(out[i].Content, mapped)
		}
	}
	return out, nil
}

func fromContextPart(part contexty.ContentPart) (prompty.ContentPart, error) {
	switch value := part.(type) {
	case contexty.TextPart:
		if !utf8.ValidString(value.Text) {
			return nil, ErrProjection
		}
		return prompty.TextPart{Text: value.Text}, nil
	case contexty.ImagePart:
		if value.Detail != "" || !utf8.ValidString(value.URL) {
			return nil, ErrProjection
		}
		return prompty.MediaPart{MediaType: imageKind, URL: value.URL}, nil
	case contexty.MediaPart:
		mediaType, _, err := mime.ParseMediaType(value.MIMEType)
		if err != nil || !strings.HasPrefix(mediaType, "image/") {
			return nil, ErrProjection
		}
		return prompty.MediaPart{MediaType: imageKind, MIMEType: value.MIMEType, Data: bytes.Clone(value.Data)}, nil
	case contexty.ToolCallPart:
		return fromContextCall(value)
	case contexty.ToolResultPart:
		return fromContextResult(value)
	default:
		return nil, ErrProjection
	}
}

func simplePayload(payload contexty.ToolPayload) bool {
	return len(payload.Binary) == 0 && payload.Error == nil && payload.Progress == nil && payload.Control == nil &&
		(len(payload.Data) == 0 || payload.Text == "")
}

func contextAnnotationsEmpty(annotations contexty.Annotations) bool {
	// Compare via the semantic zero-value helper in this caller profile.
	return annotations == (contexty.Annotations{})
}

// ToContext maps owned native messages into the same semantic subset.
// Message IDs are supplied by host and preserved separately on reverse mapping.
func ToContext(messages []prompty.ChatMessage, ids []string) ([]contexty.Message, error) {
	if len(messages) != len(ids) {
		return nil, ErrProjection
	}
	out := make([]contexty.Message, len(messages))
	for i, message := range messages {
		if ids[i] == "" || !utf8.ValidString(ids[i]) || len(message.ProviderState) != 0 ||
			len(message.Annotations) != 0 ||
			len(message.MessageAnnotations) != 0 ||
			message.ContinuationUnavailable ||
			message.Provenance != nil ||
			message.CachePolicy != nil ||
			len(message.Metadata) != 0 ||
			message.LayerKind != "" {
			return nil, ErrProjection
		}
		out[i] = contexty.Message{ID: ids[i], Role: contexty.Role(message.Role)}
		if !supportedRole(out[i].Role) {
			return nil, ErrProjection
		}
		var raw rawArguments
		for partIndex, part := range message.Content {
			mapped, err := toContextPart(part)
			if err != nil {
				return nil, err
			}
			if call, ok := part.(prompty.ToolCallPart); ok {
				raw.Calls = append(raw.Calls, rawCall{Index: partIndex, ID: call.ID, Name: call.Name, Args: call.Args})
			}
			out[i].Parts = append(out[i].Parts, mapped)
		}
		if len(raw.Calls) != 0 {
			out[i].Extensions = []contexty.Extension{raw}
		}
	}
	return out, nil
}

func toContextPart(part prompty.ContentPart) (contexty.ContentPart, error) {
	switch value := part.(type) {
	case prompty.TextPart:
		if value.CachePolicy != nil || !utf8.ValidString(value.Text) {
			return nil, ErrProjection
		}
		return contexty.TextPart{Text: value.Text}, nil
	case prompty.MediaPart:
		return toContextImage(value)
	case prompty.ToolCallPart:
		return toContextCall(value)
	case prompty.ToolResultPart:
		return toContextResult(value)
	default:
		return nil, ErrProjection
	}
}

func supportedRole(role contexty.Role) bool {
	switch role {
	case contexty.RoleSystem, contexty.Role("developer"), contexty.RoleUser, contexty.RoleAssistant, contexty.RoleTool:
		return true
	default:
		return false
	}
}

func toContextImage(value prompty.MediaPart) (contexty.ContentPart, error) {
	if value.MediaType != imageKind || value.CachePolicy != nil || !utf8.ValidString(value.URL) {
		return nil, ErrProjection
	}
	if value.URL != "" {
		if len(value.Data) != 0 || value.MIMEType != "" {
			return nil, ErrProjection
		}
		return contexty.ImagePart{URL: value.URL}, nil
	}
	mediaType, _, err := mime.ParseMediaType(value.MIMEType)
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return nil, ErrProjection
	}
	return contexty.MediaPart{MIMEType: value.MIMEType, Data: bytes.Clone(value.Data)}, nil
}

func fromContextCall(value contexty.ToolCallPart) (prompty.ContentPart, error) {
	if value.ArgumentsBlob != nil || !simplePayload(value.Arguments) || len(value.Arguments.Data) == 0 ||
		value.Arguments.MIMEType != "" || !utf8.Valid(value.Arguments.Data) || !utf8.ValidString(value.ID) || !utf8.ValidString(value.Name) {
		return nil, ErrProjection
	}
	if _, err := prompty.JSONDocumentAsMap(value.Arguments.Data); err != nil {
		return nil, ErrProjection
	}
	return prompty.ToolCallPart{ID: value.ID, Name: value.Name, Args: string(value.Arguments.Data)}, nil
}

func fromContextResult(value contexty.ToolResultPart) (prompty.ContentPart, error) {
	if !utf8.ValidString(value.Payload.Text) || !utf8.ValidString(value.ToolCallID) ||
		!utf8.ValidString(value.Name) ||
		!simplePayload(value.Payload) ||
		len(value.Payload.Data) != 0 ||
		(value.Payload.MIMEType != "" && value.Payload.MIMEType != "text/plain") {
		return nil, ErrProjection
	}
	return prompty.ToolResultPart{ToolCallID: value.ToolCallID, Name: value.Name, IsError: value.IsError,
		Content: []prompty.ContentPart{prompty.TextPart{Text: value.Payload.Text}}}, nil
}

func toContextCall(value prompty.ToolCallPart) (contexty.ContentPart, error) {
	if value.ArgsChunk != "" || value.CachePolicy != nil || !utf8.ValidString(value.Args) ||
		!json.Valid([]byte(value.Args)) ||
		!utf8.ValidString(value.ID) ||
		!utf8.ValidString(value.Name) {
		return nil, ErrProjection
	}
	if _, err := prompty.JSONDocumentAsMap(prompty.JSONDocument(value.Args)); err != nil {
		return nil, ErrProjection
	}
	return contexty.ToolCallPart{ID: value.ID, Name: value.Name, Arguments: contexty.JSONPayload(value.Args)}, nil
}

func toContextResult(value prompty.ToolResultPart) (contexty.ContentPart, error) {
	if value.CachePolicy != nil || !utf8.ValidString(value.ToolCallID) || !utf8.ValidString(value.Name) {
		return nil, ErrProjection
	}
	var text strings.Builder
	for _, content := range value.Content {
		p, ok := content.(prompty.TextPart)
		if !ok || p.CachePolicy != nil || !utf8.ValidString(p.Text) {
			return nil, ErrProjection
		}
		text.WriteString(p.Text)
	}
	// Multiple native text parts have distinct boundaries; do not flatten them.
	if len(value.Content) != 1 {
		return nil, ErrProjection
	}
	return contexty.ToolResultPart{
		ToolCallID: value.ToolCallID,
		Name:       value.Name,
		IsError:    value.IsError,
		Payload:    contexty.TextPayload(text.String()),
	}, nil
}
