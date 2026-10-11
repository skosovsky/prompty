package adapter

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type unavailableSchemaCompiler struct {
	result prompty.ValueValidator
	err    error
}

func (v unavailableSchemaCompiler) CompileSchema(prompty.JSONDocument, string) (prompty.ValueValidator, error) {
	return v.result, v.err
}

func TestPreparedSchemasRetainPerDefinitionDialect(t *testing.T) {
	t.Parallel()
	// Arrange: output and tool schemas select different supported dialects.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	fixture.descriptor.Profiles[0].ForbiddenCombinations = nil
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Dialect: prompty.SchemaDialect7,
		Schema: prompty.JSONDocument(
			`{"$schema":"http://json-schema.org/draft-07/schema#","type":"integer","exclusiveMinimum":0}`,
		),
	}
	exec.Tools = []prompty.ToolDefinition{
		{Name: "lookup", Dialect: prompty.SchemaDialect2020, Parameters: prompty.JSONDocument(`{"type":"object"}`)},
	}
	// Act.
	prepared, err := Prepare(fixture, exec, "")
	// Assert.
	require.NoError(t, err)
	reports := prepared.Diagnostics().Schemas
	require.Len(t, reports, 2)
	require.Equal(t, prompty.SchemaDialect7, reports[0].Dialect)
	require.Equal(t, prompty.SchemaDialect2020, reports[1].Dialect)
	require.Equal(t, prompty.SchemaDialect7, prepared.request.ResponseFormat.Dialect)
	require.Equal(t, prompty.SchemaDialect2020, prepared.request.Tools[0].Dialect)
	require.ErrorIs(t, prepared.schemas.output.ValidateOriginal(prompty.JSONDocument(`0`)), prompty.ErrSchemaMismatch)
	require.NoError(t, prepared.schemas.output.ValidateOriginal(prompty.JSONDocument(`1`)))
	require.Zero(t, fixture.calls)
}

func TestUnavailableSchemaValidationFailsBeforeTranslateAndDispatch(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, role := range []prompty.SchemaRole{prompty.SchemaOutput, prompty.SchemaToolInput} {
			for _, test := range []struct {
				name     string
				compiler prompty.SchemaValidator
				dialect  string
				want     error
			}{
				{"unsupported dialect", nil, "https://fixture.invalid/unsupported", prompty.ErrSchemaUnsupported},
				{"compiler rejects", unavailableSchemaCompiler{result: nil, err: fmt.Errorf("%w: fixture cannot enforce the selected contract", prompty.ErrSchemaUnsupported)}, prompty.SchemaDialect2020, prompty.ErrSchemaUnsupported},
				{"compiler returns nil", unavailableSchemaCompiler{result: nil, err: nil}, prompty.SchemaDialect2020, prompty.ErrSchemaValidatorRequired},
			} {
				t.Run(string(mode)+string(role)+test.name, func(t *testing.T) {
					t.Parallel()
					// Arrange: required schema enforcement is unavailable at preparation.
					base := &checkedFixture{descriptor: fixtureDescriptor()}
					client := NewClient(nativeCheckedFixture{checkedFixture: base})
					exec := prompty.SimplePrompt("fixture")
					exec.Validator = test.compiler
					if role == prompty.SchemaOutput {
						exec.ResponseFormat = &prompty.SchemaDefinition{
							Dialect: test.dialect,
							Schema:  prompty.JSONDocument(`{"type":"object"}`),
						}
					} else {
						exec.Tools = []prompty.ToolDefinition{
							{
								Name:       "lookup",
								Dialect:    test.dialect,
								Parameters: prompty.JSONDocument(`{"type":"object"}`),
							},
						}
					}
					// Act.
					err := executeSchemaDialectFixture(client, exec, mode)
					// Assert: no request preparation or provider side effect.
					require.ErrorIs(t, err, test.want)
					require.Zero(t, base.translations)
					require.Zero(t, base.calls)
				})
			}
		}
	}
}

func executeSchemaDialectFixture(client prompty.Invoker, exec *prompty.PromptExecution, mode prompty.StreamMode) error {
	if mode == "" {
		_, err := client.Execute(context.Background(), exec)
		return err
	}
	stream := client.ExecuteStream(context.Background(), exec, mode)
	for range stream.Events() {
	}
	return stream.Status().Err
}
