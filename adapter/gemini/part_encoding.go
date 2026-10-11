package gemini

import (
	"encoding/json"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

// Canonical state codec retains an explicit empty argument object. This does
// not alter SDK HTTP serialization or attest original provider bytes.
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
