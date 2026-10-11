package gemini

import (
	"encoding/json"
	"fmt"
	"slices"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const wireObject = "object"

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

// References and composition are deliberately outside this initial subset.
// Sorted schema-position traversal never inspects instance values as schemas.
//
//nolint:cyclop,funlen,gocognit,gocyclo // Keyword admission follows the explicit provider subset.
func validateNativeSchema(schema map[string]any, pointer string) error {
	if schema == nil {
		return fidelityError(pointer, "schema must be an object")
	}
	keys := make([]string, 0, len(schema))
	for key := range schema {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		value, path := schema[key], pointerKey(pointer, key)
		switch key {
		case "type":
			name, ok := value.(string)
			if !ok || (jsonSchemaTypeToGenai(name) == genai.TypeUnspecified && name != "null") {
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
		case "required":
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
		case "properties":
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
				if err := validateNativeSchema(child, childPath); err != nil {
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
			if err := validateNativeSchema(child, path); err != nil {
				return err
			}
		default:
			return fidelityError(path, "keyword is outside native JSON schema subset")
		}
	}
	return nil
}

// preserveNativeSchemaNumbers restores schema bytes after genai's float64 DTO conversion.
// Only schema-bearing fields are restored; caller extras and transport policy stay intact.
func preserveNativeSchemaNumbers(config *genai.GenerateContentConfig, fallback genai.ExtrasRequestProvider) error {
	hasSchema := config.ResponseJsonSchema != nil
	hasTools := len(config.Tools) > 0
	schema, err := json.Marshal(config.ResponseJsonSchema)
	if err != nil {
		return err
	}
	tools, err := json.Marshal(config.Tools)
	if err != nil {
		return err
	}
	//nolint:exhaustruct_v5 // SDK DTO: copy caller HTTP options when present.
	options := genai.HTTPOptions{}
	if config.HTTPOptions != nil {
		options = *config.HTTPOptions
		options.Headers = options.Headers.Clone()
	}
	previous := options.ExtrasRequestProvider
	if previous == nil {
		previous = fallback
	}
	options.ExtrasRequestProvider = func(body map[string]any) map[string]any {
		if previous != nil {
			body = previous(body)
		}
		if body == nil {
			return map[string]any{"invalid_native_request": json.RawMessage(`!`)}
		}
		if hasSchema {
			generation, ok := body["generationConfig"].(map[string]any)
			if !ok {
				generation = map[string]any{}
			}
			generation["responseJsonSchema"] = json.RawMessage(schema)
			delete(generation, "responseSchema")
			body["generationConfig"] = generation
		}
		if hasTools {
			body["tools"] = json.RawMessage(tools)
		}
		return body
	}
	config.HTTPOptions = &options
	return nil
}

func (a *Adapter) preserveSchemaNumbers(config *genai.GenerateContentConfig) error {
	if !a.nativeJSONSchema {
		return nil
	}
	var fallback genai.ExtrasRequestProvider
	if a.client != nil {
		fallback = a.client.ClientConfig().HTTPOptions.ExtrasRequestProvider
	}
	return preserveNativeSchemaNumbers(config, fallback)
}
