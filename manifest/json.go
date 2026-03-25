package manifest

import (
	"encoding/json"
	"errors"

	"github.com/skosovsky/prompty"
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
	var wire struct {
		ID              string                    `json:"id"`
		Version         string                    `json:"version"`
		Description     string                    `json:"description"`
		ModelOptionsRaw json.RawMessage           `json:"model_config"`
		Metadata        map[string]any            `json:"metadata"`
		InputSchema     *prompty.SchemaDefinition `json:"input_schema"`
		Tools           []prompty.ToolDefinition  `json:"tools"`
		ResponseFormat  *prompty.SchemaDefinition `json:"response_format"`
		Messages        []RawMessage              `json:"messages"`
	}
	if err := json.Unmarshal(in, &wire); err != nil {
		return err
	}

	raw.ID = wire.ID
	raw.Version = wire.Version
	raw.Description = wire.Description
	raw.Metadata = wire.Metadata
	raw.InputSchema = wire.InputSchema
	raw.Tools = wire.Tools
	raw.ResponseFormat = wire.ResponseFormat
	raw.Messages = wire.Messages

	if len(wire.ModelOptionsRaw) == 0 || string(wire.ModelOptionsRaw) == "null" {
		raw.ModelOptions = nil
		return nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(wire.ModelOptionsRaw, &cfg); err != nil {
		return err
	}
	opts, err := DecodeModelOptions(cfg)
	if err != nil {
		return err
	}
	raw.ModelOptions = opts
	return nil
}
