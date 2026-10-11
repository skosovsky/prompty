package anthropic

import (
	"bytes"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty/adapter"
)

// SDK decoding may coerce numbers or booleans to strings. Observed model
// metadata must come from a genuine source string, not that convenience value.
func observedMessageModel(message *anthropic.Message) (string, error) {
	if message.RawJSON() == "" {
		return message.Model, nil
	}
	raw := []byte(message.RawJSON())
	if !uniqueSyntheticJSON(raw) {
		return "", adapter.ErrInvalidResponse
	}
	fields, err := nativeSourceObject(raw)
	if err != nil {
		return "", err
	}
	model, present := fields["model"]
	if !present || bytes.Equal(bytes.TrimSpace(model), []byte("null")) {
		return "", nil
	}
	var value string
	if json.Unmarshal(model, &value) != nil {
		return "", adapter.ErrInvalidResponse
	}
	return value, nil
}
