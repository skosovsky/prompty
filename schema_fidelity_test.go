package prompty

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type malformedSchemaDTO struct {
	Count int `json:"count"`
}

func (malformedSchemaDTO) JSONSchema() JSONDocument { return JSONDocument(`{"type":`) }

func TestExplicitMalformedSchemaNeverFallsBack(t *testing.T) {
	t.Parallel()
	_, err := ExtractSchema(malformedSchemaDTO{})
	require.Error(t, err)
	_, err = NewTypedTool("count", func(_ context.Context, _ malformedSchemaDTO) (string, error) { return "", nil })
	require.Error(t, err)
	template, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent("fixture")}})
	require.NoError(t, err)
	plan, err := WithResponseFormatFromStruct[malformedSchemaDTO](NewRenderPlan(template))
	require.Error(t, err)
	require.Nil(t, plan)
}

func TestReflectionMatchesJSONPresenceAndQuotedScalars(t *testing.T) {
	t.Parallel()
	type embedded struct {
		Name string `json:"name"`
	}
	type payload struct {
		embedded

		Count    int      `json:"count,string"`
		Enabled  bool     `json:"enabled,string"`
		Text     string   `json:"text,string"`
		Optional *int     `json:"optional,omitempty"`
		Nullable *int     `json:"nullable"`
		Zero     int      `json:"zero,omitzero"`
		Items    []string `json:"items"`
	}
	schema, err := ExtractSchema(payload{})
	require.NoError(t, err)
	definition := &SchemaDefinition{Schema: MustJSONDocumentFromMap(schema)}
	for _, value := range []payload{{}, {Count: 42, Enabled: true, Text: "\"\n\\", Items: []string{"x"}}} {
		document, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, ValidateSchemaJSON(definition, document))
	}
}

func TestRuntimeDynamicNumbersRemainExact(t *testing.T) {
	t.Parallel()
	reg := &exactRuntimeRegistry{recipeTestRegistry: *newRecipeTestRegistry(t)}
	checkpoint := RuntimeRecipeCheckpoint{
		Descriptor: reg.desc,
		Input:      JSONDocument(`{"query":9007199254740993}`),
		LateInput:  JSONDocument(`{"suffix":[-9007199254740993,{"decimal":0.123456789012345678901}]}`),
		State:      RuntimeRecipeStateLateBound,
	}
	plan, err := BindRuntime(context.Background(), reg, checkpoint, RuntimeOverlay{}, ToolScope{})
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(
		t,
		"9007199254740993 [-9007199254740993 map[decimal:0.123456789012345678901]]",
		mustTextFromParts(t, execution.Messages[0].Content),
	)
	overlay := RuntimeOverlay{LateInput: JSONDocument(`{"suffix":1.000000000000000000001e+5}`)}
	plan, err = BindRuntime(context.Background(), reg, checkpoint, overlay, ToolScope{})
	require.NoError(t, err)
	execution, err = plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, "9007199254740993 1.000000000000000000001e+5", mustTextFromParts(t, execution.Messages[0].Content))
}

func TestRenderValidatesCombinedOriginalInputSchema(t *testing.T) {
	t.Parallel()
	definition := &SchemaDefinition{
		Schema: JSONDocument(
			`{"type":"object","properties":{"early":{"type":"integer"},"late":{"type":"number","maximum":3.5,"late":true}},"required":["early","late"]}`,
		),
	}
	template, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleUser, Content: TextContent("{{ .Input.early }} {{ .LateVars.late }}")}},
		WithInputSchema(definition),
	)
	require.NoError(t, err)
	plan := newRenderPlanFromMap(template, map[string]any{"early": json.Number("9007199254740993")})
	plan, err = plan.WithLateInputJSON(JSONDocument(`{"late":3.4}`))
	require.NoError(t, err)
	_, err = plan.Execute(context.Background())
	require.NoError(t, err)
	plan, err = plan.WithLateInputJSON(JSONDocument(`{"late":3.6}`))
	require.NoError(t, err)
	_, err = plan.Execute(context.Background())
	require.ErrorIs(t, err, ErrSchemaMismatch)
}

