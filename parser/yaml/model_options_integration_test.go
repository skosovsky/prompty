//go:build integration

package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

func TestIntegrationUnmarshal_ModelOptions_Parse(t *testing.T) {
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
