package gemini

import (
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const wireObject = "object"
const nativeReferenceKeyword = "$ref"

func (a *Adapter) mapNativeSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if err := a.validateNativeIdentity(request.Profile); err != nil {
		return prompty.SchemaMapping{}, err
	}
	schema, err := prompty.SchemaDocumentAsMap(request.Source)
	if err != nil {
		return prompty.SchemaMapping{}, err
	}
	if err := validateNativeSchema(schema, ""); err != nil {
		return prompty.SchemaMapping{}, err
	}
	if request.Role == prompty.SchemaToolInput && schema["type"] != wireObject {
		return prompty.SchemaMapping{}, fidelityError("/type", "tool parameters must describe an object")
	}
	return prompty.SchemaMapping{Schema: prompty.CloneJSONDocument(request.Source), Transformations: nil}, nil
}

func (a *Adapter) validateNativeIdentity(identity prompty.ProfileIdentity) error {
	if identity.Provider != wireGemini || identity.Endpoint == "" || identity.Model == "" ||
		identity.Endpoint != a.RuntimeDescriptor().Endpoint {
		return fidelityError("", "native schema destination does not match adapter binding")
	}
	return nil
}

func (a *Adapter) validateNativeExecution(exec *prompty.PromptExecution) error {
	if exec.ResponseFormat == nil && len(exec.Tools) == 0 {
		return nil
	}
	// Resolve the effective destination and capabilities, including caller overrides.
	report, err := adapter.Preflight(a.RuntimeDescriptor(), exec, "", true)
	if err != nil {
		return err
	}
	if err := a.validateNativeIdentity(report.Profile); err != nil {
		return err
	}
	if exec.ResponseFormat != nil {
		schema, err := prompty.SchemaMap(exec.ResponseFormat)
		if err != nil {
			return err
		}
		if err := validateNativeSchema(schema, ""); err != nil {
			return err
		}
	}
	for _, tool := range exec.Tools {
		schema, err := prompty.SchemaDocumentAsMap(tool.Parameters)
		if err != nil {
			return err
		}
		if err := validateNativeSchema(schema, ""); err != nil {
			return err
		}
		if schema["type"] != wireObject {
			return fidelityError("/type", "tool parameters must describe an object")
		}
	}
	return nil
}

// validateNativeSchema admits only the native subset and acyclic local references.
func validateNativeSchema(schema map[string]any, pointer string) error {
	graph := nativeSchemaGraph{nodes: make(map[string]map[string]any), edges: make(map[string][]string)}
	if err := graph.validate(schema, pointer); err != nil {
		return err
	}
	return graph.validateReferences()
}

// Sorted schema-position traversal never inspects instance values as schemas.
//
//nolint:cyclop,funlen,gocognit,gocyclo // Keyword admission follows the explicit provider subset.
func (graph *nativeSchemaGraph) validate(schema map[string]any, pointer string) error {
	if schema == nil {
		return fidelityError(pointer, "schema must be an object")
	}
	graph.nodes[pointer] = schema
	childSchema := func(child map[string]any, path string) error {
		graph.edges[pointer] = append(graph.edges[pointer], path)
		return graph.validate(child, path)
	}
	keys := make([]string, 0, len(schema))
	for key := range schema {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		value, path := schema[key], pointerKey(pointer, key)
		if _, ref := schema[nativeReferenceKeyword]; ref && !strings.HasPrefix(key, "$") {
			return fidelityError(path, "reference siblings are not preserved by provider")
		}
		switch key {
		case nativeReferenceKeyword:
			ref, ok := value.(string)
			if !ok || !strings.HasPrefix(ref, "#/") {
				return fidelityError(path, "only local JSON pointer references are supported")
			}
		case "format":
			format, ok := value.(string)
			if !ok || !slices.Contains([]string{"date", "date-time", "time"}, format) {
				return fidelityError(path, "unsupported format")
			}
		case "minItems", "maxItems":
			number, ok := value.(json.Number)
			if !ok {
				return fidelityError(path, "must be a nonnegative integer")
			}
			exact, ok := new(big.Rat).SetString(string(number))
			if !ok || !exact.IsInt() || exact.Sign() < 0 {
				return fidelityError(path, "must be a nonnegative integer")
			}
		case "anyOf", "prefixItems":
			children, ok := value.([]any)
			if !ok || (key == "anyOf" && len(children) == 0) {
				return fidelityError(path, "must be an array of schemas (anyOf must be nonempty)")
			}
			for i, value := range children {
				child, ok := value.(map[string]any)
				childPath := fmt.Sprintf("%s/%d", path, i)
				if !ok {
					return fidelityError(childPath, "schema must be an object")
				}
				if err := childSchema(child, childPath); err != nil {
					return err
				}
			}
		case "type":
			if !nativeSchemaType(value) {
				return fidelityError(path, "unsupported type")
			}
		case "title", "description":
			if _, ok := value.(string); !ok {
				return fidelityError(path, "must be a string")
			}
		case "minimum", "maximum":
			if _, ok := value.(json.Number); !ok {
				return fidelityError(path, "must be a number")
			}
		case "required", "propertyOrdering":
			values, ok := value.([]any)
			if !ok {
				return fidelityError(path, "must be an array")
			}
			for i, item := range values {
				if _, ok := item.(string); !ok {
					return fidelityError(fmt.Sprintf("%s/%d", path, i), "must be a string")
				}
			}
		case "enum":
			values, ok := value.([]any)
			if !ok || len(values) == 0 {
				return fidelityError(path, "must be a nonempty array")
			}
			for i, item := range values {
				switch item.(type) {
				case string, json.Number:
				default:
					return fidelityError(fmt.Sprintf("%s/%d", path, i), "only string and numeric enums are supported")
				}
			}
		case "properties", "$defs":
			members, ok := value.(map[string]any)
			if !ok {
				return fidelityError(path, "must be an object")
			}
			names := make([]string, 0, len(members))
			for name := range members {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				child, ok := members[name].(map[string]any)
				childPath := pointerKey(path, name)
				if !ok {
					return fidelityError(childPath, "schema must be an object")
				}
				if err := childSchema(child, childPath); err != nil {
					return err
				}
			}
		case "items", "additionalProperties":
			if key == "additionalProperties" {
				if _, ok := value.(bool); ok {
					continue
				}
			}
			child, ok := value.(map[string]any)
			if !ok {
				return fidelityError(path, "must be a schema object")
			}
			if err := childSchema(child, path); err != nil {
				return err
			}
		default:
			return fidelityError(path, "keyword is outside native JSON schema subset")
		}
	}
	return nil
}

// Native nullable types use exactly one concrete JSON type and null.
func nativeSchemaType(value any) bool {
	valid := func(value any) bool {
		name, ok := value.(string)
		return ok && (jsonSchemaTypeToGenai(name) != genai.TypeUnspecified || name == wireNull)
	}
	if values, ok := value.([]any); ok {
		return len(values) == 2 && valid(values[0]) && valid(values[1]) && values[0] != values[1] &&
			(values[0] == wireNull || values[1] == wireNull)
	}
	return valid(value)
}
