package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutputContract_StaticSchema(t *testing.T) {
	t.Parallel()
	contract := NewStructuredOutputContract(&SchemaDefinition{
		Name:   "answer",
		Schema: MustJSONDocumentFromMap(map[string]any{"type": "object"}),
	})

	format, err := contract.ResponseFormat()
	require.NoError(t, err)
	schema, err := contract.JSONSchema()

	require.NoError(t, err)
	assert.Equal(t, "answer", format.Name)
	assert.JSONEq(t, `{"type":"object"}`, string(schema))
}

func TestStructuredOutputContract_UnavailableSchema(t *testing.T) {
	t.Parallel()
	contract := NewStructuredOutputContract(nil)

	_, err := contract.ResponseFormat()

	require.ErrorIs(t, err, ErrStructuredOutputUnavailable)
}