func TestStructuredDynamicOutputAndToolDecodePreserveNumbers(t *testing.T) {
	t.Parallel()
	document := `{"integer":9007199254740993,"nested":[-9007199254740993,0.123456789012345678901]}`
	execution := SimplePrompt("return JSON")
	execution.ResponseFormat = &SchemaDefinition{Schema: JSONDocument(`{"type":"object"}`)}
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		return NewResponse([]ContentPart{TextPart{Text: document}}), nil
	}}
	value, err := ExecuteWithStructuredOutput[map[string]any](context.Background(), invoker, execution)
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), (*value)["integer"])
	require.Equal(
		t,
		[]any{json.Number("-9007199254740993"), json.Number("0.123456789012345678901")},
		(*value)["nested"],
	)
	decoded, err := DecodeToolArgs[map[string]any](document)
	require.NoError(t, err)
	require.Equal(t, *value, decoded)
}

type exactRuntimeRegistry struct{ recipeTestRegistry }

func (r *exactRuntimeRegistry) Plan(_ context.Context, id string, input RegistryPlanInput) (*RenderPlan, error) {
	template, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleUser, Content: TextContent("{{ .Input.query }} {{ .LateVars.suffix }}")}},
		WithMetadata(PromptMetadata{ID: id}),
		WithInputSchema(
			&SchemaDefinition{
				Schema: JSONDocument(
					`{"type":"object","properties":{"query":{"type":"number"},"suffix":{"late":true}}}`,
				),
			},
		),
	)
	if err != nil {
		return nil, err
	}
	return NewRenderPlanFromPlanInput(template, input)
}

type draft7Output struct {
	N int `json:"n"`
}

func (draft7Output) JSONSchema() JSONDocument {
	return JSONDocument(
		`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`,
	)
}

func TestExplicitOutputDialectSurvivesPublicHelpers(t *testing.T) {
	t.Parallel()
	format, err := ResponseFormatFromStruct[draft7Output]()
	require.NoError(t, err)
	require.Equal(t, "http://json-schema.org/draft-07/schema#", format.Dialect)
	require.NoError(t, ValidateSchemaJSON(format, JSONDocument(`{"n":1}`)))
	template, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent("return JSON")}})
	require.NoError(t, err)
	plan, err := WithResponseFormatFromStruct[draft7Output](NewRenderPlan(template))
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, format.Dialect, execution.ResponseFormat.Dialect)
	invoker := &scriptedInvoker{generate: func(_ context.Context, execution *PromptExecution) (*Response, error) {
		require.Equal(t, format.Dialect, execution.ResponseFormat.Dialect)
		return NewResponse([]ContentPart{TextPart{Text: `{"n":1}`}}), nil
	}}
	for _, execution := range []*PromptExecution{execution, SimplePrompt("return JSON")} {
		value, err := ExecuteWithStructuredOutput[draft7Output](context.Background(), invoker, execution)
		require.NoError(t, err)
		require.Equal(t, 1, value.N)
	}
}

func TestReflectionQuotedJSONNumberMatchesEncoding(t *testing.T) {
	t.Parallel()
	type payload struct {
		Number  json.Number  `json:"number,string"`
		Pointer *json.Number `json:"pointer,string"`
	}
	schema, err := ExtractSchema(payload{})
	require.NoError(t, err)
	for _, token := range []string{"9007199254740993", "0.123456789012345678901", "-1.234e+100", ""} {
		number := json.Number(token)
		for _, pointer := range []*json.Number{nil, &number} {
			document, err := json.Marshal(payload{Number: number, Pointer: pointer})
			require.NoError(t, err)
			require.NoError(t, ValidateSchemaJSON(&SchemaDefinition{Schema: MustJSONDocumentFromMap(schema)}, document))
		}
	}
}

func TestReflectionRejectsUnsupportedJSONTagName(t *testing.T) {
	t.Parallel()
	typeOf := reflect.StructOf([]reflect.StructField{{
		Name: "N", Type: reflect.TypeFor[int](), Tag: reflect.StructTag(`json:"a\\b"`),
	}})
	_, err := ExtractSchema(reflect.New(typeOf).Elem().Interface())
	require.ErrorIs(t, err, ErrSchemaUnsupported)
}

