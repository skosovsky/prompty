package prompty

import (
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestMissingResourceOriginsComeFromEngineRefEdges(t *testing.T) {
	t.Parallel()
	const missing = "https://fixture.invalid/missing"
	const resource = "https://fixture.invalid/resource"
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		for _, tc := range []struct {
			name, source string
			resources    map[string]JSONDocument
			locations    []SchemaSourceLocation
		}{
			{"root ref", `{"$ref":"` + missing + `"}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/$ref"}}},
			{"repeated escaped refs", `{"properties":{"a/b~c":{"$ref":"` + missing +
				`"},"z":{"$ref":"` + missing + `"}},"examples":[{"$ref":"` + missing + `"}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/a~1b~0c/$ref"},
					{Resource: "", Pointer: "/properties/z/$ref"}}},
			{"array ref", `{"allOf":[true,{"$ref":"` + missing + `"}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/allOf/1/$ref"}}},
			{"transitive resource", `{"$ref":"` + resource + `"}`,
				map[string]JSONDocument{resource: JSONDocument(`{"properties":{"value":{"$ref":"` + missing + `"}}}`)},
				[]SchemaSourceLocation{{Resource: resource, Pointer: "/properties/value/$ref"}}},
			{"relative scoped ref", `{"$id":"https://fixture.invalid/base/","properties":{"value":{"$ref":"missing"}}}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/value/$ref"}}},
			{"multiple missing targets", `{"properties":{"z":{"$ref":"https://fixture.invalid/z"},"a":{"$ref":"https://fixture.invalid/a"}}}`,
				nil, []SchemaSourceLocation{{Resource: "", Pointer: "/properties/a/$ref"},
					{Resource: "", Pointer: "/properties/z/$ref"}}},
		} {
			t.Run(dialect+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				// Arrange: only explicit resources, no loader or diagnostic success escape.
				validator := NewJSONSchemaValidator(tc.resources)
				source := JSONDocument(tc.source)
				before := CloneJSONDocument(source)
				// Act.
				compiled, err := validator.CompileSchema(source, dialect)
				// Assert: original rejection remains, origins are physical source positions.
				require.Nil(t, compiled)
				require.ErrorIs(t, err, ErrSchemaUnsupported)
				failure, ok := errors.AsType[*SchemaCompilationError](err)
				require.True(t, ok)
				require.Equal(t, tc.locations, failure.Locations)
				_, hasOriginalCause := errors.AsType[*jsonschema.LoadURLError](err)
				require.True(t, hasOriginalCause)
				require.Equal(t, before, source)
				require.NotContains(t, err.Error(), missing)
			})
		}
	}
}
