package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaPolicySurvivesExecutionWireAndClonePaths(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := SimplePrompt("fixture")
	exec.Requirements.Schema = SchemaPolicy{Mode: SchemaLowering, AllowedTransforms: []string{"fixture.drop-minimum"}}
	// Act.
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	serialized, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(serialized)
	require.NoError(t, err)
	// Assert.
	require.Equal(t, exec.Requirements.Schema, restored.Requirements.Schema)
	require.Nil(t, restored.Validator)
	for _, copied := range []*PromptExecution{
		exec.Clone(), exec.Normalize(), exec.AddMessage(ChatMessage{Role: RoleUser, Content: []ContentPart{TextPart{Text: "next"}}}),
	} {
		copied.Requirements.Schema.AllowedTransforms[0] = "changed"
	}
	wire.Requirements.Schema.AllowedTransforms[0] = "changed"
	restored.Requirements.Schema.AllowedTransforms[0] = "changed"
	require.Equal(t, []string{"fixture.drop-minimum"}, exec.Requirements.Schema.AllowedTransforms)
}

func TestSchemaPolicyWireRejectsInvalidModeAndPermissions(t *testing.T) {
	t.Parallel()
	for _, policy := range []string{
		`{"mode":"best-effort"}`, `{"mode":"strict","allowed_transforms":["drop"]}`,
		`{"allowed_transforms":["drop"]}`, `{"mode":"lowering","allowed_transforms":["drop","drop"]}`,
		`{"mode":"lowering","allowed_transforms":[""]}`, `{"mode":"lowering","legacy":true}`,
	} {
		// Arrange.
		wire := JSONDocument(
			`{"format":"prompty.execution","messages":[],"metadata":{"id":"fixture"},"requirements":{"schema":` + policy + `}}`,
		)
		// Act.
		_, err := DecodeExecutionJSON(wire)
		// Assert.
		require.ErrorIs(t, err, ErrSchemaMismatch)
	}
}

func TestSchemaPolicyWireRejectsWhitespacePermissionSemantically(t *testing.T) {
	t.Parallel()
	// Arrange: structurally a string, but not a meaningful transformation ID.
	exec := SimplePrompt("fixture")
	exec.Requirements.Schema = SchemaPolicy{Mode: SchemaLowering, AllowedTransforms: []string{" "}}
	invalid := JSONDocument(
		`{"format":"prompty.execution","messages":[],"metadata":{"id":"fixture"},"requirements":{"schema":{"mode":"lowering","allowed_transforms":[" "]}}}`,
	)
	// Act.
	_, encodeErr := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	_, decodeErr := DecodeExecutionJSON(invalid)
	// Assert.
	require.ErrorIs(t, encodeErr, ErrSchemaUnsupported)
	require.ErrorIs(t, decodeErr, ErrSchemaUnsupported)
}

func TestSchemaCompilationReportSurvivesResponseWire(t *testing.T) {
	t.Parallel()
	// Arrange.
	request := compileFixtureRequest(`{"type":"integer"}`)
	request.Name = "result"
	compiled, err := CompileProviderSchema(
		request,
		schemaMapperFunc(func(r SchemaMappingRequest) (SchemaMapping, error) {
			return SchemaMapping{Schema: r.Source}, nil
		}),
	)
	require.NoError(t, err)
	response := NewResponse([]ContentPart{TextPart{Text: "1"}})
	response.Diagnostics = ExecutionDiagnostics{
		Profile: request.Profile, Snapshot: "fixture", Schemas: []SchemaCompilationReport{compiled.Report()},
	}
	// Act.
	wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	serialized, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeResponseJSON(serialized)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, compiled.Report(), restored.Diagnostics.Schemas[0])
	wire.Diagnostics.Schemas[0].Name = "changed"
	restored.Diagnostics.Schemas[0].Name = "changed"
	require.Equal(t, "result", response.Diagnostics.Schemas[0].Name)
}
