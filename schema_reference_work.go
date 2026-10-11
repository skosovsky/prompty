package prompty

import "github.com/santhosh-tekuri/jsonschema/v6"

// Generic JSON fields are indexed, not probed. Only actual engine graph edges
// enter the worklist. Resolving a valid edge discovers arbitrary extension
// targets without treating annotations or unreferenced definitions as schemas.
type diagnosticReferenceWork struct {
	byLocation map[SchemaSourceLocation]*diagnosticReference
	missing    map[string]bool
	seen       map[*jsonschema.Schema]bool
	enqueued   map[*diagnosticReference]bool
	pending    []*diagnosticReference
}

func newDiagnosticReferenceWork(
	references []*diagnosticReference,
	placeholders map[string]string,
) *diagnosticReferenceWork {
	work := &diagnosticReferenceWork{
		byLocation: make(map[SchemaSourceLocation]*diagnosticReference, len(references)),
		missing:    make(map[string]bool, len(placeholders)),
		seen:       make(map[*jsonschema.Schema]bool),
		enqueued:   make(map[*diagnosticReference]bool),
		pending:    nil,
	}
	for _, reference := range references {
		resource := reference.resource
		if resource == schemaSourceURI {
			resource = ""
		}
		work.byLocation[SchemaSourceLocation{Resource: resource, Pointer: reference.pointer + "/" + reference.keyword}] = reference
	}
	for _, uri := range placeholders {
		work.missing[uri] = true
	}
	return work
}

func (work *diagnosticReferenceWork) discover(graph *jsonschema.Schema) {
	var locations []SchemaSourceLocation
	collectMissingSchemaOrigins(graph, work.missing, work.seen, &locations)
	for _, location := range locations {
		reference := work.byLocation[location]
		if reference != nil && !work.enqueued[reference] {
			work.enqueued[reference] = true
			work.pending = append(work.pending, reference)
		}
	}
}
