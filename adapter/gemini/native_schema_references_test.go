package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestNativeSchemaConsumerFormsOnWire(t *testing.T) {
	t.Parallel()
	fixtures := []struct{ name, schema, answer string }{
		{
			"shared",
			`{"type":"object","$defs":{"value":{"type":"integer","minimum":9007199254740993}},"properties":{"first":{"$ref":"#/$defs/value"},"second":{"$ref":"#/$defs/value"}},"required":["first","second"]}`,
			`{"first":9007199254740993,"second":9007199254740993}`,
		},
		{
			"escaped",
			`{"type":"object","$defs":{"a/b~c":{"type":"number","maximum":1.234567890123456789}},"properties":{"first":{"$ref":"#/$defs/a~1b~0c"}}}`,
			`{"first":1.1}`,
		},
		{
			"encoded",
			`{"type":"object","$defs":{"value":{"type":"integer"}},"properties":{"first":{"$ref":"#/$defs/%76alue"}}}`,
			`{"first":1}`,
		},
		{"property", `{"type":"object","properties":{"$ref":{"type":"string"}}}`, `{"$ref":"data"}`},
		{
			"forms",
			`{"type":"object","properties":{"list":{"type":"array","prefixItems":[{"type":"integer","minimum":9007199254740993},{"type":"number","maximum":1.234567890123456789}],"items":{"anyOf":[{"type":"number","minimum":0.10000000000000001,"maximum":1.234567890123456789},{"type":"null"}]},"minItems":2,"maxItems":3},"date":{"type":["string","null"],"format":"date"}},"propertyOrdering":["date","list"]}`,
			`{"list":[9007199254740993,1.1,null],"date":null}`,
		},
		{
			"large-array-bound",
			`{"type":"object","properties":{"unused":{"type":"array","minItems":9007199254740993,"maxItems":9007199254740994}}}`,
			`{}`,
		},
		{"time", `{"type":"object","properties":{"value":{"type":"string","format":"time"}}}`, `{"value":"12:00:00Z"}`},
		{
			"date-time",
			`{"type":"object","properties":{"value":{"type":"string","format":"date-time"}}}`,
			`{"value":"2026-10-11T12:00:00Z"}`,
		},
	}
	for _, fixture := range fixtures {
		for _, tool := range []bool{false, true} {
			role := "output"
			if tool {
				role = "tool"
			}
			t.Run(fixture.name+"/"+role, func(t *testing.T) {
				t.Parallel()
				// Arrange: checked public API and real GenAI serializer.
				adp, calls, bodies := nativeSchemaFixture(t, fixture.answer)
				exec := prompty.SimplePrompt("fixture")
				if tool {
					exec.Tools = []prompty.ToolDefinition{
						{Name: "lookup", Parameters: prompty.JSONDocument(fixture.schema)},
					}
				} else {
					exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(fixture.schema)}
				}
				before := exec.Clone()
				// Act.
				_, err := adapter.NewClient(adp).Execute(context.Background(), exec)
				// Assert: compare the entire transported schema, preserving numeric lexemes.
				require.NoError(t, err)
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, before, exec)
				var body struct {
					Generation struct {
						Schema json.RawMessage `json:"responseJsonSchema"`
					} `json:"generationConfig"`
					Tools []struct {
						Functions []struct {
							Schema json.RawMessage `json:"parametersJsonSchema"`
						} `json:"functionDeclarations"`
					} `json:"tools"`
				}
				require.NoError(t, json.Unmarshal([]byte(<-bodies), &body))
				wire := body.Generation.Schema
				if tool {
					wire = body.Tools[0].Functions[0].Schema
				}
				original, err := prompty.SchemaDocumentAsMap(prompty.JSONDocument(fixture.schema))
				require.NoError(t, err)
				transported, err := prompty.SchemaDocumentAsMap(wire)
				require.NoError(t, err)
				encodedOriginal, marshalErr := json.Marshal(original)
				require.NoError(t, marshalErr)
				var sdkRepresentation any
				require.NoError(t, json.Unmarshal(encodedOriginal, &sdkRepresentation))
				encodedSDK, marshalErr := json.Marshal(sdkRepresentation)
				require.NoError(t, marshalErr)
				normalized, mapErr := prompty.SchemaDocumentAsMap(encodedSDK)
				require.NoError(t, mapErr)
				require.Equal(t, normalized, transported)
			})
		}
	}
}

