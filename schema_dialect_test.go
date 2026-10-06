package prompty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const draft7OutputFixture = `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",` +
	`"properties":{"value":{"type":"integer","exclusiveMinimum":0}},"required":["value"]}`

func TestSchemaDialectSurvivesCloneAndExecutionWire(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := SimplePrompt("fixture")
	exec.ResponseFormat = &SchemaDefinition{Dialect: SchemaDialect7, Schema: JSONDocument(draft7OutputFixture)}
	exec.Tools = []ToolDefinition{
		{Name: "lookup", Dialect: SchemaDialect7, Parameters: JSONDocument(draft7OutputFixture)},
	}
	// Act.
	cloned := exec.Clone()
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	decoded, err := DecodeExecutionJSON(encoded)
	// Assert: executable wire schema and all ownership boundaries retain the selection.
	require.NoError(t, err)
	for _, value := range []*PromptExecution{cloned, decoded} {
		require.Equal(t, SchemaDialect7, value.ResponseFormat.Dialect)
		require.Equal(t, SchemaDialect7, value.Tools[0].Dialect)
		require.Equal(t, exec.ResponseFormat.Schema, value.ResponseFormat.Schema)
		require.Equal(t, exec.Tools[0].Parameters, value.Tools[0].Parameters)
	}
	cloned.ResponseFormat.Schema[0] = ' '
	cloned.Tools[0].Parameters[0] = ' '
	require.Equal(t, byte('{'), exec.ResponseFormat.Schema[0])
	require.Equal(t, byte('{'), decoded.Tools[0].Parameters[0])
}

func TestStructuredHelpersSelectExplicitSchemaDialect(t *testing.T) {
	t.Parallel()
	for _, mode := range []StreamMode{"", StreamNative} {
		for _, test := range []struct {
			value string
			valid bool
		}{
			{`{"value":1}`, true}, {`{"value":0}`, false},
		} {
			t.Run(string(mode)+test.value, func(t *testing.T) {
				t.Parallel()
				// Arrange: the declared dialect must be retained through helper cloning.
				type output struct {
					Value int `json:"value"`
				}
				exec := SimplePrompt("fixture")
				exec.ResponseFormat = &SchemaDefinition{
					Dialect: SchemaDialect7,
					Schema:  JSONDocument(draft7OutputFixture),
				}
				calls := 0
				invoker := &scriptedInvoker{
					generate: func(_ context.Context, work *PromptExecution) (*Response, error) {
						calls++
						require.Equal(t, SchemaDialect7, work.ResponseFormat.Dialect)
						return NewResponse([]ContentPart{TextPart{Text: test.value}}), nil
					},
				}
				// Act.
				var err error
				if mode == "" {
					_, err = ExecuteWithStructuredOutput[output](context.Background(), invoker, exec)
				} else {
					_, err = collectSeq(StreamStructuredOutput[output](context.Background(), invoker, exec, mode))
				}
				// Assert: domain decode cannot accept zero against the original constraint.
				if test.valid {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ErrSchemaMismatch)
				}
				require.Equal(t, 1, calls)
				require.Equal(t, JSONDocument(draft7OutputFixture), exec.ResponseFormat.Schema)
			})
		}
	}
}

func TestToolSchemaSelectionUsesExplicitDialect(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := SimplePrompt("fixture")
	exec.Tools = []ToolDefinition{
		{Name: "lookup", Dialect: SchemaDialect7, Parameters: JSONDocument(draft7OutputFixture)},
	}
	// Act.
	compiled, err := compileToolSchemas(exec.Clone())
	// Assert.
	require.NoError(t, err)
	require.NoError(t, compiled["lookup"].ValidateJSON(JSONDocument(`{"value":1}`)))
	require.ErrorIs(t, compiled["lookup"].ValidateJSON(JSONDocument(`{"value":0}`)), ErrSchemaMismatch)
	exec.Tools[0].Dialect = SchemaDialect2020
	_, err = compileToolSchemas(exec)
	require.ErrorIs(t, err, ErrSchemaUnsupported)
}

func TestHelpersRejectDialectMismatchBeforeInvocation(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"structured sync", "structured stream", "typed tools"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			// Arrange: omitted dialect selects the default, not the declared older dialect.
			exec := SimplePrompt("fixture")
			exec.ResponseFormat = &SchemaDefinition{Schema: JSONDocument(draft7OutputFixture)}
			calls := 0
			invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
				calls++
				return NewResponse(nil), nil
			}}
			// Act.
			var err error
			switch path {
			case "structured sync":
				_, err = ExecuteWithStructuredOutput[struct{}](context.Background(), invoker, exec)
			case "structured stream":
				_, err = collectSeq(StreamStructuredOutput[struct{}](context.Background(), invoker, exec, StreamNative))
			case "typed tools":
				exec.ResponseFormat = nil
				exec.Tools = []ToolDefinition{{Name: "lookup", Parameters: JSONDocument(draft7OutputFixture)}}
				_, err = ExecuteWithTypedTools(context.Background(), invoker, exec, NewTypedToolRegistry())
			}
			// Assert.
			require.ErrorIs(t, err, ErrSchemaUnsupported)
			require.Zero(t, calls)
		})
	}
}
