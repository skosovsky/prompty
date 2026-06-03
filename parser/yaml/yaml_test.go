package yaml

import (
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnmarshal_NormalizedMapTypes verifies that YAML parsing normalizes map[any]any to map[string]any
// for inputs/properties, tools[].Parameters, response_format.Schema, and message metadata.
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
    content: "{{ .Input.query }}"
inputs:
  query:
    type: string
    required: true
  nested:
    type: object
    properties:
      foo:
        type: string
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

	// prompty.MustJSONDocumentAsMap(inputs.Schema)["properties"] must be map[string]any (not map[any]any)
	require.NotNil(t, raw.InputSchema)
	require.NotNil(t, raw.InputSchema.Schema)
	props, ok := prompty.MustJSONDocumentAsMap(raw.InputSchema.Schema)["properties"].(map[string]any)
	require.True(t, ok, "prompty.MustJSONDocumentAsMap(inputs.Schema)[properties] must be map[string]any")
	require.NotNil(t, props)
	assert.Contains(t, props, "query")
	assert.Contains(t, props, "nested")
	nested, ok := props["nested"].(map[string]any)
	require.True(t, ok, "nested property schema must be map[string]any")
	require.NotNil(t, nested["properties"])
	nestedProps, ok := nested["properties"].(map[string]any)
	require.True(t, ok, "nested.properties must be map[string]any")
	assert.Contains(t, nestedProps, "foo")

	// prompty.MustJSONDocumentAsMap(tools[0].Parameters)["properties"] must be map[string]any
	require.Len(t, raw.Tools, 1)
	toolProps, ok := prompty.MustJSONDocumentAsMap(raw.Tools[0].Parameters)["properties"].(map[string]any)
	require.True(t, ok, "prompty.MustJSONDocumentAsMap(tools[0].Parameters)[properties] must be map[string]any")
	assert.Contains(t, toolProps, "arg")

	// prompty.MustJSONDocumentAsMap(response_format.Schema)["properties"] must be map[string]any
	require.NotNil(t, raw.ResponseFormat)
	require.NotNil(t, raw.ResponseFormat.Schema)
	rfProps, ok := prompty.MustJSONDocumentAsMap(raw.ResponseFormat.Schema)["properties"].(map[string]any)
	require.True(t, ok, "prompty.MustJSONDocumentAsMap(response_format.Schema)[properties] must be map[string]any")
	assert.Contains(t, rfProps, "result")

	// message metadata must be map[string]any
	require.Len(t, raw.Messages, 2)
	require.NotNil(t, raw.Messages[0].Metadata)
	metaDoc, err := prompty.MapToJSONDocument(raw.Messages[0].Metadata)
	require.NoError(t, err)
	assert.Equal(t, "val", prompty.MustJSONDocumentAsMap(metaDoc)["custom_key"])
}

// TestUnmarshal_InputsFlatFormatRejected verifies that raw JSON-schema style inputs are rejected in v2.
func TestUnmarshal_InputsFlatFormatRejected(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: flat_schema
version: "1"
messages:
  - role: user
    content: "{{ .Input.query }}"
inputs:
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract-style")
}

func TestUnmarshal_InputsSchemaWrapperRejected(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: wrapper_schema
version: "1"
messages:
  - role: user
    content: "{{ .Input.query }}"
inputs:
  schema:
    type: object
    properties:
      query:
        type: string
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract-style")
}

func TestUnmarshal_ModelOptionsTyped(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_model_opts
version: "1"
model_options:
  model: gpt-4o
  temperature: 0.7
  max_tokens: 2048
  top_p: 0.8
  stop:
    - END
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
	assert.InDelta(
		t,
		0.5,
		prompty.MustJSONDocumentAsMap(raw.ModelOptions.ProviderSettings)["frequency_penalty"].(float64),
		1e-9,
	)
	assert.Equal(t, true, prompty.MustJSONDocumentAsMap(raw.ModelOptions.ProviderSettings)["custom_flag"])
}

func TestUnmarshal_ModelOptionsTyped_RejectsTopLevelVendorKeys(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_model_opts_legacy
version: "1"
model_options:
  model: gpt-4o
  custom_mode: fast
messages:
  - role: system
    content: "Hi"
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid model_options key: custom_mode; use provider_settings")
}

