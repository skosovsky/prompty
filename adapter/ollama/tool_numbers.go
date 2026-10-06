package ollama

import (
	"bytes"
	"encoding/json"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty/adapter"
)

func decodeExactToolArguments(data []byte) (api.ToolCallFunctionArguments, error) {
	var empty api.ToolCallFunctionArguments
	if !json.Valid(data) || uniqueArgumentValue(json.NewDecoder(bytes.NewReader(data))) != nil {
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

func unverifiedToolNumbers(value any) bool { return adapter.HasUnverifiedJSONNumbers(value) }

func uniqueArgumentValue(decoder *json.Decoder) error {
	decoder.UseNumber()
	token, tokenErr := decoder.Token()
	if tokenErr != nil {
		return adapter.ErrMalformedArgs
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	if delimiter == '{' {
		return uniqueArgumentObject(decoder)
	}
	if delimiter != '[' {
		return adapter.ErrMalformedArgs
	}
	for decoder.More() {
		if err := uniqueArgumentValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim(']') {
		return adapter.ErrMalformedArgs
	}
	return nil
}

func uniqueArgumentObject(decoder *json.Decoder) error {
	keys := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return adapter.ErrMalformedArgs
		}
		key, ok := token.(string)
		if !ok || keys[key] {
			return adapter.ErrMalformedArgs
		}
		keys[key] = true
		if err := uniqueArgumentValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return adapter.ErrMalformedArgs
	}
	return nil
}
