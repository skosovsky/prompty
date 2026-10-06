package anthropic

import (
	"bytes"
	"encoding/json"
)

// Duplicate object members are not an authoritative extraction result, even
// when a decoder would overwrite them. Raw children avoid numeric conversion.
func uniqueSyntheticJSON(document json.RawMessage) bool {
	if !json.Valid(document) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	return uniqueSyntheticValue(decoder)
}

func uniqueSyntheticValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	if token != json.Delim('{') && token != json.Delim('[') {
		return true
	}
	keys := make(map[string]bool)
	for decoder.More() {
		if token == json.Delim('{') {
			key, keyErr := decoder.Token()
			name, ok := key.(string)
			if keyErr != nil || !ok || keys[name] {
				return false
			}
			keys[name] = true
		}
		if !uniqueSyntheticValue(decoder) {
			return false
		}
	}
	_, err = decoder.Token()
	return err == nil
}
