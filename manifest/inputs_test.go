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

func TestDecodeInputs_NilReturnsNilSchema(t *testing.T) {
	t.Parallel()
	schema, err := DecodeInputs(nil)
	require.NoError(t, err)
	assert.Nil(t, schema)
}