func TestTemplateBindingPreservesNullableValues(t *testing.T) {
	t.Parallel()
	type item struct {
		Count int `json:"count" prompt:"count"`
	}
	type payload struct {
		Count *int    `json:"count" prompt:"count"`
		Items []*item `json:"items" prompt:"items"`
	}
	schema, err := ExtractSchema(payload{})
	require.NoError(t, err)
	for _, value := range []payload{{}, {Items: []*item{nil, {Count: 0}}}} {
		template, err := NewChatPromptTemplate(
			[]MessageTemplate{
				{Role: RoleUser, Content: TextContent("{{if eq .Input.count nil}}null{{else}}{{.Input.count}}{{end}}")},
			},
			WithInputSchema(&SchemaDefinition{Schema: MustJSONDocumentFromMap(schema)}),
		)
		require.NoError(t, err)
		plan, err := NewRenderPlanFromStruct(template, value)
		require.NoError(t, err)
		execution, err := plan.Execute(context.Background())
		require.NoError(t, err)
		require.Equal(t, "null", mustTextFromParts(t, execution.Messages[0].Content))
		vars, _, err := bindTemplateVars(value)
		require.NoError(t, err)
		document, err := json.Marshal(vars)
		require.NoError(t, err)
		original, err := json.Marshal(value)
		require.NoError(t, err)
		require.JSONEq(t, string(original), string(document))
	}
}

func TestExplicitOutputDialectSurvivesStreaming(t *testing.T) {
	t.Parallel()
	invoker := &scriptedInvoker{
		generateStream: func(_ context.Context, execution *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			require.Equal(t, "http://json-schema.org/draft-07/schema#", execution.ResponseFormat.Dialect)
			require.NoError(t, ValidateSchemaJSON(execution.ResponseFormat, JSONDocument(`[{"n":1}]`)))
			return func(yield func(*ResponseChunk, error) bool) {
				yield(
					&ResponseChunk{
						Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
						Content:  []ContentPart{TextPart{Text: `[{"n":1}]`}},
						Kind:     StreamFinish,
						Outcome:  OutcomeCompleted,
					},
					nil,
				)
			}
		},
	}
	values, err := collectSeq(
		StreamStructuredOutput[draft7Output](context.Background(), invoker, SimplePrompt("return JSON"), StreamNative),
	)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, 1, values[0].N)
}

func TestInputSchemaPreservesEmptyAndNullableHistory(t *testing.T) {
	t.Parallel()
	type payload struct {
		History []ChatMessage `prompt:"history"`
	}
	template, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleUser, Content: TextContent("current")}},
		WithInputSchema(
			&SchemaDefinition{
				Schema: JSONDocument(
					`{"type":"object","properties":{"history":{"type":["array","null"],"format":"messages","items":{"type":"object"}}},"required":["history"],"additionalProperties":false}`,
				),
			},
		),
	)
	require.NoError(t, err)
	for _, history := range [][]ChatMessage{nil, {}, {NewUserMessage("previous")}} {
		input, err := PlanInputFrom(payload{History: history})
		require.NoError(t, err)
		require.NotContains(t, input.boundVars, "history")
		plan, err := NewRenderPlanFromPlanInput(template, input)
		require.NoError(t, err)
		execution, err := plan.Execute(context.Background())
		require.NoError(t, err)
		require.Len(t, execution.Messages, len(history)+1)
	}
}

type strictQuotedCount int

func (strictQuotedCount) JSONSchema() JSONDocument {
	return JSONDocument(`{"type":"integer","const":42}`)
}

type strictCodecCount int

func (value strictCodecCount) MarshalJSON() ([]byte, error) { return json.Marshal(int(value)) }
func (strictCodecCount) JSONSchema() JSONDocument {
	return JSONDocument(`{"type":"integer","const":42}`)
}

func TestReflectionRejectsFieldProviderStringProjection(t *testing.T) {
	t.Parallel()
	type payload struct {
		Count strictQuotedCount `json:"count,string"`
	}
	type codecPayload struct {
		Count strictCodecCount `json:"count,string"`
	}
	for _, value := range []any{payload{Count: 43}, codecPayload{Count: 43}} {
		_, err := ExtractSchema(value)
		require.ErrorIs(t, err, ErrSchemaUnsupported)
	}
	_, err := NewTypedTool("count", func(_ context.Context, _ payload) (string, error) { return "", nil })
	require.ErrorIs(t, err, ErrSchemaUnsupported)
	_, err = ResponseFormatFromStruct[payload]()
	require.ErrorIs(t, err, ErrSchemaUnsupported)
}

