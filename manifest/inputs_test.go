package manifest

import (
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeInputs_EmptyMapReturnsEmptyObjectSchema(t *testing.T) {
	t.Parallel()
	schema, err := DecodeInputs(map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, schema)
	require.NotEmpty(t, schema.Schema)

	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	require.NoError(t, err)
	assert.Equal(t, "object", doc["type"])
	props, ok := doc["properties"].(map[string]any)
	require.True(t, ok, "properties must be an object map")
	assert.Empty(t, props)
}

func TestDecodeInputs_LateFlagPreservesMetadata(t *testing.T) {
	t.Parallel()
	schema, err := DecodeInputs(map[string]any{
		"patient_dossier": map[string]any{
			"type": "string",
			"late": true,
		},
		"user_name": map[string]any{
			"type":     "string",
			"required": true,
		},
	})
	require.NoError(t, err)
	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	require.NoError(t, err)
	props, ok := doc["properties"].(map[string]any)
	require.True(t, ok)
	lateProp, ok := props["patient_dossier"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, lateProp["late"])
	assert.Equal(t, true, lateProp["x-prompty-late"])
}

func TestDecodeInputs_LateMustBeBoolean(t *testing.T) {
	t.Parallel()
	_, err := DecodeInputs(map[string]any{
		"bad": map[string]any{"type": "string", "late": "yes"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "late must be boolean")
}

func TestDecodeInputs_NilReturnsNilSchema(t *testing.T) {
	t.Parallel()
	schema, err := DecodeInputs(nil)
	require.NoError(t, err)
	assert.Nil(t, schema)
}
