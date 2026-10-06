package yaml

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	yamlv3 "gopkg.in/yaml.v3"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

func TestSchemaNumbersExactAndNonJSONRejected(t *testing.T) {
	t.Parallel()
	for _, value := range []string{".nan", ".inf", "-.inf"} {
		for _, field := range []string{"response_format:\n  type: number\n  minimum: ", "inputs:\n  value:\n    type: number\n    minimum: ", "tools:\n  - name: test\n    parameters:\n      type: number\n      minimum: "} {
			var raw manifest.RawManifest
			err := New().Unmarshal([]byte("id: exact\n"+field+value+"\n"), &raw)
			require.ErrorIs(t, err, prompty.ErrInvalidManifest)
		}
	}
	var raw manifest.RawManifest
	err := New().Unmarshal([]byte("id: exact\nresponse_format:\n  type: number\n  minimum: 9007199254740993\n  maximum: 9007199254740993.1234567890123456789\n"), &raw)
	require.NoError(t, err)
	schema, err := prompty.SchemaMap(raw.ResponseFormat)
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), schema["minimum"])
	require.Equal(t, json.Number("9007199254740993.1234567890123456789"), schema["maximum"])
}

func TestYAMLNumberTokensAvoidResolverOverflow(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ source, expected string }{
		{"1e1000", "1e1000"}, {"1.e2", "1.0e2"}, {"0001.20", "1.20"},
		{"99999999999999999999999999999999999999999999999999999999999999", "99999999999999999999999999999999999999999999999999999999999999"},
	} {
		var value exactMap
		require.NoError(t, yamlv3.Unmarshal([]byte("value: "+test.source), &value))
		require.Equal(t, json.Number(test.expected), value["value"])
	}
	for _, source := range []string{`value: "1e1000"`, `value: !!str 1e1000`} {
		var value exactMap
		require.NoError(t, yamlv3.Unmarshal([]byte(source), &value))
		require.Equal(t, "1e1000", value["value"])
	}
}
