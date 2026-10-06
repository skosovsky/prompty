package prompty

import "github.com/santhosh-tekuri/jsonschema/v6"

// This compiler-only observation deliberately interrupts compilation after
// builtin keyword resolution, before queued resource roots are processed.
// It never supplies validation semantics or becomes a caller-visible cause.
type diagnosticSchemaObservationError struct{ schema *jsonschema.Schema }

func (*diagnosticSchemaObservationError) Error() string {
	return "prompty: diagnostic schema observation"
}

func registerDiagnosticObservation(compiler *jsonschema.Compiler, identity string) {
	compiler.RegisterVocabulary(&jsonschema.Vocabulary{
		URL:        identity,
		Schema:     nil,
		Subschemas: nil,
		Compile: func(ctx *jsonschema.CompilerContext, _ map[string]any) (jsonschema.SchemaExt, error) {
			return nil, &diagnosticSchemaObservationError{schema: ctx.Enqueue(nil)}
		},
	})
	compiler.AssertVocabs()
}

func diagnosticObservedTarget(schema *jsonschema.Schema, keyword string) *jsonschema.Schema {
	if keyword == "$ref" {
		return schema.Ref
	}
	if keyword == "$dynamicRef" && schema.DynamicRef != nil {
		return schema.DynamicRef.Ref
	}
	return nil // Ignored keyword in the actual dialect, not an invented edge.
}
