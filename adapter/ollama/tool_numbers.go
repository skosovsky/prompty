package ollama

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty/adapter"
)

func decodeExactToolArguments(data []byte) (api.ToolCallFunctionArguments, error) {
	var empty api.ToolCallFunctionArguments
	if !json.Valid(data) {
		return empty, adapter.ErrMalformedArgs
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var object map[string]any
	if decoder.Decode(&object) != nil || object == nil {
		return empty, adapter.ErrMalformedArgs
	}
	args := api.NewToolCallFunctionArguments()
	for key, value := range object {
		args.Set(key, value)
	}
	return args, nil
}

func unverifiedToolNumbers(value any) bool {
	switch v := value.(type) {
	case float32, float64:
		return true
	case map[string]any:
		for _, nested := range v {
			if unverifiedToolNumbers(nested) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, unverifiedToolNumbers)
	}
	return false
}
