package openai

import (
	"encoding/json"
	"strings"

	"github.com/skosovsky/prompty/adapter"
)

// observedModel checks the consumed source field before trusting SDK string
// coercion. Raw-free caller-owned SDK values remain valid low-level inputs.
func observedModel(source, model string) (string, error) {
	if source == "" {
		return model, nil
	}
	decoder := json.NewDecoder(strings.NewReader(source))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return "", adapter.ErrInvalidResponse
	}
	seen := false
	for decoder.More() {
		key, keyErr := decoder.Token()
		var value json.RawMessage
		if keyErr != nil || decoder.Decode(&value) != nil {
			return "", adapter.ErrInvalidResponse
		}
		if key != "model" {
			continue
		}
		var reported string
		if seen || len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &reported) != nil || reported != model {
			return "", adapter.ErrInvalidResponse
		}
		seen = true
	}
	if _, err = decoder.Token(); err != nil {
		return "", adapter.ErrInvalidResponse
	}
	if !seen {
		return "", nil
	}
	return model, nil
}