func TestReflectionRejectsRecursiveCompositeTypes(t *testing.T) {
	t.Parallel()
	type recursiveSlice []recursiveSlice
	type recursivePointer *recursivePointer
	type recursiveArray [1]*recursiveArray
	type payload struct {
		Value recursivePointer `json:"value"`
	}
	for _, value := range []any{recursiveSlice(nil), recursivePointer(nil), recursiveArray{}, payload{}} {
		_, err := ExtractSchema(value)
		require.ErrorIs(t, err, ErrSchemaUnsupported)
	}
}

type localRefOutput struct {
	N int `json:"n"`
}

func (localRefOutput) JSONSchema() JSONDocument {
	return JSONDocument(
		`{"type":"object","$defs":{"positive":{"type":"integer","minimum":1}},"properties":{"n":{"$ref":"#/$defs/positive"}},"required":["n"]}`,
	)
}

type draft7RefOutput struct {
	N int `json:"n"`
}

func (draft7RefOutput) JSONSchema() JSONDocument {
	return JSONDocument(
		`{"$schema":"http://json-schema.org/draft-07/schema#","$id":"https://example.invalid/output","type":"object","definitions":{"positive":{"type":"integer","minimum":1}},"properties":{"n":{"$ref":"#/definitions/positive"}},"required":["n"]}`,
	)
}

func TestStructuredStreamingPreservesItemReferenceScope(t *testing.T) {
	t.Parallel()
	for _, document := range []string{`{"n":1}`, `[{"n":1}]`, `[{"n":0}]`} {
		invoker := &scriptedInvoker{
			generateStream: func(_ context.Context, execution *PromptExecution) iter.Seq2[*ResponseChunk, error] {
				if document != `[{"n":0}]` {
					require.NoError(t, ValidateSchemaJSON(execution.ResponseFormat, JSONDocument(document)))
				}
				return func(yield func(*ResponseChunk, error) bool) {
					yield(
						&ResponseChunk{
							Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
							Content:  []ContentPart{TextPart{Text: document}},
							Kind:     StreamFinish,
							Outcome:  OutcomeCompleted,
						},
						nil,
					)
				}
			},
		}
		values, err := collectSeq(
			StreamStructuredOutput[localRefOutput](context.Background(), invoker, SimplePrompt("JSON"), StreamNative),
		)
		draft7Values, draft7Err := collectSeq(
			StreamStructuredOutput[draft7RefOutput](context.Background(), invoker, SimplePrompt("JSON"), StreamNative),
		)
		if document == `[{"n":0}]` {
			require.ErrorIs(t, err, ErrSchemaMismatch)
			require.ErrorIs(t, draft7Err, ErrSchemaMismatch)
			continue
		}
		require.NoError(t, err)
		require.NoError(t, draft7Err)
		require.Equal(t, []localRefOutput{{N: 1}}, values)
		require.Equal(t, []draft7RefOutput{{N: 1}}, draft7Values)
	}
}

// Each marker keeps provider contracts immutable across parallel tests.
type streamID interface{ schemaID() string }
type emptyStreamID struct{}

func (emptyStreamID) schemaID() string { return "" }

type defaultStreamID struct{}

func (defaultStreamID) schemaID() string { return schemaSourceURI }

type relativeStreamID struct{}

func (relativeStreamID) schemaID() string { return "nested/item.json" }

type absoluteStreamID struct{}

func (absoluteStreamID) schemaID() string { return "https://example.invalid/item" }

type streamIDOutput[I streamID] struct {
	N int `json:"n"`
}

func (streamIDOutput[I]) JSONSchema() JSONDocument {
	var id I
	return JSONDocument(
		fmt.Sprintf(
			`{"$id":%q,"type":"object","$defs":{"positive":{"type":"integer","minimum":1}},"properties":{"n":{"$ref":"#/$defs/positive"}},"required":["n"]}`,
			id.schemaID(),
		),
	)
}

type draft7StreamIDOutput[I streamID] struct {
	N int `json:"n"`
}

