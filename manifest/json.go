package manifest

import (
	"encoding/json"
	"errors"
)

// JSONParser implements Unmarshaler for JSON manifests (no YAML dependency).
type JSONParser struct{}

// NewJSONParser returns a parser for JSON manifests.
func NewJSONParser() *JSONParser {
	return &JSONParser{}
}

// Unmarshal parses JSON into RawManifest.
func (JSONParser) Unmarshal(in []byte, out any) error {
	raw, ok := out.(*RawManifest)
	if !ok {
		return errors.New("manifest: out must be *RawManifest")
	}
	return json.Unmarshal(in, raw)
}
