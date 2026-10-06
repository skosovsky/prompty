package prompty

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type schemaMapperFunc func(SchemaMappingRequest) (SchemaMapping, error)

func (f schemaMapperFunc) MapSchema(request SchemaMappingRequest) (SchemaMapping, error) {
	return f(request)
}

func compileFixtureRequest(source string) SchemaCompileRequest {
	return SchemaCompileRequest{
		Source: JSONDocument(source), Role: SchemaOutput,
		Profile:   ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"},
		Validator: NewJSONSchemaValidator(nil),
	}
}

func TestSchemaCompilationStrictPreservesNumbersAndExactDigests(t *testing.T) {
	t.Parallel()
	// Arrange: large integer must not pass through float64.
	request := compileFixtureRequest(`{"type":"integer","const":9007199254740993}`)
	mapper := schemaMapperFunc(func(r SchemaMappingRequest) (SchemaMapping, error) {
		return SchemaMapping{Schema: r.Source}, nil
	})
	// Act.
	compiled, err := CompileProviderSchema(request, mapper)
	// Assert.
	require.NoError(t, err)
	report := compiled.Report()
	require.Equal(t, SchemaStrict, report.Guarantee)
	require.Equal(t, schemaDigest(request.Source), report.SourceDigest)
	require.Equal(t, report.SourceDigest, report.TranslatedDigest)
	require.NoError(t, compiled.ValidateOriginal(JSONDocument(`9007199254740993`)))
	require.ErrorIs(t, compiled.ValidateOriginal(JSONDocument(`9007199254740992`)), ErrSchemaMismatch)
}

func TestSchemaCompilationLoweringRequiresOriginalValidation(t *testing.T) {
	t.Parallel()
	// Arrange: mapped schema drops minimum, but source remains authoritative.
	request := compileFixtureRequest(`{"type":"integer","minimum":10}`)
	request.Policy = SchemaPolicy{Mode: SchemaLowering, AllowedTransforms: []string{"drop-minimum"}}
	mapped := JSONDocument(`{"type":"integer"}`)
	changes := []SchemaTransformation{{ID: "drop-minimum", Pointer: "/minimum", Reason: "fixture mapping limitation"}}
	mapper := schemaMapperFunc(func(r SchemaMappingRequest) (SchemaMapping, error) {
		// Adversarial mutation of received aliases must not change the guard's policy/source.
		r.Source[0] = ' '
		r.Policy.AllowedTransforms[0] = "not-permitted"
		return SchemaMapping{Schema: mapped, Transformations: changes}, nil
	})
	// Act.
	compiled, err := CompileProviderSchema(request, mapper)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, SchemaLowering, compiled.Report().Guarantee)
	require.ErrorIs(t, compiled.ValidateOriginal(JSONDocument(`1`)), ErrSchemaMismatch)
	require.NoError(t, compiled.ValidateOriginal(JSONDocument(`10`)))
	require.NotEqual(t, compiled.Report().SourceDigest, compiled.Report().TranslatedDigest)
	require.Equal(t, "drop-minimum", request.Policy.AllowedTransforms[0])
	require.Equal(t, byte('{'), request.Source[0])
	// Mutating mapper result or getters must not mutate the compiled snapshot.
	mapped[0] = ' '
	changes[0].ID = "changed"
	compiled.Source()[0] = ' '
	compiled.Translated()[0] = ' '
	compiled.Report().Transformations[0].ID = "changed"
	require.Equal(t, byte('{'), compiled.Source()[0])
	require.Equal(t, byte('{'), compiled.Translated()[0])
	require.Equal(t, "drop-minimum", compiled.Report().Transformations[0].ID)
}

func TestSchemaCompilationRejectsUnpermittedOrUnreportedChanges(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		policy  SchemaPolicy
		changes []SchemaTransformation
	}{
		{"zero strict", SchemaPolicy{}, nil},
		{"strict report", SchemaPolicy{}, []SchemaTransformation{{ID: "drop", Pointer: "/minimum", Reason: "drop"}}},
		{"missing report", SchemaPolicy{Mode: SchemaLowering, AllowedTransforms: []string{"drop"}}, nil},
		{"not permitted", SchemaPolicy{Mode: SchemaLowering}, []SchemaTransformation{{ID: "drop", Pointer: "/minimum", Reason: "drop"}}},
		{"bad pointer", SchemaPolicy{Mode: SchemaLowering, AllowedTransforms: []string{"drop"}}, []SchemaTransformation{{ID: "drop", Pointer: "/a~2b", Reason: "drop"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			request := compileFixtureRequest(`{"type":"integer","minimum":10}`)
			request.Policy = tt.policy
			mapper := schemaMapperFunc(func(SchemaMappingRequest) (SchemaMapping, error) {
				return SchemaMapping{Schema: JSONDocument(`{"type":"integer"}`), Transformations: tt.changes}, nil
			})
			// Act.
			compiled, err := CompileProviderSchema(request, mapper)
			// Assert.
			require.Nil(t, compiled)
			require.ErrorIs(t, err, ErrSchemaUnsupported)
		})
	}
}