func (draft7StreamIDOutput[I]) JSONSchema() JSONDocument {
	document := (streamIDOutput[I]{}).JSONSchema()
	return JSONDocument(
		strings.ReplaceAll(
			strings.Replace(string(document), "{", `{"$schema":"http://json-schema.org/draft-07/schema#",`, 1),
			"$defs",
			"definitions",
		),
	)
}
func checkStructuredStreamID[I streamID](t *testing.T) {
	t.Helper()
	checkStructuredStreamIDType[streamIDOutput[I]](t)
	checkStructuredStreamIDType[draft7StreamIDOutput[I]](t)
}
func checkStructuredStreamIDType[T any](t *testing.T) {
	t.Helper()
	for _, document := range []string{`{"n":1}`, `[{"n":1}]`, `[{"n":0}]`} {
		// Arrange: the provider inspects the exact document contract it receives.
		called := false
		invoker := &scriptedInvoker{
			generateStream: func(_ context.Context, execution *PromptExecution) iter.Seq2[*ResponseChunk, error] {
				called = true
				if document != `[{"n":0}]` {
					require.NoError(t, ValidateSchemaJSON(execution.ResponseFormat, JSONDocument(document)))
				}
				return func(yield func(*ResponseChunk, error) bool) {
					yield(
						&ResponseChunk{
							Identity: StreamIdentity{PartIDs: []string{"text"}},
							Content:  []ContentPart{TextPart{Text: document}},
							Kind:     StreamFinish,
							Outcome:  OutcomeCompleted,
						},
						nil,
					)
				}
			},
		}
		// Act.
		values, err := collectSeq(
			StreamStructuredOutput[T](
				context.Background(),
				invoker,
				SimplePrompt("JSON"),
				StreamNative,
			),
		)
		// Assert: resource identities cannot prevent invocation or bypass item validation.
		require.True(t, called)
		if document == `[{"n":0}]` {
			require.ErrorIs(t, err, ErrSchemaMismatch)
		} else {
			require.NoError(t, err)
			require.Len(t, values, 1)
			document, marshalErr := json.Marshal(values[0])
			require.NoError(t, marshalErr)
			require.JSONEq(t, `{"n":1}`, string(document))
		}
	}
}
func TestStructuredStreamItemResourceIDs(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) { t.Parallel(); checkStructuredStreamID[emptyStreamID](t) })
	t.Run("default source", func(t *testing.T) { t.Parallel(); checkStructuredStreamID[defaultStreamID](t) })
	t.Run("relative", func(t *testing.T) { t.Parallel(); checkStructuredStreamID[relativeStreamID](t) })
	t.Run("absolute", func(t *testing.T) { t.Parallel(); checkStructuredStreamID[absoluteStreamID](t) })
}

func TestCanonicalSchemaSourceRetainsLocalErrorLocations(t *testing.T) {
	t.Parallel()
	// Arrange: an absolute root identity also becomes the in-memory retrieval address.
	definition := JSONDocument(
		`{"$id":"https://example.invalid/root","type":"object","properties":{"n":{"$ref":"missing.json"}}}`,
	)
	validator := NewJSONSchemaValidator(nil)
	// Act.
	_, err := validator.CompileSchema(definition, SchemaDialect2020)
	// Assert: no implicit IO and diagnostics still point into the supplied document.
	require.ErrorIs(t, err, ErrSchemaUnsupported)
	var failure *SchemaCompilationError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, []SchemaSourceLocation{{Resource: "", Pointer: "/properties/n/$ref"}}, failure.Locations)
}

func (r *exactRuntimeRegistry) CaptureManifest(ctx context.Context, id string) (*ManifestSnapshot, error) {
	return captureRecipeFixture(ctx, id, r)
}

type task45RecursiveEmbedded struct {
	*task45RecursiveEmbedded

	Value chan int
}

type task45RecursiveNamed struct {
	*task45RecursiveNamed

	Next *task45RecursiveNamed `json:"next"`
}

func TestSchemaDominanceSkipsAnonymousEmbeddingCycles(t *testing.T) {
	t.Parallel()
	// Arrange: recursive anonymous metadata loses to the outer JSON field.
	value := struct {
		task45RecursiveEmbedded

		Value string
	}{Value: "known"}
	encoded, err := json.Marshal(value) //nolint:musttag // Exercise tag-free encoding/json field dominance.
	require.NoError(t, err)
	require.JSONEq(t, `{"Value":"known"}`, string(encoded))
	// Act.
	schema, err := ExtractSchema(value)
	// Assert: infer only the selected string; unsupported selected controls fail.
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Value": map[string]any{"type": "string"}}, schema["properties"])
	selected := task45RecursiveEmbedded{}
	require.Nil(t, selected.task45RecursiveEmbedded)
	_, err = ExtractSchema(selected)
	require.Error(t, err)
	recursive := task45RecursiveNamed{}
	require.Nil(t, recursive.task45RecursiveNamed)
	_, err = ExtractSchema(recursive)
	require.ErrorIs(t, err, ErrSchemaUnsupported)
}
