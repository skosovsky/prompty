package prompts

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/stretchr/testify/require"
)

func TestGeneratedSchemaValidationPresenceAndFractionalBounds(t *testing.T) {
	t.Parallel()
	for _, document := range []string{
		`{"amount":3.4,"count":0,"enabled":false,"text":""}`,
		`{"amount":0,"count":0,"enabled":false,"text":""}`,
		`{"amount":3.400000000000000000001,"count":9007199254740993,"enabled":false,"text":""}`,
		`{"amount":-9007199254740993,"count":1e0,"enabled":false,"text":""}`,
		`{"amount":3.4,"count":0,"enabled":false,"text":"","payload":{"count":0}}`,
	} {
		var input SchemaFidelityInput
		require.NoError(t, json.Unmarshal([]byte(document), &input))
		require.NoError(t, input.ValidateSchema())
		recipe, err := NewPromptCatalog(registryStub{}).NewSchemaFidelityRecipe(context.Background(), input)
		require.NoError(t, err)
		late := SchemaFidelityLateInput{LateAmount: json.Number("3.4")}
		recipe, err = recipe.BindLate(late)
		require.NoError(t, err)
		checkpoint, err := recipe.CheckpointJSON()
		require.NoError(t, err)
		_, ok, err := NewPromptIndex(registryStub{}).DecodeRecipeCheckpoint(SchemaFidelity, checkpoint)
		require.True(t, ok)
		require.NoError(t, err)
	}
	for _, document := range []string{
		`{"amount":3.6,"count":0,"enabled":false,"text":""}`,
		`{"amount":3.500000000000000000001,"count":0,"enabled":false,"text":""}`,
		`{"amount":3.4,"enabled":false,"text":""}`,
		`{"amount":3.4,"count":0,"text":""}`,
		`{"amount":3.4,"count":0,"enabled":false}`,
		`{"amount":3.4,"count":0,"enabled":false,"text":"","payload":{}}`,
	} {
		var input SchemaFidelityInput
		require.ErrorIs(t, json.Unmarshal([]byte(document), &input), prompty.ErrSchemaMismatch)
	}
	var late SchemaFidelityLateInput
	require.ErrorIs(t, json.Unmarshal([]byte(`{"late_amount":3.6}`), &late), prompty.ErrSchemaMismatch)
	require.ErrorIs(t, json.Unmarshal([]byte(`{}`), &late), prompty.ErrSchemaMismatch)
}
