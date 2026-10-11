package gemini

import (
	"bytes"
	"fmt"
	"net/url"
	"slices"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// CapabilityFunctionResultMedia is caller-owned endpoint/model knowledge for
// native text/image function results. It does not declare general media support.
const CapabilityFunctionResultMedia = "gemini.function_result_media"

func hasFunctionResultMedia(messages []prompty.ChatMessage) bool {
	for _, message := range messages {
		for _, part := range message.Content {
			var content []prompty.ContentPart
			switch value := part.(type) {
			case prompty.ToolResultPart:
				content = value.Content
			case *prompty.ToolResultPart:
				if value != nil {
					content = value.Content
				}
			}
			for _, child := range content {
				switch child.(type) {
				case prompty.MediaPart, *prompty.MediaPart:
					return true
				}
			}
		}
	}
	return false
}

func (a *Adapter) functionResultParts(content []prompty.ContentPart) (any, []*genai.FunctionResponsePart, error) {
	var media []*genai.FunctionResponsePart
	ordered := make([]any, 0, len(content))
	for index, part := range content {
		switch value := part.(type) {
		case prompty.TextPart:
			ordered = append(ordered, value.Text)
		case *prompty.TextPart:
			if value == nil {
				return nil, nil, adapter.ErrUnsupportedContentType
			}
			ordered = append(ordered, value.Text)
		case prompty.MediaPart:
			image, err := a.functionResultImage(value, index)
			if err != nil {
				return nil, nil, err
			}
			media = append(media, image)
			ordered = append(ordered, map[string]any{nativeReferenceKeyword: fmt.Sprintf("prompty_part_%d", index)})
		case *prompty.MediaPart:
			if value == nil {
				return nil, nil, adapter.ErrUnsupportedContentType
			}
			image, err := a.functionResultImage(*value, index)
			if err != nil {
				return nil, nil, err
			}
			media = append(media, image)
			ordered = append(ordered, map[string]any{nativeReferenceKeyword: fmt.Sprintf("prompty_part_%d", index)})
		default:
			return nil, nil, adapter.ErrUnsupportedContentType
		}
	}
	if len(media) == 0 {
		text, err := prompty.StrictTextFromParts(content)
		return text, nil, err
	}
	return ordered, media, nil
}

func (a *Adapter) functionResultImage(image prompty.MediaPart, index int) (*genai.FunctionResponsePart, error) {
	if image.MediaType != "image" ||
		!slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, image.MIMEType) ||
		(len(image.Data) == 0) == (image.URL == "") {
		return nil, adapter.ErrUnsupportedContentType
	}
	name := fmt.Sprintf("prompty_part_%d", index)
	if len(image.Data) != 0 {
		//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
		return &genai.FunctionResponsePart{
			InlineData: &genai.FunctionResponseBlob{
				Data:        bytes.Clone(image.Data),
				MIMEType:    image.MIMEType,
				DisplayName: name,
			},
		}, nil
	}
	if a.client == nil || a.client.ClientConfig().Backend != genai.BackendVertexAI {
		return nil, adapter.ErrUnsupportedContentType
	}
	uri, err := url.Parse(image.URL)
	if err != nil || uri.Scheme != "gs" || uri.Host == "" || uri.Path == "" || uri.Path == "/" ||
		uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || uri.Port() != "" || uri.Opaque != "" {
		return nil, adapter.ErrUnsupportedContentType
	}
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	return &genai.FunctionResponsePart{
		FileData: &genai.FunctionResponseFileData{
			FileURI:     image.URL,
			MIMEType:    image.MIMEType,
			DisplayName: name,
		},
	}, nil
}
