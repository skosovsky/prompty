package openai

import (
	"encoding/base64"
	"net/url"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func responseInputParts(message prompty.ChatMessage) (responses.ResponseInputMessageContentListParam, error) {
	if !responseCacheOmitted(message.CachePolicy) {
		return nil, adapter.ErrUnsupportedCapability
	}
	if len(message.ProviderState) != 0 || message.ContinuationUnavailable ||
		len(message.Annotations) != 0 || len(message.MessageAnnotations) != 0 || len(message.Metadata) != 0 {
		return nil, adapter.ErrUnsupportedCapability
	}
	var content responses.ResponseInputMessageContentListParam
	for _, part := range message.Content {
		var item responses.ResponseInputContentUnionParam
		switch value := part.(type) {
		case prompty.TextPart:
			if !responseCacheOmitted(value.CachePolicy) {
				return nil, adapter.ErrUnsupportedCapability
			}
			item.OfInputText = &responses.ResponseInputTextParam{Text: value.Text}
		case *prompty.TextPart:
			if value == nil || !responseCacheOmitted(value.CachePolicy) {
				return nil, adapter.ErrUnsupportedCapability
			}
			item.OfInputText = &responses.ResponseInputTextParam{Text: value.Text}
		case prompty.MediaPart:
			var err error
			item, err = responseInputMedia(message.Role, value)
			if err != nil {
				return nil, err
			}
		case *prompty.MediaPart:
			if value == nil {
				return nil, adapter.ErrUnsupportedCapability
			}
			var err error
			item, err = responseInputMedia(message.Role, *value)
			if err != nil {
				return nil, err
			}
		default:
			return nil, adapter.ErrUnsupportedCapability
		}
		content = append(content, item)
	}
	if len(content) == 0 {
		content = append(
			content,
			responses.ResponseInputContentUnionParam{OfInputText: &responses.ResponseInputTextParam{Text: ""}},
		)
	}
	return content, nil
}

func responseInputMedia(role prompty.Role, media prompty.MediaPart) (responses.ResponseInputContentUnionParam, error) {
	var item responses.ResponseInputContentUnionParam
	if role != prompty.RoleUser || !responseCacheOmitted(media.CachePolicy) ||
		(media.URL == "") == (len(media.Data) == 0) {
		return item, adapter.ErrUnsupportedCapability
	}
	if media.URL != "" {
		uri, err := url.Parse(media.URL)
		if err != nil || uri.Host == "" || uri.User != nil || uri.Fragment != "" ||
			(uri.Scheme != "http" && uri.Scheme != "https") {
			return item, adapter.ErrUnsupportedCapability
		}
	}
	switch media.MediaType {
	case "image":
		switch media.MIMEType {
		case "image/png", "image/jpeg", "image/webp", "image/gif":
		default:
			return item, adapter.ErrUnsupportedCapability
		}
		reference := media.URL
		if len(media.Data) > 0 {
			reference = "data:" + media.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(media.Data)
		}
		item.OfInputImage = &responses.ResponseInputImageParam{
			ImageURL: sdk.String(reference),
			Detail:   responses.ResponseInputImageDetailAuto,
		}
	case "document":
		if media.MIMEType == "" ||
			!strings.HasPrefix(media.MIMEType, "application/") && !strings.HasPrefix(media.MIMEType, "text/") {
			return item, adapter.ErrUnsupportedCapability
		}
		file := &responses.ResponseInputFileParam{}
		if media.URL != "" {
			file.FileURL = sdk.String(media.URL)
		} else {
			if media.MIMEType != "application/pdf" {
				return item, adapter.ErrUnsupportedCapability
			}
			file.Filename = sdk.String("input.pdf")
			file.FileData = sdk.String("data:application/pdf;base64," + base64.StdEncoding.EncodeToString(media.Data))
		}
		item.OfInputFile = file
	default:
		return item, adapter.ErrUnsupportedCapability
	}
	return item, nil
}
