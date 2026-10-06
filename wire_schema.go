package prompty

import (
	"embed"
	"encoding/json"
)

//go:embed schemas/wire-common.json schemas/execution-wire.json schemas/response-wire.json schemas/cache-resource-wire.json schemas/remote-response-wire.json schemas/batch-wire.json
var wireSchemaFiles embed.FS

// WireSchemaResources returns isolated executable schemas with explicit resources.
// Part association/scope/expiry checks are additional semantic invariants in Go.
func WireSchemaResources() map[string]JSONDocument {
	resources := make(map[string]JSONDocument)
	for uri, path := range map[string]string{
		"https://prompty.invalid/wire-common":          "schemas/wire-common.json",
		"https://prompty.invalid/execution-wire":       "schemas/execution-wire.json",
		"https://prompty.invalid/response-wire":        "schemas/response-wire.json",
		"https://prompty.invalid/cache-resource-wire":  "schemas/cache-resource-wire.json",
		"https://prompty.invalid/remote-response-wire": "schemas/remote-response-wire.json",
		"https://prompty.invalid/batch-wire":           "schemas/batch-wire.json",
	} {
		data, err := wireSchemaFiles.ReadFile(path)
		if err != nil {
			panic("prompty: embedded wire schema missing")
		}
		resources[uri] = JSONDocument(data)
	}
	return resources
}

// ValidateWireJSON validates the full serialized contract before DTO decoding.
// The compiler uses only bundled resources, with no network/filesystem loading.
func ValidateWireJSON(document JSONDocument, format string) error {
	var uri string
	switch format {
	case ExecutionWireFormat:
		uri = "https://prompty.invalid/execution-wire"
	case ResponseWireFormat:
		uri = "https://prompty.invalid/response-wire"
	case CacheResourceWireFormat:
		uri = "https://prompty.invalid/cache-resource-wire"
	case RemoteResponseWireFormat:
		uri = "https://prompty.invalid/remote-response-wire"
	case BatchWireFormat:
		uri = "https://prompty.invalid/batch-wire"
	default:
		return ErrWireFormat
	}
	resources := WireSchemaResources()
	validator, err := NewJSONSchemaValidator(resources).CompileSchema(resources[uri], SchemaDialect2020)
	if err != nil {
		return err
	}
	return validator.ValidateJSON(document)
}

// DecodeExecutionJSON validates the schema, format and semantic associations.
func DecodeExecutionJSON(document JSONDocument) (*PromptExecution, error) {
	if err := ValidateWireJSON(document, ExecutionWireFormat); err != nil {
		return nil, err
	}
	var wire PromptExecutionWire
	if err := json.Unmarshal(document, &wire); err != nil {
		return nil, err
	}
	return UnmarshalExecution(wire)
}

// DecodeResponseJSON validates the schema, format and semantic associations.
func DecodeResponseJSON(document JSONDocument) (*Response, error) {
	if err := ValidateWireJSON(document, ResponseWireFormat); err != nil {
		return nil, err
	}
	var wire ResponseWire
	if err := json.Unmarshal(document, &wire); err != nil {
		return nil, err
	}
	return UnmarshalResponse(wire)
}
