package prompty

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingFragmentAndAnchorOriginsUseReachableEngineEdges(t *testing.T) {
	t.Parallel()
	const resource = "https://fixture.invalid/resource"
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		for _, tc := range []struct {
			name, source string
			resources    map[string]JSONDocument
			locations    []SchemaSourceLocation
		}{
			{"local fragment", `{"$ref":"#/missing"}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/$ref"}}},
			{"ref-looking enum data", `{"properties":{"a":{"$ref":"#/missing"}},"enum":[{"$ref":"first"},{"$ref":"second"}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/a/$ref"}}},
			{"non-schema examples data", `{"properties":{"a":{"$ref":"#/missing"}},"examples":[{"$ref":"first","type":"not-a-schema-type"}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/a/$ref"}}},
			{"non-schema enum data", `{"properties":{"a":{"$ref":"#/missing"}},"enum":[{"$ref":"first","type":42}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/a/$ref"}}},
			{"ref-looking assertion and annotation data", `{"properties":{"a":{"$ref":"#/missing"}},"const":{"$ref":"first"},"default":{"$ref":"first"},"examples":[{"$ref":"first"},{"$ref":"second"}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/a/$ref"}}},
			{"local anchor", `{"properties":{"a/b~c":{"$ref":"#absent"}}}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/a~1b~0c/$ref"}}},
			{"URI escaped position", `{"properties":{"hash# and\nvalue":{"$ref":"#/missing"}}}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/hash# and\nvalue/$ref"}}},
			{"missing resource fragment", `{"$ref":"https://fixture.invalid/absent#/missing"}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/$ref"}}},
			{"missing resource anchor", `{"$ref":"https://fixture.invalid/absent#missing"}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/$ref"}}},
			{"explicit resource fragment", `{"$ref":"` + resource + `#/missing"}`,
				map[string]JSONDocument{resource: JSONDocument(`{"type":"object"}`)},
				[]SchemaSourceLocation{{Resource: "", Pointer: "/$ref"}}},
			{"transitive resource anchor", `{"$ref":"` + resource + `"}`,
				map[string]JSONDocument{resource: JSONDocument(`{"properties":{"value":{"$ref":"#missing"}}}`)},
				[]SchemaSourceLocation{{Resource: resource, Pointer: "/properties/value/$ref"}}},
			{"referenced extension schema", `{"$ref":"#/custom","custom":{"$ref":"#/missing"}}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/custom/$ref"}}},
			{"annotation and unreachable definitions", `{"properties":{"value":{"$ref":"#/missing"}},"definitions":{"unused":{"$ref":"#/another"}},"examples":[{"$ref":"#/annotation"}],"default":{"$ref":"#/default"}}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/value/$ref"}}},
			{"scoped id", `{"properties":{"value":{"$id":"https://fixture.invalid/nested","properties":{"p":{"$ref":"#/missing"}}}}}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/value/properties/p/$ref"}}},
			{"mixed missing edges", `{"allOf":[{"$ref":"https://fixture.invalid/resource-missing"},{"$ref":"#/missing"}]}`, nil,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/allOf/0/$ref"}, {Resource: "", Pointer: "/allOf/1/$ref"}}},
		} {
			t.Run(dialect+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				// Arrange: caller source/resources remain the actual rejected contract.
				source := JSONDocument(tc.source)
				before := CloneJSONDocument(source)
				validator := NewJSONSchemaValidator(tc.resources)
				// Act.
				compiled, err := validator.CompileSchema(source, dialect)
				// Assert: only engine-reachable failing ref origins, never target-as-origin.
				require.Nil(t, compiled)
				require.ErrorIs(t, err, ErrSchemaUnsupported)
				failure, ok := errors.AsType[*SchemaCompilationError](err)
				require.True(t, ok)
				require.Equal(t, tc.locations, failure.Locations)
				require.Error(t, failure.Cause)
				require.Equal(t, before, source)
				for name, value := range tc.resources {
					require.Equal(t, value, validator.resources[name])
				}
			})
		}
	}
}

func TestReferenceOriginProbeRetainsDialectAndDynamicRefSemantics(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()
			// Arrange: draft7 ignores ref siblings; 2020 evaluates them.
			source := JSONDocument(`{"$ref":"#/missing","properties":{"p":{"$dynamicRef":"#absent"}}}`)
			wanted := []SchemaSourceLocation{{Resource: "", Pointer: "/$ref"}}
			if dialect == SchemaDialect2020 {
				wanted = append(wanted, SchemaSourceLocation{Resource: "", Pointer: "/properties/p/$dynamicRef"})
			}
			// Act: random private placeholder identity must not affect diagnostics.
			for range 3 {
				compiled, err := NewJSONSchemaValidator(nil).CompileSchema(source, dialect)
				// Assert: original rejection and deterministic active source positions only.
				require.Nil(t, compiled)
				failure, ok := errors.AsType[*SchemaCompilationError](err)
				require.True(t, ok)
				require.Equal(t, wanted, failure.Locations)
			}
		})
	}
}
