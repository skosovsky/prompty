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

// TestUnmarshal_InputSchema_FlatFormat verifies that flat-format input_schema (type/properties at top level)
// is correctly parsed and wrapped into SchemaDefinition.Schema, so prompty-gen receives non-empty properties.
func TestUnmarshal_InputSchema_FlatFormat(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: flat_schema
version: "1"
messages:
  - role: user
    content: "{{ .query }}"
input_schema:
  type: object
  properties:
    current_doctor_time:
      type: string
    timezone:
      type: string
    chat_history:
      type: array
      items:
        type: string
  required:
    - current_doctor_time
response_format:
  type: object
  properties:
    result:
      type: string
`)
	var raw manifest.RawManifest
	p := New()
	err := p.Unmarshal(yamlData, &raw)
	require.NoError(t, err)

	// Flat format: whole input_schema block is the JSON schema, wrapped in SchemaDefinition.Schema
	require.NotNil(t, raw.InputSchema)
	require.NotNil(t, raw.InputSchema.Schema)
	assert.Equal(t, "object", raw.InputSchema.Schema["type"])
	props, ok := raw.InputSchema.Schema["properties"].(map[string]any)
	require.True(t, ok, "input_schema.Schema[properties] must be map[string]any in flat format")
	require.NotNil(t, props)
	assert.Contains(t, props, "current_doctor_time")
	assert.Contains(t, props, "timezone")
	assert.Contains(t, props, "chat_history")
	required, ok := raw.InputSchema.Schema["required"].([]any)
	require.True(t, ok)
	require.Len(t, required, 1)
	assert.Equal(t, "current_doctor_time", required[0])

	// response_format flat format
	require.NotNil(t, raw.ResponseFormat)
	require.NotNil(t, raw.ResponseFormat.Schema)
	rfProps, ok := raw.ResponseFormat.Schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, rfProps, "result")
}

func TestUnmarshal_ModelOptionsTyped(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_model_opts
version: "1"
model_config:
  model: gpt-4o
  temperature: 0.7
  max_tokens: 2048
  top_p: 0.8
  stop:
    - END
  frequency_penalty: 0.2
  provider_settings:
    frequency_penalty: 0.5
    custom_flag: true
messages:
  - role: system
    content: "Hi"
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.NoError(t, err)
	require.NotNil(t, raw.ModelOptions)
	assert.Equal(t, "gpt-4o", raw.ModelOptions.Model)
	require.NotNil(t, raw.ModelOptions.Temperature)
	assert.InDelta(t, 0.7, *raw.ModelOptions.Temperature, 1e-9)
	require.NotNil(t, raw.ModelOptions.MaxTokens)
	assert.Equal(t, int64(2048), *raw.ModelOptions.MaxTokens)
	require.NotNil(t, raw.ModelOptions.TopP)
	assert.InDelta(t, 0.8, *raw.ModelOptions.TopP, 1e-9)
	assert.Equal(t, []string{"END"}, raw.ModelOptions.Stop)
	require.NotNil(t, raw.ModelOptions.ProviderSettings)
	assert.InDelta(t, 0.5, raw.ModelOptions.ProviderSettings["frequency_penalty"].(float64), 1e-9)
	assert.Equal(t, true, raw.ModelOptions.ProviderSettings["custom_flag"])
}

func TestUnmarshal_ModelOptionsEmptyBlockReturnsNil(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: empty_model_opts
version: "1"
model_config: {}
messages:
  - role: system
    content: "Hi"
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.NoError(t, err)
	assert.Nil(t, raw.ModelOptions)
}

func TestUnmarshal_ModelOptions_ParseIntegration(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_parse_model_opts
version: "1"
model_config:
  model: gemini-2.5-pro
  temperature: 0.3
  top_p: 0.9
  custom_mode: fast
messages:
  - role: system
    content: "Hi"
`)
	tpl, err := manifest.Parse(yamlData, New())
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.NotNil(t, tpl.ModelOptions)
	assert.Equal(t, "gemini-2.5-pro", tpl.ModelOptions.Model)
	require.NotNil(t, tpl.ModelOptions.Temperature)
	assert.InDelta(t, 0.3, *tpl.ModelOptions.Temperature, 1e-9)
	require.NotNil(t, tpl.ModelOptions.TopP)
	assert.InDelta(t, 0.9, *tpl.ModelOptions.TopP, 1e-9)
	assert.Equal(t, map[string]any{"custom_mode": "fast"}, tpl.ModelOptions.ProviderSettings)

	exec, err := tpl.Format(map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.NotNil(t, exec.ModelOptions)
	assert.Equal(t, "gemini-2.5-pro", exec.ModelOptions.Model)
	assert.Equal(t, map[string]any{"custom_mode": "fast"}, exec.ModelOptions.ProviderSettings)
}
