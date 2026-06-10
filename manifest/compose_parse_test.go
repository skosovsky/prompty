package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestParseJSON_ComposedManifestRoundTrip(t *testing.T) {
	t.Parallel()
	data := []byte(`{
  "id": "main",
  "inputs": {"q": {"type": "string"}},
  "imports": [{"id": "child"}],
  "layers": [
    {"id": "base", "role": "system", "content": [{"type": "text", "text": "hi"}]},
    {"id": "imp", "import_ref": "child"}
  ]
}`)
	var raw RawManifest
	require.NoError(t, NewJSONParser().Unmarshal(data, &raw))
	require.Len(t, raw.Imports, 1)
	require.Len(t, raw.Layers, 2)
}

func TestBuildFromRaw_ComposedRequiresLoader(t *testing.T) {
	t.Parallel()
	raw := &RawManifest{
		ID: "main",
		Layers: []RawLayer{{
			ID: "base", Role: "system",
			Content: []RawContentPart{{Type: "text", Text: "hi"}},
		}},
		InputSchema: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{
				"type": "object", "properties": map[string]any{},
			}),
		},
	}
	_, err := BuildFromRaw(raw, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compose loader")
}
