package openai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/skosovsky/prompty"
)

const (
	// LoweringCloseObject forbids originally permitted additional properties.
	LoweringCloseObject = "openai.close-object"
	// LoweringRequiredNullable replaces omission with a required nullable value.
	LoweringRequiredNullable = "openai.required-nullable"
)

// MapSchema preserves tool parameters and explicitly lowers strict output schemas.
// Provider availability is decided by the checked profile, not by this mapper.
func (*Adapter) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if request.Profile.Provider != wireOpenai {
		return prompty.SchemaMapping{}, schemaError("", "foreign schema profile")
	}
	source, err := prompty.SchemaDocumentAsMap(request.Source)
	if err != nil {
		return prompty.SchemaMapping{}, err
	}
	if request.Role == prompty.SchemaToolInput {
		return prompty.SchemaMapping{Transformations: nil, Schema: prompty.CloneJSONDocument(request.Source)}, nil
	}
	changes, err := mapOutputSchema(source, request.Policy)
	if err != nil {
		return prompty.SchemaMapping{}, err
	}
	translated, err := json.Marshal(source)
	return prompty.SchemaMapping{Schema: translated, Transformations: changes}, err
}

func schemaError(pointer, reason string) error {
	return &prompty.SchemaFidelityError{Pointer: pointer, Reason: reason}
}

func schemaPointer(parent, key string) string {
	return parent + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func mapOutputSchema(source map[string]any, policy prompty.SchemaPolicy) ([]prompty.SchemaTransformation, error) {
	var changes []prompty.SchemaTransformation
	allow := func(id, pointer, reason string) error {
		if policy.Mode != prompty.SchemaLowering || !slices.Contains(policy.AllowedTransforms, id) {
			return schemaError(pointer, "strict output requires an explicitly permitted transformation: "+id)
		}
		changes = append(changes, prompty.SchemaTransformation{ID: id, Pointer: pointer, Reason: reason})
		return nil
	}
	err := walkOutputSchema(source, "", allow)
	return changes, err
}

type schemaChange func(id, pointer, reason string) error

func walkOutputSchema(value any, pointer string, allow schemaChange) error {
	node, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if err := walkSchemaChildren(node, pointer, allow); err != nil {
		return err
	}
	if schemaTypeIncludes(node[wireType], "object") {
		additionalAllowed, ok := node["additionalProperties"].(bool)
		if !ok || additionalAllowed {
			path := schemaPointer(pointer, "additionalProperties")
			err := allow(LoweringCloseObject, path, "additional properties become forbidden")
			if err != nil {
				return err
			}
			node["additionalProperties"] = false
		}
		if err := requireOutputProperties(node, pointer, allow); err != nil {
			return err
		}
	}
	return nil
}

func requireOutputProperties(node map[string]any, pointer string, allow schemaChange) error {
	properties, _ := node[wireProperties].(map[string]any)
	required := requiredNames(node["required"])
	names := sortedSchemaKeys(properties)
	for _, name := range names {
		if required[name] {
			continue
		}
		path := schemaPointer(schemaPointer(pointer, wireProperties), name)
		err := allow(LoweringRequiredNullable, path, "optional property becomes required and accepts null")
		if err != nil {
			return err
		}
		// Wrapping the complete schema also handles enum, const, refs and compositions.
		properties[name] = map[string]any{wireAnyOf: []any{properties[name], map[string]any{wireType: "null"}}}
		node["required"] = append(requiredNamesInOrder(node["required"]), name)
	}
	return nil
}

func walkSchemaChildren(node map[string]any, pointer string, allow schemaChange) error {
	for _, key := range []string{wireDefs, wireDefinitions, wireProperties, "patternProperties", "dependentSchemas", "dependencies"} {
		children, _ := node[key].(map[string]any)
		for _, name := range sortedSchemaKeys(children) {
			path := schemaPointer(schemaPointer(pointer, key), name)
			if err := walkOutputSchema(children[name], path, allow); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{
		wireItems, "additionalItems", "additionalProperties", "unevaluatedProperties", "unevaluatedItems",
		"contains", "not", "if", "then", "else", "propertyNames",
	} {
		if err := walkOutputSchema(node[key], schemaPointer(pointer, key), allow); err != nil {
			return err
		}
	}
	for _, key := range []string{wireAnyOf, "allOf", "oneOf", "prefixItems", wireItems} {
		if err := walkSchemaArray(node[key], schemaPointer(pointer, key), allow); err != nil {
			return err
		}
	}
	return nil
}

func walkSchemaArray(value any, pointer string, allow schemaChange) error {
	children, _ := value.([]any)
	for i, child := range children {
		if err := walkOutputSchema(child, fmt.Sprintf("%s/%d", pointer, i), allow); err != nil {
			return err
		}
	}
	return nil
}

func sortedSchemaKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// The SDK reflection encoder needs [json.RawMessage], not named-string [json.Number].
func schemaSDKMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, item := range source {
		out[key] = schemaSDKNumbers(item)
	}
	return out
}

func schemaSDKNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		return json.RawMessage(typed)
	case map[string]any:
		return schemaSDKMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = schemaSDKNumbers(item)
		}
		return out
	default:
		return value
	}
}
