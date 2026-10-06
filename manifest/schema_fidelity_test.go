package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestCompositionRejectsLossOfFieldConstraints(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{
		{`{"type":"string","maxLength":100}`, `{"type":"string","maxLength":3}`},
		{`{"type":"string","enum":["a","b"]}`, `{"type":"string","enum":["a"]}`},
		{`{"type":"object","properties":{"name":{"maxLength":100}}}`, `{"type":"object","properties":{"name":{"maxLength":3}}}`},
		{`{"type":"integer","const":9007199254740992}`, `{"type":"integer","const":9007199254740993}`},
	} {
		local := &prompty.SchemaDefinition{
			Schema: prompty.JSONDocument(`{"type":"object","properties":{"value":` + pair[0] + `}}`),
		}
		child := &RawManifest{
			InputSchema: &prompty.SchemaDefinition{
				Schema: prompty.JSONDocument(`{"type":"object","properties":{"value":` + pair[1] + `}}`),
			},
		}
		_, err := MergeInputSchemas(local, child)
		require.Error(t, err)
	}
}

func TestCompositionExactNumericEqualityAndRequiredUnion(t *testing.T) {
	t.Parallel()
	local := &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(
			`{"type":"object","properties":{"value":{"type":"integer","const":1}},"required":["value"]}`,
		),
	}
	child := &RawManifest{
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.JSONDocument(
				`{"type":"object","properties":{"value":{"const":1e0,"type":"integer"},"other":{"type":"boolean"}},"required":["other"]}`,
			),
		},
	}
	merged, err := MergeInputSchemas(local, child)
	require.NoError(t, err)
	require.NoError(t, prompty.ValidateSchemaJSON(merged, prompty.JSONDocument(`{"value":1.0,"other":false}`)))
	require.ErrorIs(
		t,
		prompty.ValidateSchemaJSON(merged, prompty.JSONDocument(`{"value":1}`)),
		prompty.ErrSchemaMismatch,
	)
}
