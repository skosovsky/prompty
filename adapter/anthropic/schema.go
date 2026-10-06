package anthropic

import (
	"encoding/json"

	"github.com/skosovsky/prompty"
)

// MapSchema passes canonical object schemas to native JSON schema SDK fields.
// Profile availability is checked separately; final results still require original validation.
func (*Adapter) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if request.Profile.Provider != "anthropic" {
		return prompty.SchemaMapping{}, &prompty.SchemaFidelityError{Pointer: "", Reason: "foreign schema profile"}
	}
	if _, err := prompty.SchemaDocumentAsMap(request.Source); err != nil {
		return prompty.SchemaMapping{}, err
	}
	return prompty.SchemaMapping{Schema: prompty.CloneJSONDocument(request.Source)}, nil
}

func sdkSchemaMapNumbers(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, item := range source {
		out[key] = sdkSchemaNumbers(item)
	}
	return out
}

// The SDK treats [json.Number] as a named string instead of a JSON numeric literal.
// [json.RawMessage] implements [json.Marshaler] and preserves exact numeric constraints.
func sdkSchemaNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		return json.RawMessage(typed)
	case map[string]any:
		return sdkSchemaMapNumbers(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sdkSchemaNumbers(item)
		}
		return out
	default:
		return value
	}
}
