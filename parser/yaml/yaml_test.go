package yaml

import (
	"testing"

	"github.com/skosovsky/prompty/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnmarshal_NormalizedMapTypes verifies that YAML parsing normalizes map[any]any to map[string]any
// for input_schema.Schema, tools[].Parameters, response_format.Schema, and message metadata.
// This regression test ensures gopkg.in/yaml.v3 nested maps work with prompty-gen and manifest BuildFromRaw.
func TestUnmarshal_NormalizedMapTypes(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_norm
version: "1"
messages:
  - role: system
    content: "Hi"
    metadata:
      custom_key: "val"
  - role: user
    content: "{{ .query }}"
input_schema:
  schema:
    type: object
    properties:
      query:
        type: string
      nested:
        type: object
        properties:
          foo:
            type: string
    required:
      - query
tools:
  - name: my_tool
    description: "Tool"
    parameters:
      type: object
      properties:
        arg:
          type: string
response_format:
  name: out
  schema:
    type: object
    properties:
      result:
        type: string
`)
	var raw manifest.RawManifest
	p := New()
	err := p.Unmarshal(yamlData, &raw)
	require.NoError(t, err)

	// input_schema.Schema["properties"] must be map[string]any (not map[any]any)
	require.NotNil(t, raw.InputSchema)
	require.NotNil(t, raw.InputSchema.Schema)
	props, ok := raw.InputSchema.Schema["properties"].(map[string]any)
	require.True(t, ok, "input_schema.Schema[properties] must be map[string]any")
	require.NotNil(t, props)
	assert.Contains(t, props, "query")
	assert.Contains(t, props, "nested")
	nested, ok := props["nested"].(map[string]any)
	require.True(t, ok, "nested property schema must be map[string]any")
	require.NotNil(t, nested["properties"])
	nestedProps, ok := nested["properties"].(map[string]any)
	require.True(t, ok, "nested.properties must be map[string]any")
	assert.Contains(t, nestedProps, "foo")

	// tools[0].Parameters["properties"] must be map[string]any
	require.Len(t, raw.Tools, 1)
	toolProps, ok := raw.Tools[0].Parameters["properties"].(map[string]any)
	require.True(t, ok, "tools[0].Parameters[properties] must be map[string]any")
	assert.Contains(t, toolProps, "arg")

	// response_format.Schema["properties"] must be map[string]any
	require.NotNil(t, raw.ResponseFormat)
	require.NotNil(t, raw.ResponseFormat.Schema)
	rfProps, ok := raw.ResponseFormat.Schema["properties"].(map[string]any)
	require.True(t, ok, "response_format.Schema[properties] must be map[string]any")
	assert.Contains(t, rfProps, "result")

	// message metadata must be map[string]any
	require.Len(t, raw.Messages, 2)
	require.NotNil(t, raw.Messages[0].Metadata)
	assert.Equal(t, "val", raw.Messages[0].Metadata["custom_key"])
}
