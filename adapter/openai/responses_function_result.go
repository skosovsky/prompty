package openai

import (
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func responseFunctionResultContent(
	parts []prompty.ContentPart,
) (responses.ResponseFunctionCallOutputItemListParam, error) {
	// Reuse source/MIME/URL validation, not message-role translation. Native results
	// have their own content union and no role field or independent error flag.
	content, err := responseInputParts(prompty.ChatMessage{Role: prompty.RoleUser, Content: parts})
	if err != nil {
		return nil, err
	}
	var native responses.ResponseFunctionCallOutputItemListParam
	for _, item := range content {
		var mapped responses.ResponseFunctionCallOutputItemUnionParam
		switch {
		case item.OfInputText != nil:
			mapped.OfInputText = &responses.ResponseInputTextContentParam{Text: item.OfInputText.Text}
		case item.OfInputImage != nil:
			image := item.OfInputImage
			mapped.OfInputImage = &responses.ResponseInputImageContentParam{
				ImageURL: image.ImageURL, FileID: image.FileID,
				Detail: responses.ResponseInputImageContentDetail(image.Detail),
			}
		case item.OfInputFile != nil:
			file := item.OfInputFile
			mapped.OfInputFile = &responses.ResponseInputFileContentParam{
				FileData: file.FileData, FileID: file.FileID, FileURL: file.FileURL, Filename: file.Filename,
			}
		default:
			return nil, adapter.ErrUnsupportedCapability
		}
		native = append(native, mapped)
	}
	return native, nil
}
