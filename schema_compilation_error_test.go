package prompty

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestSchemaCompilationLocations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, pointer string
	}{
		{"escaped property", `{"properties":{"a/b~c":{"minimum":"private-value"}}}`, "/properties/a~1b~0c/minimum"},
		{"array position", `{"allOf":[true,{"type":42}]}`, "/allOf/1/type"},
		{"root", `[]`, ""},
		{"referenced extension subtree", `{"$ref":"#/custom","custom":{"minimum":"bad"}}`, "/custom/minimum"},
		{"declared dialect", `{"$schema":"private-dialect"}`, "/$schema"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			validator := NewJSONSchemaValidator(nil)
			// Act.
			compiled, err := validator.CompileSchema(JSONDocument(tc.source), "")
			// Assert.
			require.Nil(t, compiled)
			require.ErrorIs(t, err, ErrSchemaUnsupported)
			failure, ok := errors.AsType[*SchemaCompilationError](err)
			require.True(t, ok)
			require.Contains(t, failure.Locations, SchemaSourceLocation{Resource: "", Pointer: tc.pointer})
			require.NotContains(t, failure.Error(), "private")
			if tc.name != "declared dialect" {
				_, hasCause := errors.AsType[*jsonschema.SchemaValidationError](err)
				require.True(t, hasCause)
			}
		})
	}
}

func TestSchemaCompilationResourceLocations(t *testing.T) {
	t.Parallel()
	// Arrange.
	const resource = "https://fixture.invalid/resource"
	validator := NewJSONSchemaValidator(map[string]JSONDocument{
		resource: JSONDocument(`{"properties":{"value":{"minimum":"invalid"}}}`),
	})
	// Act.
	compiled, err := validator.CompileSchema(JSONDocument(`{"$ref":"`+resource+`"}`), "")
	// Assert: the failing schema is the resource, not the referencing source or metaschema.
	require.Nil(t, compiled)
	failure, ok := errors.AsType[*SchemaCompilationError](err)
	require.True(t, ok)
	require.Equal(
		t,
		[]SchemaSourceLocation{{Resource: resource, Pointer: "/properties/value/minimum"}},
		failure.Locations,
	)
}

func TestSchemaCompilationUnknownLocationIsNotRoot(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`{"type":`, `{`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			validator := NewJSONSchemaValidator(nil)
			// Act.
			compiled, err := validator.CompileSchema(JSONDocument(source), "")
			// Assert.
			require.Nil(t, compiled)
			require.ErrorIs(t, err, ErrSchemaUnsupported)
			failure, ok := errors.AsType[*SchemaCompilationError](err)
			require.True(t, ok)
			require.Empty(t, failure.Locations)
			require.Error(t, failure.Cause)
		})
	}
}

func TestSchemaCompilationLocationOrdering(t *testing.T) {
	t.Parallel()
	// Arrange.
	validator := NewJSONSchemaValidator(nil)
	source := JSONDocument(`{"properties":{"z":{"minimum":"bad"},"a":{"maximum":"bad"}}}`)
	// Act.
	_, err := validator.CompileSchema(source, "")
	// Assert.
	failure, ok := errors.AsType[*SchemaCompilationError](err)
	require.True(t, ok)
	require.Equal(t, []SchemaSourceLocation{
		{Resource: "", Pointer: "/properties/a/maximum"},
		{Resource: "", Pointer: "/properties/z/minimum"},
	}, failure.Locations)
}

func TestSchemaCompilationLocationsStopBeforeMapper(t *testing.T) {
	t.Parallel()
	for _, role := range []SchemaRole{SchemaOutput, SchemaToolInput} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			request := compileFixtureRequest(`{"properties":{"value":{"minimum":"bad"}}}`)
			request.Role = role
			source := CloneJSONDocument(request.Source)
			calls := 0
			mapper := schemaMapperFunc(func(r SchemaMappingRequest) (SchemaMapping, error) {
				calls++
				return SchemaMapping{Schema: r.Source}, nil
			})
			// Act.
			compiled, err := CompileProviderSchema(request, mapper)
			// Assert.
			require.Nil(t, compiled)
			require.Zero(t, calls)
			require.Equal(t, source, request.Source)
			failure, ok := errors.AsType[*SchemaCompilationError](err)
			require.True(t, ok)
			require.Equal(
				t,
				[]SchemaSourceLocation{{Resource: "", Pointer: "/properties/value/minimum"}},
				failure.Locations,
			)
		})
	}
}

func TestSchemaCompilationDecoderCausePreserved(t *testing.T) {
	t.Parallel()
	// Arrange.
	validator := NewJSONSchemaValidator(nil)
	// Act.
	_, err := validator.CompileSchema(JSONDocument(`{"type":?}`), "")
	// Assert.
	require.ErrorIs(t, err, ErrSchemaUnsupported)
	failure, ok := errors.AsType[*SchemaCompilationError](err)
	require.True(t, ok)
	require.Empty(t, failure.Locations)
	require.Error(t, failure.Cause)
	_, hasSyntaxCause := errors.AsType[*json.SyntaxError](err)
	require.True(t, hasSyntaxCause)
}

func TestSchemaCompilationResourceRegistrationOrder(t *testing.T) {
	t.Parallel()
	// Arrange: both resources are malformed; failure must not depend on map iteration.
	validator := NewJSONSchemaValidator(map[string]JSONDocument{
		"https://fixture.invalid/a": JSONDocument(`?`),
		"https://fixture.invalid/z": JSONDocument(`{"type":?}`),
	})
	for range 16 {
		// Act.
		compiled, err := validator.CompileSchema(JSONDocument(`true`), "")
		// Assert.
		require.Nil(t, compiled)
		syntax, ok := errors.AsType[*json.SyntaxError](err)
		require.True(t, ok)
		require.Equal(t, int64(1), syntax.Offset)
	}
}

func TestSchemaCompilationDraft7Locations(t *testing.T) {
	t.Parallel()
	// Arrange: tuple items are a valid schema location in the explicitly selected dialect.
	validator := NewJSONSchemaValidator(nil)
	source := JSONDocument(`{"items":[true,{"minimum":"bad"}]}`)
	// Act.
	compiled, err := validator.CompileSchema(source, SchemaDialect7)
	// Assert.
	require.Nil(t, compiled)
	failure, ok := errors.AsType[*SchemaCompilationError](err)
	require.True(t, ok)
	require.Contains(t, failure.Locations, SchemaSourceLocation{Resource: "", Pointer: "/items/1/minimum"})
}
