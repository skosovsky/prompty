package prompty

import (
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

// JSONDocumentAsMap decodes doc into a map for adapter translation (internal shape).
func JSONDocumentAsMap(doc JSONDocument) (map[string]any, error) {
	if len(doc) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(doc, &out); err != nil {
		return nil, fmt.Errorf("json document: decode map: %w", err)
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
	return JSONDocumentAsMap(def.Schema)
}

// ProviderSettingsMap decodes ModelOptions.ProviderSettings for adapter translation.
func ProviderSettingsMap(opts *ModelOptions) (map[string]any, error) {
	if opts == nil {
		return map[string]any{}, nil
	}
	return JSONDocumentAsMap(opts.ProviderSettings)
}

// newRenderPlanFromMap builds a render plan from map-shaped template variables (tests/internal only).
func newRenderPlanFromMap(tpl *ChatPromptTemplate, input map[string]any) (*RenderPlan, error) {
	if tpl == nil {
		return nil, errors.New("render plan: template is nil")
	}
	return NewRenderPlanFromMap(tpl, input), nil
}