func TestSchemaCompilationCannotChangeLargeNumericConstraintInStrictMode(t *testing.T) {
	t.Parallel()
	// Arrange.
	request := compileFixtureRequest(`{"const":9007199254740993}`)
	mapper := schemaMapperFunc(func(SchemaMappingRequest) (SchemaMapping, error) {
		return SchemaMapping{Schema: JSONDocument(`{"const":9007199254740992}`)}, nil
	})
	// Act.
	compiled, err := CompileProviderSchema(request, mapper)
	// Assert.
	require.Nil(t, compiled)
	require.ErrorIs(t, err, ErrSchemaUnsupported)
}

func TestSchemaCompilationMissingValidatorAndUnresolvedReferenceFailBeforeMapper(t *testing.T) {
	t.Parallel()
	for _, missingValidator := range []bool{true, false} {
		// Arrange.
		request := compileFixtureRequest(`{"$ref":"https://unprovided.invalid/schema"}`)
		if missingValidator {
			request.Validator = nil
		}
		calls := 0
		mapper := schemaMapperFunc(func(SchemaMappingRequest) (SchemaMapping, error) {
			calls++
			return SchemaMapping{}, nil
		})
		// Act.
		compiled, err := CompileProviderSchema(request, mapper)
		// Assert.
		require.Error(t, err)
		require.Nil(t, compiled)
		require.Zero(t, calls)
	}
}

func TestLoweredNullablePropertyStillUsesOriginalContract(t *testing.T) {
	t.Parallel()
	// Arrange: optional non-null source is not equivalent to required nullable output.
	request := compileFixtureRequest(`{"type":"object","properties":{"x":{"type":"integer"}}}`)
	request.Policy = SchemaPolicy{Mode: SchemaLowering, AllowedTransforms: []string{"required-nullable"}}
	mapper := schemaMapperFunc(func(SchemaMappingRequest) (SchemaMapping, error) {
		return SchemaMapping{
			Schema: JSONDocument(`{"type":"object","properties":{"x":{"type":["integer","null"]}},"required":["x"]}`),
			Transformations: []SchemaTransformation{{
				ID: "required-nullable", Pointer: "/properties/x", Reason: "fixture required nullable mapping",
			}},
		}, nil
	})
	// Act.
	compiled, err := CompileProviderSchema(request, mapper)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, SchemaLowering, compiled.Report().Guarantee)
	require.ErrorIs(t, compiled.ValidateOriginal(JSONDocument(`{"x":null}`)), ErrSchemaMismatch)
	require.NoError(t, compiled.ValidateOriginal(JSONDocument(`{}`)))
	require.NoError(t, compiled.ValidateOriginal(JSONDocument(`{"x":1}`)))
}

func TestSchemaCompilationInvalidPolicyStopsBeforeMapper(t *testing.T) {
	t.Parallel()
	for _, policy := range []SchemaPolicy{
		{Mode: "best-effort"},
		{Mode: SchemaStrict, AllowedTransforms: []string{"drop"}},
		{Mode: SchemaLowering, AllowedTransforms: []string{"drop", "drop"}},
		{Mode: SchemaLowering, AllowedTransforms: []string{" "}},
	} {
		// Arrange.
		request := compileFixtureRequest(`{}`)
		request.Policy = policy
		calls := 0
		mapper := schemaMapperFunc(func(SchemaMappingRequest) (SchemaMapping, error) {
			calls++
			return SchemaMapping{}, nil
		})
		// Act.
		compiled, err := CompileProviderSchema(request, mapper)
		// Assert.
		require.Nil(t, compiled)
		require.ErrorIs(t, err, ErrSchemaUnsupported)
		require.Zero(t, calls)
	}
}
