package prompty

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	SchemaDialect2020 = "https://json-schema.org/draft/2020-12/schema"
	SchemaDialect7    = "http://json-schema.org/draft-07/schema#"
)

var (
	ErrSchemaMismatch          = errors.New("prompty: response violates original JSON Schema")
	ErrSchemaJSONDecode        = errors.New("prompty: schema validation input is not one complete JSON document")
	ErrSchemaUnsupported       = errors.New("prompty: unsupported JSON Schema contract")
	ErrSchemaValidatorRequired = errors.New("prompty: schema validator required")
)

// ValueValidator validates a complete JSON document against an already compiled schema.
type ValueValidator interface {
	ValidateJSON(value JSONDocument) error
}

// SchemaValidator is an application-injectable schema compiler, separate from domain Validate.
// Implementations must not fetch external references without explicit caller policy.
// Successful compilation promises enforcement of the selected schema contract.
// Return an error rather than silently ignoring unsupported validation constraints,
// and never return a nil ValueValidator with a nil error.
type SchemaValidator interface {
	CompileSchema(source JSONDocument, dialect string) (ValueValidator, error)
}

// JSONSchemaValidator uses the standard validator with an isolated, explicit resource set.
// It supports the declared dialects and never performs filesystem or network reference loading.
type JSONSchemaValidator struct {
	resources map[string]JSONDocument
}

// NewJSONSchemaValidator clones caller-owned schema resources.
func NewJSONSchemaValidator(resources map[string]JSONDocument) *JSONSchemaValidator {
	out := &JSONSchemaValidator{resources: make(map[string]JSONDocument, len(resources))}
	for name, doc := range resources {
		out.resources[name] = CloneJSONDocument(doc)
	}
	return out
}

// CompileSchema checks the schema itself before any provider call.
func (v *JSONSchemaValidator) CompileSchema(source JSONDocument, dialect string) (ValueValidator, error) {
	if dialect == "" {
		dialect = SchemaDialect2020
	}
	if !slices.Contains([]string{SchemaDialect2020, SchemaDialect7}, dialect) {
		return nil, newSchemaCompilationError(errors.New("unsupported schema dialect"))
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(source))
	if err != nil {
		return nil, newSchemaCompilationError(err)
	}
	if object, ok := document.(map[string]any); ok {
		if declared, ok := object["$schema"].(string); ok && declared != dialect {
			return nil, &SchemaCompilationError{
				Locations: []SchemaSourceLocation{{Resource: "", Pointer: "/$schema"}},
				Cause:     errors.New("declared schema dialect differs from requested dialect"),
			}
		}
	}
	var resources map[string]JSONDocument
	if v != nil {
		resources = v.resources
	}
	compiler, err := newJSONSchemaCompiler(document, dialect, resources, explicitSchemaLoader{})
	if err != nil {
		return nil, newSchemaCompilationError(err)
	}
	schema, err := compiler.Compile(schemaSourceURI)
	if err != nil {
		failure := newSchemaCompilationError(err)
		if isSchemaReferenceFailure(err) {
			failure.Locations = missingSchemaReferenceOrigins(document, dialect, resources)
			sortSchemaSourceLocations(failure.Locations)
			failure.Locations = slices.Compact(failure.Locations)
		}
		return nil, failure
	}
	return compiledJSONValidator{schema: schema}, nil
}

func newJSONSchemaCompiler(
	document any,
	dialect string,
	resources map[string]JSONDocument,
	loader jsonschema.URLLoader,
) (*jsonschema.Compiler, error) {
	compiler := jsonschema.NewCompiler()
	if dialect == SchemaDialect7 {
		compiler.DefaultDraft(jsonschema.Draft7)
	} else {
		compiler.DefaultDraft(jsonschema.Draft2020)
	}
	compiler.UseLoader(loader)
	if resources != nil {
		for _, name := range slices.Sorted(maps.Keys(resources)) {
			resource := resources[name]
			doc, parseErr := jsonschema.UnmarshalJSON(bytes.NewReader(resource))
			if parseErr != nil {
				return nil, parseErr
			}
			if addErr := compiler.AddResource(name, doc); addErr != nil {
				return nil, addErr
			}
		}
	}
	if addErr := compiler.AddResource(schemaSourceURI, document); addErr != nil {
		return nil, addErr
	}
	return compiler, nil
}

type explicitSchemaLoader struct{}

func (explicitSchemaLoader) Load(location string) (any, error) {
	return nil, fmt.Errorf("%w: reference %q is not a supplied resource", ErrSchemaUnsupported, location)
}

type compiledJSONValidator struct{ schema *jsonschema.Schema }

func (v compiledJSONValidator) ValidateJSON(value JSONDocument) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(value))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSchemaJSONDecode, err)
	}
	if err := v.schema.Validate(document); err != nil {
		return fmt.Errorf("%w: %w", ErrSchemaMismatch, err)
	}
	return nil
}
