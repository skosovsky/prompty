package yaml

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

func TestManifestRetainsSchemaDialects(t *testing.T) {
	t.Parallel()
	// Arrange.
	source := []byte(`id: dialect_fixture
response_format:
  dialect: http://json-schema.org/draft-07/schema#
  schema:
    type: object
tools:
  - name: lookup
    dialect: https://json-schema.org/draft/2020-12/schema
    parameters:
      type: object
`)
	var raw manifest.RawManifest
	// Act.
	err := New().Unmarshal(source, &raw)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, prompty.SchemaDialect7, raw.ResponseFormat.Dialect)
	require.Equal(t, prompty.SchemaDialect2020, raw.Tools[0].Dialect)
}

func TestManifestRejectsNonStringOutputDialect(t *testing.T) {
	t.Parallel()
	// Arrange: a malformed selection must not silently become the default.
	source := []byte("id: dialect_fixture\nresponse_format:\n  dialect: 7\n  schema:\n    type: object\n")
	var raw manifest.RawManifest
	// Act.
	err := New().Unmarshal(source, &raw)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrInvalidManifest)
}
