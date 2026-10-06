package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestNullableReferenceLoweringRetainsOriginalNumericEnum(t *testing.T) {
	t.Parallel()
	// Arrange: the optional property refers to an exact numeric enum, not a nullable type union.
	source := prompty.JSONDocument(`{"type":"object","properties":{"x":{"$ref":"#/$defs/value"}},` +
		`"additionalProperties":false,"$defs":{"value":{"type":"integer","enum":[9007199254740993]}}}`)
	original := prompty.CloneJSONDocument(source)
	request := prompty.SchemaCompileRequest{
		Source:  source,
		Role:    prompty.SchemaOutput,
		Profile: prompty.ProfileIdentity{Provider: "openai", Endpoint: "fixture", Model: "fixture"},
		Policy: prompty.SchemaPolicy{
			Mode:              prompty.SchemaLowering,
			AllowedTransforms: []string{LoweringRequiredNullable},
		},
		Validator: prompty.NewJSONSchemaValidator(nil),
	}
	// Act.
	compiled, err := prompty.CompileProviderSchema(request, New())
	// Assert: the wrapper preserves the ref and original missing/null/number distinctions.
	require.NoError(t, err)
	require.Equal(t, original, source)
	require.Equal(t, original, compiled.Source())
	require.Len(t, compiled.Report().Transformations, 1)
	require.Equal(t, "/properties/x", compiled.Report().Transformations[0].Pointer)
	require.NoError(t, compiled.ValidateOriginal(prompty.JSONDocument(`{}`)))
	require.NoError(t, compiled.ValidateOriginal(prompty.JSONDocument(`{"x":9007199254740993}`)))
	require.ErrorIs(t, compiled.ValidateOriginal(prompty.JSONDocument(`{"x":null}`)), prompty.ErrSchemaMismatch)
	require.ErrorIs(
		t,
		compiled.ValidateOriginal(prompty.JSONDocument(`{"x":9007199254740992}`)),
		prompty.ErrSchemaMismatch,
	)
	translated, err := request.Validator.CompileSchema(compiled.Translated(), "")
	require.NoError(t, err)
	require.NoError(t, translated.ValidateJSON(prompty.JSONDocument(`{"x":null}`)))
	require.ErrorIs(t, translated.ValidateJSON(prompty.JSONDocument(`{}`)), prompty.ErrSchemaMismatch)

	// Act: use the actual SDK translation/serialization, not a handwritten wire map.
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: compiled.Translated()}
	sdkRequest, err := New().Translate(exec)
	require.NoError(t, err)
	encoded, err := json.Marshal(sdkRequest)
	// Assert: serialization retains the definition/reference and exact enum number.
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"$ref":"#/$defs/value"`)
	require.Contains(t, string(encoded), `"enum":[9007199254740993]`)
	require.NotContains(t, string(encoded), `9007199254740992`)
	require.Equal(t, original, source)
}
