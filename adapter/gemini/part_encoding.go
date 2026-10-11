package gemini

import (
	"encoding/json"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

// SDK omitempty drops an explicit empty argument object. Preserve it in source
// state and final contents just as nonempty exact numeric arguments are kept.
func encodeExactGeminiPart(part *genai.Part) ([]byte, error) {
	encoded, err := json.Marshal(part)
	if err != nil {
		return nil, prompty.ErrProviderState
	}
	if part == nil || part.FunctionCall == nil || part.FunctionCall.Args == nil || len(part.FunctionCall.Args) != 0 {
		return encoded, nil
	}
	object, ok := geminiJSONValue(encoded).(map[string]any)
	if !ok {
		return nil, prompty.ErrProviderState
	}
	call, ok := object["functionCall"].(map[string]any)
	if !ok {
		return nil, prompty.ErrProviderState
	}
	call["args"] = map[string]any{}
	return json.Marshal(object)
}
func encodeExactGeminiContents(contents []*genai.Content) ([]byte, error) {
	encoded, err := json.Marshal(contents)
	if err != nil {
		return nil, prompty.ErrProviderState
	}
	if contents == nil {
		return encoded, nil
	}
	objects, ok := geminiJSONValue(encoded).([]any)
	if !ok {
		return nil, prompty.ErrProviderState
	}
	for index, content := range contents {
		if content == nil {
			continue
		}
		object, ok := objects[index].(map[string]any)
		if !ok {
			return nil, prompty.ErrProviderState
		}
		if len(content.Parts) == 0 {
			continue
		}
		parts, ok := object["parts"].([]any)
		if !ok || len(parts) != len(content.Parts) {
			return nil, prompty.ErrProviderState
		}
		for ordinal, part := range content.Parts {
			exact, partErr := encodeExactGeminiPart(part)
			if partErr != nil {
				return nil, partErr
			}
			parts[ordinal] = geminiJSONValue(exact)
		}
	}
	return json.Marshal(objects)
}
