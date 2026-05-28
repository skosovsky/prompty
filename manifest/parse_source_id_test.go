package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestParse_RejectsLegacySourceID(t *testing.T) {
	t.Parallel()
	raw := `{
		"id": "demo",
		"messages": [{
			"role": "system",
			"source_id": "legacy",
			"content": [{"type": "text", "text": "hi"}]
		}]
	}`
	_, err := Parse([]byte(raw), NewJSONParser())
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrLegacyManifestVersion)
}