func TestUnmarshal_ModelOptionsEmptyBlockReturnsNil(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: empty_model_opts
version: "1"
model_options: {}
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
model_options:
  model: gemini-2.5-pro
  temperature: 0.3
  top_p: 0.9
  provider_settings:
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
	assert.Equal(
		t,
		map[string]any{"custom_mode": "fast"},
		prompty.MustJSONDocumentAsMap(tpl.ModelOptions.ProviderSettings),
	)

	exec, err := executeTemplatePlan(tpl, map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.NotNil(t, exec.ModelOptions)
	assert.Equal(t, "gemini-2.5-pro", exec.ModelOptions.Model)
	assert.Equal(
		t,
		map[string]any{"custom_mode": "fast"},
		prompty.MustJSONDocumentAsMap(exec.ModelOptions.ProviderSettings),
	)
}

func TestUnmarshal_ContentMediaPart(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_media
version: "1"
messages:
  - role: user
    content:
      - type: text
        text: "Look:"
      - type: media
        media_type: image
        mime_type: image/png
        url: "{{ .Input.img }}"
`)
	tpl, err := manifest.Parse(yamlData, New())
	require.NoError(t, err)
	require.Len(t, tpl.Messages, 1)
	require.Len(t, tpl.Messages[0].Content, 2)
	assert.Equal(t, "media", tpl.Messages[0].Content[1].Type)
	assert.Equal(t, "image", tpl.Messages[0].Content[1].MediaType)
	assert.Equal(t, "image/png", tpl.Messages[0].Content[1].MIMEType)
	assert.Equal(t, "{{ .Input.img }}", tpl.Messages[0].Content[1].URL)

	exec, err := executeTemplatePlan(tpl, map[string]any{"img": "https://example.com/img.png"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	require.Len(t, exec.Messages[0].Content, 2)
	media := exec.Messages[0].Content[1].(prompty.MediaPart)
	assert.Equal(t, "image", media.MediaType)
	assert.Equal(t, "image/png", media.MIMEType)
	assert.Equal(t, "https://example.com/img.png", media.URL)
}

func TestUnmarshal_CacheControl_MessageAndPart(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_cache_control
version: "1"
messages:
  - role: system
    cache_control:
      type: ephemeral
    content:
      - type: text
        text: "Policy"
        cache_control:
          type: ephemeral
  - role: user
    content: "Hi"
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.NoError(t, err)
	require.Len(t, raw.Messages, 2)
	require.NotNil(t, raw.Messages[0].CacheControl)
	assert.Equal(t, "ephemeral", raw.Messages[0].CacheControl.Type)
	require.Len(t, raw.Messages[0].Content, 1)
	require.NotNil(t, raw.Messages[0].Content[0].CacheControl)
	assert.Equal(t, "ephemeral", raw.Messages[0].Content[0].CacheControl.Type)
}

func TestUnmarshal_LegacyCacheFieldRejected(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_legacy_cache
version: "1"
messages:
  - role: system
    cache: true
    content: "Hi"
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field cache not found")
}

func TestUnmarshal_LegacyPartCacheFieldRejected(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_legacy_part_cache
version: "1"
messages:
  - role: user
    content:
      - type: text
        text: "Hi"
        cache: true
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field cache not found")
}

func TestUnmarshal_UnknownContentPartFieldRejected(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: yaml_unknown_part_field
version: "1"
messages:
  - role: user
    content:
      - type: text
        text: "Hi"
        foo: bar
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field foo not found")
}

func TestUnmarshal_RequiredTools(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: doctor_agent
version: "1"
required_tools:
  - doctor_search_knowledge_base
  - get_current_time
messages:
  - role: system
    content: "Hi"
inputs:
  query:
    type: string
`)
	var raw manifest.RawManifest
	require.NoError(t, New().Unmarshal(yamlData, &raw))
	assert.Equal(t, []string{"doctor_search_knowledge_base", "get_current_time"}, raw.RequiredTools)
}

func TestUnmarshal_ContractInputsAndTopLevelLayerKind(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
id: contract_layer
version: "1"
layer_kind: policy
inputs:
  query:
    type: string
    required: true
  tone:
    type: string
    default: strict
messages:
  - role: user
    content: "Q: {{ .Input.query }} ({{ .Input.tone }})"
`)
	var raw manifest.RawManifest
	err := New().Unmarshal(yamlData, &raw)
	require.NoError(t, err)
	tpl, err := manifest.BuildFromRaw(&raw, nil)
	require.NoError(t, err)
	require.Len(t, tpl.Messages, 1)
	assert.Equal(t, prompty.LayerKind("policy"), tpl.Messages[0].LayerKind)
	assert.Equal(t, "strict", tpl.PartialVariables["tone"])
}
