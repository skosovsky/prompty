package prompty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dualSchemaBase struct {
	ID string `json:"id"`
}

type dualSchemaOptional struct {
	Note string `json:"note"`
}

type dualSchemaDetails struct {
	Active bool `json:"active"`
}

type dualSchemaReflectResult struct {
	dualSchemaBase
	*dualSchemaOptional

	Name    string            `json:"name"`
	Count   int               `json:"count,omitempty"`
	Tags    []string          `json:"tags"`
	Score   *float64          `json:"score"`
	Details dualSchemaDetails `json:"details"`
}

type nilPointerSchemaProvider struct{}

func (*nilPointerSchemaProvider) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	}
}

func TestDualSchemaEngine_Consistency(t *testing.T) {
	t.Parallel()

	reflectSchema := ExtractSchema(dualSchemaReflectResult{})
	fixtureSchema := loadDualSchemaFixture(t)

	require.NotNil(t, reflectSchema)
	require.NotNil(t, fixtureSchema)
	assert.Equal(t, fixtureSchema, reflectSchema)
	assert.Equal(t, false, reflectSchema["additionalProperties"])
	assert.Equal(t, []string{"id", "name", "tags", "details"}, reflectSchema["required"])
}

func TestExtractSchema_EmbeddedStructFlattening(t *testing.T) {
	t.Parallel()

	type base struct {
		ID string `json:"id"`
	}
	type user struct {
		base

		Name string `json:"name"`
	}

	schema := ExtractSchema(user{})
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "id")
	assert.Contains(t, props, "name")
	assert.NotContains(t, props, "base")
}

func TestExtractSchema_EmbeddedStructWithExplicitJSONNameDoesNotFlatten(t *testing.T) {
	t.Parallel()

	type base struct {
		ID string `json:"id"`
	}
	type user struct {
		base `json:"base"`

		Name string `json:"name"`
	}

	schema := ExtractSchema(user{})
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.NotContains(t, props, "id")
	require.Contains(t, props, "base")
	baseSchema := props["base"].(map[string]any)
	assert.Equal(t, false, baseSchema["additionalProperties"])
	assert.Contains(t, baseSchema["properties"].(map[string]any), "id")
}

func TestExtractSchema_TypedNilPointerMatchesValueType(t *testing.T) {
	t.Parallel()

	var ptr *dualSchemaReflectResult
	assert.Equal(t, ExtractSchema(dualSchemaReflectResult{}), ExtractSchema(ptr))
}

func TestExtractSchema_NilPointerSchemaProviderDoesNotPanic(t *testing.T) {
	t.Parallel()

	var ptr *nilPointerSchemaProvider
	require.NotPanics(t, func() {
		schema := ExtractSchema(ptr)
		require.NotNil(t, schema)
		assert.Equal(t, false, schema["additionalProperties"])
	})
}

func TestExtractSchema_RecursiveStructTerminates(t *testing.T) {
	t.Parallel()

	type node struct {
		Value int   `json:"value"`
		Next  *node `json:"next,omitempty"`
	}

	schema := ExtractSchema(node{})
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	next := props["next"].(map[string]any)
	assert.Equal(t, "object", next["type"])
	assert.Equal(t, false, next["additionalProperties"])
	assert.Empty(t, next["properties"])
}

func TestExtractSchema_JSONTagDashCommaUsesLiteralDashName(t *testing.T) {
	t.Parallel()

	type payload struct {
		Value string `json:"-,"` //nolint:staticcheck // literal "-" as JSON key, not omit
	}

	schema := ExtractSchema(payload{})
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "-")
	assert.NotContains(t, props, "Value")
	assert.Equal(t, []string{"-"}, schema["required"])
}

func TestExtractSchema_ByteSliceUsesStringSchema(t *testing.T) {
	t.Parallel()

	type payload struct {
		Data []byte `json:"data"`
	}

	schema := ExtractSchema(payload{})
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "data")
	assert.Equal(t, "string", props["data"].(map[string]any)["type"])
}

func loadDualSchemaFixture(t *testing.T) map[string]any {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("cmd", "prompty-gen", "testdata", "dual_schema_fixture.json"))
	require.NoError(t, err)

	var raw any
	require.NoError(t, json.Unmarshal(data, &raw))

	schema, ok := normalizeDualSchemaFixtureValue(t, raw).(map[string]any)
	require.True(t, ok)
	return schema
}

func normalizeDualSchemaFixtureValue(t *testing.T, value any) any {
	t.Helper()

	switch x := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, item := range x {
			out[key] = normalizeDualSchemaFixtureValue(t, item)
		}
		if required, ok := out["required"].([]any); ok {
			names := make([]string, 0, len(required))
			for _, item := range required {
				name, ok := item.(string)
				require.True(t, ok)
				names = append(names, name)
			}
			out["required"] = names
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeDualSchemaFixtureValue(t, item)
		}
		return out
	default:
		return value
	}
}