func TestNativeSchemaReferenceFailuresBeforeDispatch(t *testing.T) {
	t.Parallel()
	fixtures := []string{
		`{"$defs":{"node":{"$ref":"#/$defs/node"}},"$ref":"#/$defs/node"}`,
		`{"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`,
		`{"$defs":{"node":{"type":"object","properties":{"next":{"$ref":"#/$defs/node"}}}},"$ref":"#/$defs/node"}`,
		`{"$defs":{"node":{"type":"object","properties":{"next":{"anyOf":[{"type":"null"},{"$ref":"#/$defs/node"}]}},"required":["next"]}},"$ref":"#/$defs/node"}`,
		`{"properties":{"parent":{"properties":{"child":{"$ref":"#/properties/parent"}}}}}`,
		`{"$ref":"https://foreign.invalid/schema"}`,
		`{"$ref":"#/missing"}`,
		`{"$ref":"#"}`,
		`{"$ref":"#anchor"}`,
		`{"type":"object","$ref":"#/properties"}`,
		`{"properties":{},"$ref":"#/properties"}`,
		`{"$defs":{"x":{"type":"string"}},"$ref":"#/$defs/x","description":"secret"}`,
		`{"$defs":{"x":{"type":"string"}},"$ref":"#/$defs/x","$id":"secret"}`,
		`{"$defs":{"x~2":{"type":"string"}},"$ref":"#/$defs/x~2"}`,
		`{"$defs":{"x":{"type":"string"}},"$ref":"#/$defs/%zz"}`,
		`{"$defs":{"x":{"type":"string"}},"$ref":"#/$defs/%2578"}`,
		`{"$defs":[]}`,
		`{"$defs":{"x":true}}`,
		`{"$ref":1}`,
	}
	for _, schema := range fixtures {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			adp, calls, _ := nativeSchemaFixture(t, `{}`)
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(schema)}
			// Act: direct admission and checked execution must both fail without I/O.
			_, err := adp.Translate(exec)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			_, err = adapter.NewClient(adp).Execute(context.Background(), exec)
			require.Error(t, err)
			require.Zero(t, calls.Load())
			exec.ResponseFormat = nil
			exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(schema)}}
			_, err = adp.Translate(exec)
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			_, err = adapter.NewClient(adp).Execute(context.Background(), exec)
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}

func TestNativeSchemaKeywordShapes(t *testing.T) {
	t.Parallel()
	for _, schema := range []string{
		`{"anyOf":[]}`, `{"anyOf":{}}`, `{"anyOf":[true]}`, `{"prefixItems":{}}`, `{"prefixItems":[true]}`,
		`{"minItems":-1}`, `{"maxItems":1.5}`, `{"minItems":"2"}`, `{"minItems":true}`,
		`{"format":1}`, `{"format":"uuid"}`, `{"format":"unknown"}`, `{"propertyOrdering":[1]}`, `{"propertyOrdering":{}}`,
		`{"type":[]}`, `{"type":["null","null"]}`, `{"type":["string","integer"]}`, `{"type":["string",{}]}`, `{"type":["unknown","null"]}`,
		`{"$anchor":"value"}`, `{"$dynamicRef":"#/$defs/value"}`,
	} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			adp, calls, _ := nativeSchemaFixture(t, `{}`)
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(schema)}
			// Act.
			_, err := adp.Translate(exec)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			_, err = adapter.NewClient(adp).Execute(context.Background(), exec)
			require.Error(t, err)
			require.Zero(t, calls.Load())
			exec.ResponseFormat = nil
			exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(schema)}}
			_, err = adp.Translate(exec)
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			_, err = adapter.NewClient(adp).Execute(context.Background(), exec)
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}

func TestNativeReferencedOriginalValidation(t *testing.T) {
	t.Parallel()
	// Arrange.
	adp, calls, _ := nativeSchemaFixture(t, `{"value":0}`)
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(
			`{"$defs":{"value":{"type":"integer","minimum":1}},"type":"object","properties":{"value":{"$ref":"#/$defs/value"}},"required":["value"]}`,
		),
	}
	// Act.
	result, err := adapter.NewClient(adp).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
	require.NotNil(t, result)
	require.EqualValues(t, 1, calls.Load())
}

func TestNativeToolRootCannotUseReference(t *testing.T) {
	t.Parallel()
	for _, schema := range []string{`{"$defs":{"root":{"type":"object"}},"$ref":"#/$defs/root"}`, `{"type":["object","null"]}`, `{"type":"array","items":{"type":"string"}}`} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			adp, calls, _ := nativeSchemaFixture(t, `{}`)
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(schema)}}
			// Act.
			_, err := adapter.NewClient(adp).Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			require.Zero(t, calls.Load())
		})
	}
}
