package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

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
		LayerKind       prompty.LayerKind         `json:"layer_kind,omitempty"`
		RequiredTools   []string                  `json:"required_tools"`
		ModelOptionsRaw json.RawMessage           `json:"model_options"`
		Metadata        map[string]any            `json:"metadata"`
		InputsRaw       map[string]any            `json:"inputs"`
		Tools           []prompty.ToolDefinition  `json:"tools"`
		ResponseFormat  *prompty.SchemaDefinition `json:"response_format"`
		Messages        []RawMessage              `json:"messages"`
		LegacyModelRaw  map[string]any            `json:"model_config"`
		LegacyInputs    *prompty.SchemaDefinition `json:"input_schema"`
	}
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wire); err != nil {
		return err
	}
	// Reject trailing JSON values after the manifest object.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("manifest: multiple JSON values are not allowed")
		}
		return err
	}

	raw.ID = wire.ID
	raw.Version = wire.Version
	raw.Description = wire.Description
	raw.LayerKind = wire.LayerKind
	raw.RequiredTools = wire.RequiredTools
	raw.Metadata = wire.Metadata
	raw.Tools = wire.Tools
	raw.ResponseFormat = wire.ResponseFormat
	raw.Messages = wire.Messages
	raw.LegacyModelConfig = wire.LegacyModelRaw
	raw.LegacyInputSchema = wire.LegacyInputs

	inputs, err := DecodeInputs(wire.InputsRaw)
	if err != nil {
		return err
	}
	raw.InputSchema = inputs

	if len(wire.ModelOptionsRaw) == 0 || string(wire.ModelOptionsRaw) == "null" {
		raw.ModelOptions = nil
		return nil
	}

	var cfg map[string]any
	if unmarshalErr := json.Unmarshal(wire.ModelOptionsRaw, &cfg); unmarshalErr != nil {
		return unmarshalErr
	}
	opts, err := DecodeModelOptions(cfg)
	if err != nil {
		return err
	}
	raw.ModelOptions = opts
	return nil
}
