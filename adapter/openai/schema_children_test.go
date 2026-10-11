package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestStrictOutputVisitsSchemaChildren(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		schema  string
		pointer string
		valid   string
		dialect string
	}{
		{
			name:    "tuple item",
			schema:  `{"type":"array","items":[{"type":"object"}]}`,
			pointer: "/items/0/additionalProperties",
			valid:   `[{"extra":1}]`,
			dialect: prompty.SchemaDialect7,
		},
		{
			name:    "additional item",
			schema:  `{"type":"array","items":[{"type":"string"}],"additionalItems":{"type":"object"}}`,
			pointer: "/additionalItems/additionalProperties",
			valid:   `["first",{"extra":1}]`,
			dialect: prompty.SchemaDialect7,
		},
		{
			name:    "schema dependency",
			schema:  `{"dependencies":{"a~/b":{"type":"object"}}}`,
			pointer: "/dependencies/a~0~1b/additionalProperties",
			valid:   `{"a~/b":true,"extra":1}`,
			dialect: prompty.SchemaDialect7,
		},
		{
			name:    "additional property",
			schema:  `{"additionalProperties":{"type":"object"}}`,
			pointer: "/additionalProperties/additionalProperties",
			valid:   `{"value":{"extra":1}}`,
			dialect: prompty.SchemaDialect2020,
		},
		{
			name:    "unevaluated property",
			schema:  `{"unevaluatedProperties":{"type":"object"}}`,
			pointer: "/unevaluatedProperties/additionalProperties",
			valid:   `{"value":{"extra":1}}`,
			dialect: prompty.SchemaDialect2020,
		},
		{
			name:    "unevaluated item",
			schema:  `{"type":"array","unevaluatedItems":{"type":"object"}}`,
			pointer: "/unevaluatedItems/additionalProperties",
			valid:   `[{"extra":1}]`,
			dialect: prompty.SchemaDialect2020,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: the original permits additional properties in a nested object.
			source := prompty.JSONDocument(`{"$schema":"` + test.dialect + `",` + test.schema[1:])
			original := prompty.CloneJSONDocument(source)
			request := prompty.SchemaCompileRequest{
				Source:    source,
				Dialect:   test.dialect,
				Role:      prompty.SchemaOutput,
				Profile:   prompty.ProfileIdentity{Provider: "openai", Endpoint: "fixture", Model: "fixture"},
				Validator: prompty.NewJSONSchemaValidator(nil),
			}
			// Act: strict compilation must find the nested semantic change.
			_, err := prompty.CompileProviderSchema(request, New())
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			var fidelity *prompty.SchemaFidelityError
			require.ErrorAs(t, err, &fidelity)
			require.Equal(t, test.pointer, fidelity.Pointer)
			require.Equal(t, original, source)

			// Arrange: permit exactly the reported object-closing transformation.
			request.Policy = prompty.SchemaPolicy{
				Mode:              prompty.SchemaLowering,
				AllowedTransforms: []string{LoweringCloseObject},
			}
			// Act.
			compiled, err := prompty.CompileProviderSchema(request, New())
			// Assert: lowering is explicit and original validation stays authoritative.
			require.NoError(t, err)
			require.Equal(t, original, source)
			require.Equal(t, []prompty.SchemaTransformation{{
				ID: LoweringCloseObject, Pointer: test.pointer, Reason: "additional properties become forbidden",
			}}, compiled.Report().Transformations)
			require.NoError(t, compiled.ValidateOriginal(prompty.JSONDocument(test.valid)))
			translated, err := request.Validator.CompileSchema(compiled.Translated(), request.Dialect)
			require.NoError(t, err)
			require.ErrorIs(t, translated.ValidateJSON(prompty.JSONDocument(test.valid)), prompty.ErrSchemaMismatch)
		})
	}
}

func TestStrictOutputDoesNotWalkPropertyDependencyData(t *testing.T) {
	t.Parallel()
	// Arrange: property dependencies and annotations are not schema locations.
	source := prompty.JSONDocument(`{"$schema":"http://json-schema.org/draft-07/schema#",` +
		`"dependencies":{"x":["y"]},"default":{"type":"object"},` +
		`"examples":[{"type":"object"}],"contentSchema":{"type":"object"}}`)
	request := prompty.SchemaCompileRequest{
		Source:    source,
		Dialect:   prompty.SchemaDialect7,
		Role:      prompty.SchemaOutput,
		Profile:   prompty.ProfileIdentity{Provider: "openai", Endpoint: "fixture", Model: "fixture"},
		Validator: prompty.NewJSONSchemaValidator(nil),
	}
	// Act.
	compiled, err := prompty.CompileProviderSchema(request, New())
	// Assert.
	require.NoError(t, err)
	require.Empty(t, compiled.Report().Transformations)
	require.NoError(t, compiled.ValidateOriginal(prompty.JSONDocument(`{"x":1,"y":2}`)))
	require.ErrorIs(t, compiled.ValidateOriginal(prompty.JSONDocument(`{"x":1}`)), prompty.ErrSchemaMismatch)
}
