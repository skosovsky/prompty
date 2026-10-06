package prompty

import (
	"bytes"
	"crypto/rand"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type diagnosticReference struct {
	resource    string
	pointer     string
	keyword     string
	value       string
	placeholder string
	node        map[string]any
	failed      bool
}

// Candidate positions are not origins. Only the final root-reachable engine
// graph verifies which diagnostic edges are active in the original dialect.
func failedSchemaReferenceOrigins(
	document any,
	dialect string,
	resources map[string]JSONDocument,
	sourceURIs ...string,
) []SchemaSourceLocation {
	sourceURI := schemaSourceURI
	if len(sourceURIs) > 0 {
		sourceURI = sourceURIs[0]
	}
	placeholder := "urn:prompty:reference-diagnostic:" + rand.Text()
	placeholders := make(map[string]string)
	var references []*diagnosticReference
	documents := make(map[string]any, len(resources)+1)
	documents[sourceURI] = neutralizeDiagnosticReferences(
		document,
		sourceURI,
		"",
		placeholder,
		placeholders,
		&references,
	)
	for _, name := range slices.Sorted(maps.Keys(resources)) {
		parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(resources[name]))
		if err != nil {
			return nil
		}
		documents[name] = neutralizeDiagnosticReferences(parsed, name, "", placeholder, placeholders, &references)
	}
	compiler, err := diagnosticReferenceCompiler(documents, dialect, placeholders)
	if err != nil {
		return nil
	}
	// An immutable all-neutralized graph seeds target existence metadata once.
	// Never use it to validate caller data or to claim original reference validity.
	existence, err := diagnosticReferenceCompiler(documents, dialect, placeholders)
	if err != nil {
		return nil
	}
	rootGraph, err := existence.Compile(sourceURI)
	if err != nil {
		return nil
	}
	work := newDiagnosticReferenceWork(references, placeholders)
	work.discover(rootGraph)
	registerDiagnosticObservation(compiler, placeholder+":observation")
	for len(work.pending) > 0 {
		reference := work.pending[0]
		work.pending = work.pending[1:]
		if !probeDiagnosticReference(compiler, existence, reference, work) {
			return nil
		}
	}
	for _, reference := range references {
		if !reference.failed {
			reference.node[reference.keyword] = reference.value
		}
	}
	// Restore valid refs so definitions/extension targets and original reachability
	// are retained. False placeholders are diagnostic-only and never validators.
	compiler, err = diagnosticReferenceCompiler(documents, dialect, placeholders)
	if err != nil {
		return nil
	}
	graph, err := compiler.Compile(sourceURI)
	if err != nil {
		return nil
	}
	var locations []SchemaSourceLocation
	missing := make(map[string]bool, len(placeholders))
	for _, uri := range placeholders {
		missing[uri] = true
	}
	collectMissingSchemaOrigins(
		graph,
		missing,
		make(map[*jsonschema.Schema]bool),
		&locations,
	)
	return locations
}

func diagnosticReferenceCompiler(
	documents map[string]any,
	dialect string,
	placeholders map[string]string,
) (*jsonschema.Compiler, error) {
	compiler := jsonschema.NewCompiler()
	if dialect == SchemaDialect7 {
		compiler.DefaultDraft(jsonschema.Draft7)
	} else {
		compiler.DefaultDraft(jsonschema.Draft2020)
	}
	compiler.UseLoader(explicitSchemaLoader{})
	for _, name := range slices.Sorted(maps.Keys(documents)) {
		if err := compiler.AddResource(name, documents[name]); err != nil {
			return nil, err
		}
	}
	for _, uri := range placeholders {
		if err := compiler.AddResource(uri, false); err != nil {
			return nil, err
		}
	}
	return compiler, nil
}

func probeDiagnosticReference(
	compiler, existence *jsonschema.Compiler,
	reference *diagnosticReference,
	work *diagnosticReferenceWork,
) bool {
	// One temporary direct edge; engine alone resolves dialect/IDs/URI/anchors.
	reference.node[reference.keyword] = reference.value
	location, parseErr := url.Parse(reference.resource)
	if parseErr != nil {
		return false
	}
	location.Fragment, location.RawFragment = reference.pointer, ""
	_, err := compiler.Compile(location.String())
	reference.node[reference.keyword] = reference.placeholder
	if observation, observed := errors.AsType[*diagnosticSchemaObservationError](err); observed {
		target := diagnosticObservedTarget(observation.schema, reference.keyword)
		if target != nil {
			// Absolute physical target is engine metadata, not a custom resolver.
			var targetGraph *jsonschema.Schema
			targetGraph, err = existence.Compile(target.Location)
			if err == nil {
				work.discover(targetGraph)
			}
		}
	}
	// Non-reference failure is inconclusive, never a successful validator.
	reference.failed = isSchemaReferenceFailure(err)
	return true
}

func neutralizeDiagnosticReferences(
	value any,
	resource, pointer, placeholder string,
	placeholders map[string]string,
	references *[]*diagnosticReference,
) any {
	switch v := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(v))
		for _, key := range slices.Sorted(maps.Keys(v)) {
			child := v[key]
			if text, ok := child.(string); ok && (key == schemaKeywordRef || key == schemaKeywordDynamicRef) {
				uri, exists := placeholders[text]
				if !exists {
					uri = placeholder + ":value:" + strconv.Itoa(len(placeholders))
					placeholders[text] = uri
				}
				*references = append(*references, &diagnosticReference{
					resource:    resource,
					pointer:     pointer,
					keyword:     key,
					value:       text,
					placeholder: uri,
					node:        clone,
					failed:      false,
				})
				clone[key] = uri
				continue
			}
			token := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			clone[key] = neutralizeDiagnosticReferences(
				child,
				resource,
				pointer+"/"+token,
				placeholder,
				placeholders,
				references,
			)
		}
		return clone
	case []any:
		clone := make([]any, len(v))
		for index, child := range v {
			clone[index] = neutralizeDiagnosticReferences(
				child,
				resource,
				pointer+"/"+strconv.Itoa(index),
				placeholder,
				placeholders,
				references,
			)
		}
		return clone
	default:
		return value
	}
}

func isSchemaReferenceFailure(err error) bool {
	_, load := errors.AsType[*jsonschema.LoadURLError](err)
	_, anchor := errors.AsType[*jsonschema.AnchorNotFoundError](err)
	_, pointer := errors.AsType[*jsonschema.JSONPointerNotFoundError](err)
	_, invalid := errors.AsType[*jsonschema.InvalidJsonPointerError](err)
	_, uri := errors.AsType[*jsonschema.ParseURLError](err)
	return load || anchor || pointer || invalid || uri
}
