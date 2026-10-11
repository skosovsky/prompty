package prompty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// JSONDocument is an opaque JSON payload for schema/metadata provider boundaries.
type JSONDocument = json.RawMessage

// MarshalJSONDocument encodes v as JSONDocument.
func MarshalJSONDocument(v any) (JSONDocument, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json document: %w", err)
	}
	return JSONDocument(data), nil
}

// CloneJSONDocument returns a deep copy of doc.
func CloneJSONDocument(doc JSONDocument) JSONDocument {
	if len(doc) == 0 {
		return nil
	}
	out := make([]byte, len(doc))
	copy(out, doc)
	return JSONDocument(out)
}

// JSONDocumentAsMap decodes one object, preserving dynamic numbers as [json.Number].
func JSONDocumentAsMap(doc JSONDocument) (map[string]any, error) {
	if len(doc) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if !json.Valid(doc) {
		return nil, errors.New("json document: invalid complete JSON document")
	}
	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("json document: decode map: %w", err)
	}
	if out == nil {
		return nil, errors.New("json document: expected an object, got null")
	}
	return out, nil
}

// MapToJSONDocument encodes a map as JSONDocument.
func MapToJSONDocument(m map[string]any) (JSONDocument, error) {
	if m == nil {
		return nil, nil
	}
	return MarshalJSONDocument(m)
}

// MustJSONDocumentFromMap encodes m as JSONDocument and panics on error (tests/examples).
func MustJSONDocumentFromMap(m map[string]any) JSONDocument {
	doc, err := MapToJSONDocument(m)
	if err != nil {
		panic(err)
	}
	return doc
}

// MustJSONDocumentAsMap decodes doc and panics on error (tests/examples).
func MustJSONDocumentAsMap(doc JSONDocument) map[string]any {
	out, err := JSONDocumentAsMap(doc)
	if err != nil {
		panic(err)
	}
	return out
}

// SchemaMap decodes SchemaDefinition.Schema for adapter translation.
func SchemaMap(def *SchemaDefinition) (map[string]any, error) {
	if def == nil {
		return map[string]any{}, nil
	}
	return SchemaDocumentAsMap(def.Schema)
}

// SchemaDocumentAsMap preserves JSON numbers exactly for SDK schema mapping.
// Unlike general metadata decoding, it must not round large numeric constraints.
func SchemaDocumentAsMap(doc JSONDocument) (map[string]any, error) {
	if !json.Valid(doc) {
		return nil, fmt.Errorf("%w: schema must be a complete JSON document", ErrSchemaUnsupported)
	}
	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, fmt.Errorf("%w: SDK schema mapping requires an object", ErrSchemaUnsupported)
	}
	return object, nil
}

// ProviderSettingsMap decodes ModelOptions.ProviderSettings for adapter translation.
func ProviderSettingsMap(opts *ModelOptions) (map[string]any, error) {
	if opts == nil || len(opts.ProviderSettings) == 0 {
		return map[string]any{}, nil
	}
	if !json.Valid(opts.ProviderSettings) {
		return nil, errors.New("provider settings: invalid JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(opts.ProviderSettings))
	decoder.UseNumber()
	var settings map[string]any
	if err := decoder.Decode(&settings); err != nil {
		return nil, errors.New("provider settings: invalid JSON object")
	}
	if settings == nil {
		return nil, errors.New("provider settings: invalid JSON object")
	}
	return settings, nil
}
