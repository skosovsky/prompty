package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type loweringFixture struct {
	*checkedFixture

	response   *prompty.Response
	translated prompty.JSONDocument
}

type unmappedFixture struct {
	ProviderAdapter[*prompty.PromptExecution, *prompty.Response]

	descriptor RuntimeDescriptor
}

func (f unmappedFixture) RuntimeDescriptor() RuntimeDescriptor { return f.descriptor }

func TestSchemaKnowledgeCannotImplementMissingMapper(t *testing.T) {
	t.Parallel()
	// Arrange: embedding only the transport interface does not expose the schema port.
	base := &checkedFixture{descriptor: fixtureDescriptor()}
	fixture := unmappedFixture{ProviderAdapter: base, descriptor: base.descriptor}
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"integer"}`)}
	profile := base.descriptor.Profiles[0]
	exec.Requirements.Override = &profile
	// Act.
	_, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
	require.Zero(t, base.translations)
	require.Zero(t, base.calls)
}

func (f *loweringFixture) MapSchema(prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	return prompty.SchemaMapping{
		Schema: prompty.JSONDocument(`{"type":"integer"}`),
		Transformations: []prompty.SchemaTransformation{
			{ID: "fixture.drop-minimum", Pointer: "/minimum", Reason: "fixture limitation"},
		},
	}, nil
}

func (f *loweringFixture) Translate(exec *prompty.PromptExecution) (*prompty.PromptExecution, error) {
	f.translations++
	f.translated = prompty.CloneJSONDocument(exec.ResponseFormat.Schema)
	return exec, nil
}

func (f *loweringFixture) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	f.calls++
	return f.response, nil
}

func TestCheckedLoweringValidatesOriginalAndPreservesExecutionEvidence(t *testing.T) {
	t.Parallel()
	// Arrange: valid provider-schema integer violates original minimum.
	fixture := &loweringFixture{
		checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
		response:       prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "1"}}),
	}
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Name:   "result",
		Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`),
	}
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode:              prompty.SchemaLowering,
		AllowedTransforms: []string{"fixture.drop-minimum"},
	}
	// Act.
	response, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert: post-dispatch validation failure does not erase the known response or retry.
	require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
	require.NotNil(t, response)
	require.Equal(t, 1, fixture.calls)
	require.JSONEq(t, `{"type":"integer"}`, string(fixture.translated))
	require.JSONEq(t, `{"type":"integer","minimum":10}`, string(exec.ResponseFormat.Schema))
	require.Len(t, response.Diagnostics.Schemas, 1)
	report := response.Diagnostics.Schemas[0]
	require.Equal(t, "result", report.Name)
	require.Equal(t, prompty.SchemaLowering, report.Guarantee)
	require.NotEqual(t, report.SourceDigest, report.TranslatedDigest)
}

func TestCheckedStrictRejectsProviderLoweringBeforeTranslation(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &loweringFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()}}
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`)}
	// Act.
	_, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
	require.Zero(t, fixture.calls)
	require.Zero(t, fixture.translations)
}

func TestCompiledToolSchemasValidateEvenWhenOutcomeIsWrong(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = []prompty.ToolDefinition{
		{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object","required":["x"]}`)},
	}
	prepared, err := compilePreparedSchemas(&checkedFixture{}, exec, fixtureDescriptor().Profiles[0].Identity)
	require.NoError(t, err)
	response := prompty.NewResponse([]prompty.ContentPart{prompty.ToolCallPart{Name: "lookup", Args: `{}`}})
	// Act.
	err = prepared.validate(response)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
}
