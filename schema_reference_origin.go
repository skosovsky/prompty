package prompty

import (
	"net/url"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// This isolated compiler is only a source-location probe after real compilation
// failed. Its false-schema placeholders and compiled graph must never validate data.
func missingSchemaReferenceOrigins(
	document any,
	dialect string,
	resources map[string]JSONDocument,
) []SchemaSourceLocation {
	loader := &diagnosticSchemaLoader{missing: make(map[string]bool)}
	compiler, err := newJSONSchemaCompiler(document, dialect, resources, loader)
	if err != nil {
		return nil
	}
	graph, err := compiler.Compile(schemaSourceURI)
	if err != nil {
		return failedSchemaReferenceOrigins(document, dialect, resources)
	}
	var locations []SchemaSourceLocation
	seen := make(map[*jsonschema.Schema]bool)
	collectMissingSchemaOrigins(graph, loader.missing, seen, &locations)
	return locations
}

type diagnosticSchemaLoader struct{ missing map[string]bool }

func (l *diagnosticSchemaLoader) Load(location string) (any, error) {
	l.missing[location] = true
	return false, nil // Compilation metadata only; no network or successful validator.
}

func collectMissingSchemaOrigins(
	schema *jsonschema.Schema,
	missing map[string]bool,
	seen map[*jsonschema.Schema]bool,
	locations *[]SchemaSourceLocation,
) {
	if schema == nil || seen[schema] {
		return
	}
	seen[schema] = true
	appendMissingSchemaOrigin(schema, schema.Ref, "$ref", missing, locations)
	appendMissingSchemaOrigin(schema, schema.RecursiveRef, "$recursiveRef", missing, locations)
	if schema.DynamicRef != nil {
		appendMissingSchemaOrigin(schema, schema.DynamicRef.Ref, "$dynamicRef", missing, locations)
		collectMissingSchemaOrigins(schema.DynamicRef.Ref, missing, seen, locations)
	}
	for _, child := range []any{
		schema.Ref, schema.RecursiveRef, schema.Not, schema.AllOf, schema.AnyOf, schema.OneOf,
		schema.If, schema.Then, schema.Else, schema.PropertyNames, schema.Properties,
		schema.AdditionalProperties, schema.Dependencies, schema.DependentSchemas,
		schema.UnevaluatedProperties, schema.Contains, schema.Items, schema.AdditionalItems,
		schema.PrefixItems, schema.Items2020, schema.UnevaluatedItems, schema.ContentSchema,
	} {
		collectMissingSchemaChildren(child, missing, seen, locations)
	}
	for _, child := range schema.PatternProperties {
		collectMissingSchemaOrigins(child, missing, seen, locations)
	}
}

func collectMissingSchemaChildren(
	value any,
	missing map[string]bool,
	seen map[*jsonschema.Schema]bool,
	locations *[]SchemaSourceLocation,
) {
	switch children := value.(type) {
	case *jsonschema.Schema:
		collectMissingSchemaOrigins(children, missing, seen, locations)
	case []*jsonschema.Schema:
		for _, child := range children {
			collectMissingSchemaOrigins(child, missing, seen, locations)
		}
	case map[string]*jsonschema.Schema:
		for _, child := range children {
			collectMissingSchemaOrigins(child, missing, seen, locations)
		}
	case map[string]any:
		for _, child := range children {
			collectMissingSchemaChildren(child, missing, seen, locations)
		}
	}
}

func appendMissingSchemaOrigin(
	origin, target *jsonschema.Schema,
	keyword string,
	missing map[string]bool,
	locations *[]SchemaSourceLocation,
) {
	if target == nil {
		return
	}
	u, err := url.Parse(target.Location)
	if err != nil {
		return
	}
	u.Fragment, u.RawFragment = "", ""
	if !missing[u.String()] {
		return
	}
	if location, valid := schemaCompilationLocation(origin.Location); valid {
		location.Pointer += "/" + keyword
		*locations = append(*locations, location)
	}
}
