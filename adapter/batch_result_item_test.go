package adapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type resultOnlyFixture struct {
	*checkedFixture

	mappings int
}

func (f *resultOnlyFixture) MapSchema(prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	f.mappings++
	return prompty.SchemaMapping{}, prompty.ErrSchemaUnsupported
}

func batchResultFixture() *resultOnlyFixture {
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities[prompty.CapabilityBatchResults] = prompty.CapabilitySupported
	return &resultOnlyFixture{checkedFixture: &checkedFixture{descriptor: desc}}
}

func TestResultOnlyPreparationDoesNotTranslateOrLowerOriginalSchemas(t *testing.T) {
	t.Parallel()
	// Arrange: provider mapping rejects this schema, but its original validator is valid.
	fixture := batchResultFixture()
	exec := prompty.SimplePrompt("original input")
	fixture.descriptor.Profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`)}
	exec.Requirements.Capabilities = []string{"unsupported-generation-only"}
	original := exec.Clone()
	// Act: bind local validation to the independent results capability.
	prepared, err := PrepareBatchResultItem(fixture, exec)
	require.NoError(t, err)
	response, err := ParseBatchResultItem(fixture, prepared,
		prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "1"}}))
	// Assert: no provider mapper/Translate/Execute and original minimum still enforced.
	require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
	require.NotNil(t, response)
	require.Zero(t, fixture.mappings)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
	require.Equal(t, original, exec)
	require.Len(t, prepared.Diagnostics.Schemas, 1)
	report := prepared.Diagnostics.Schemas[0]
	require.Equal(t, report.SourceDigest, report.TranslatedDigest)
	_, err = ParseBatchResultItem(fixture, prepared,
		prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "10"}}))
	require.NoError(t, err)
}

func TestResultOnlyPreparationIgnoresExpiredInputStateWithoutWeakeningSubmit(t *testing.T) {
	t.Parallel()
	// Arrange: opaque original input has expired since remote generation.
	fixture := batchResultFixture()
	exec := prompty.SimplePrompt("original")
	expired := time.Now().Add(-time.Hour)
	exec.Messages[0].ProviderState = []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
		Format: prompty.ProviderStateFormat, Scope: fixture.descriptor.Profiles[0].Identity,
		Codec: "fixture.opaque", Payload: prompty.JSONDocument(`{"opaque":true}`),
		Required: true, ExpiresAt: &expired,
	}}}
	// Act.
	prepared, err := PrepareBatchResultItem(fixture, exec)
	// Assert: no replay; generation still rejects this input through its ordinary gate.
	require.NoError(t, err)
	require.NotNil(t, prepared)
	_, err = PrepareBatchItem(fixture, exec)
	require.Error(t, err)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestResultOnlyPreparationRequiresItsOwnCapabilityAndRuntime(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := batchResultFixture()
	fixture.descriptor.Profiles[0].Capabilities[prompty.CapabilityBatchResults] = prompty.CapabilityUnknown
	// Act.
	_, err := PrepareBatchResultItem(fixture, prompty.SimplePrompt("input"))
	// Assert: generation support never substitutes for unknown retrieval support.
	require.ErrorIs(t, err, ErrUnknownProfile)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
	_, err = ParseBatchResultItem(fixture, nil, prompty.NewResponse(nil))
	require.ErrorIs(t, err, prompty.ErrBatchScope)
}

func TestResultOnlyOriginalToolContractAndForcedSelection(t *testing.T) {
	t.Parallel()
	// Arrange: results are available even if tool-generation support is now unavailable.
	fixture := batchResultFixture()
	fixture.descriptor.Profiles[0].Capabilities[prompty.CapabilityTools] = prompty.CapabilityUnsupported
	exec := prompty.SimplePrompt("original")
	exec.Tools = []prompty.ToolDefinition{
		{
			Name: "lookup",
			Parameters: prompty.JSONDocument(
				`{"type":"object","properties":{"n":{"type":"integer","enum":[9007199254740993]}},"required":["n"]}`,
			),
		},
		{Name: "other", Parameters: prompty.JSONDocument(`{"type":"object"}`)},
	}
	exec.ForcedTool = "lookup"
	prepared, err := PrepareBatchResultItem(fixture, exec)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, tool, args string
		valid            bool
	}{
		{"valid", "lookup", `{"n":9007199254740993}`, true},
		{"rounded", "lookup", `{"n":9007199254740992}`, false},
		{"wrong-choice", "other", `{}`, false},
		{"unknown", "undeclared", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Act.
			response := prompty.NewResponse([]prompty.ContentPart{prompty.ToolCallPart{Name: tc.tool, Args: tc.args}})
			response.Outcome = prompty.OutcomeToolCalls
			_, parseErr := ParseBatchResultItem(fixture, prepared, response)
			// Assert: original arguments and selection remain enforceable without a handler.
			if tc.valid {
				require.NoError(t, parseErr)
			} else {
				require.ErrorIs(t, parseErr, prompty.ErrSchemaMismatch)
			}
		})
	}
}
