package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

const fixtureOptionsSchema = `{"type":"object","properties":{"provider_settings":{"type":"object",` +
	`"properties":{"reasoning_effort":{"enum":["low"]},"thinking_budget":{"type":"integer","minimum":1024,"maximum":2048},` +
	`"thinking_mode":{"enum":["adaptive","enabled"]},"thinking_visibility":{"enum":["omitted"]}},"additionalProperties":false,` +
	`"allOf":[{"if":{"properties":{"thinking_mode":{"const":"adaptive"}},"required":["thinking_mode"]},` +
	`"then":{"not":{"required":["thinking_budget"]}}}]}},"not":{"required":["temperature"]}}`

func TestModelOptionsRequireEffectiveReasoningProfileBeforeTranslation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		settings prompty.JSONDocument
		schema   prompty.JSONDocument
		model    string
		want     error
	}{
		{"missing contract", prompty.JSONDocument(`{"reasoning_effort":"low"}`), nil, "", ErrUnknownProfile},
		{"foreign model", prompty.JSONDocument(`{"reasoning_effort":"low"}`), prompty.JSONDocument(fixtureOptionsSchema), "unknown", ErrUnknownProfile},
		{"unsupported effort", prompty.JSONDocument(`{"reasoning_effort":"high"}`), prompty.JSONDocument(fixtureOptionsSchema), "", ErrUnsupportedCapability},
		{"fractional budget", prompty.JSONDocument(`{"thinking_budget":1024.5}`), prompty.JSONDocument(fixtureOptionsSchema), "", ErrUnsupportedCapability},
		{"budget below minimum", prompty.JSONDocument(`{"thinking_budget":10}`), prompty.JSONDocument(fixtureOptionsSchema), "", ErrUnsupportedCapability},
		{"adaptive and budget", prompty.JSONDocument(`{"thinking_mode":"adaptive","thinking_budget":1024}`), prompty.JSONDocument(fixtureOptionsSchema), "", ErrUnsupportedCapability},
		{"unsupported visibility", prompty.JSONDocument(`{"thinking_visibility":"summarized"}`), prompty.JSONDocument(fixtureOptionsSchema), "", ErrUnsupportedCapability},
		{"unresolved reference", prompty.JSONDocument(`{"reasoning_effort":"low"}`), prompty.JSONDocument(`{"$ref":"https://unbound.invalid/options"}`), "", ErrUnsupportedCapability},
		{"null settings", prompty.JSONDocument(`null`), nil, "", ErrUnsupportedCapability},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			desc := fixtureDescriptor()
			desc.ReasoningSettings = []string{
				"reasoning_effort",
				"thinking_budget",
				"thinking_mode",
				"thinking_visibility",
			}
			desc.Profiles[0].ModelOptionsSchema = tc.schema
			fixture := &checkedFixture{descriptor: desc}
			exec := prompty.SimplePrompt("fixture")
			exec.ModelOptions = &prompty.ModelOptions{Model: tc.model, ProviderSettings: tc.settings}
			// Act.
			_, err := NewClient(fixture).Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, tc.want)
			var capabilityErr *CapabilityError
			require.ErrorAs(t, err, &capabilityErr)
			require.Equal(t, "model_options", capabilityErr.Diagnostics.Checks[0].Requirement)
			require.Zero(t, fixture.translations)
			require.Zero(t, fixture.calls)
		})
	}
}

func TestModelOptionsSchemaSnapshotOverrideAndWire(t *testing.T) {
	t.Parallel()
	// Arrange: unknown effective model resolved only by scoped, explicit knowledge.
	desc := fixtureDescriptor()
	desc.ReasoningSettings = []string{"reasoning_effort"}
	profile := prompty.CloneCapabilityProfile(desc.Profiles[0])
	profile.Identity.Model = "override"
	profile.ModelOptionsSchema = prompty.JSONDocument(fixtureOptionsSchema)
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{
		Model:            "override",
		ProviderSettings: prompty.JSONDocument(`{"reasoning_effort":"low"}`),
	}
	exec.Requirements.Override = &profile
	fixture := &checkedFixture{descriptor: desc}
	// Act.
	prepared, err := Prepare(fixture, exec, "")
	require.NoError(t, err)
	wire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalExecution(wire)
	require.NoError(t, err)
	cloned := restored.Clone()
	cloned.Requirements.Override.ModelOptionsSchema[0] = 'x'
	// Assert.
	require.Equal(t, profile.ModelOptionsSchema, restored.Requirements.Override.ModelOptionsSchema)
	require.True(t, prepared.Diagnostics().Override)
	require.Equal(t, profile.Identity, prepared.Diagnostics().Profile)
	require.Equal(t, prompty.CapabilitySupported, prepared.Diagnostics().Checks[0].Status)
	// Arrange/Act: an incompatible option cannot disappear silently.
	restored.ModelOptions.Temperature = new(0.5)
	_, err = Prepare(fixture, restored, "")
	// Assert.
	require.ErrorIs(t, err, ErrUnsupportedCapability)
	require.Equal(t, 1, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestReasoningAndToolsCombinationFailsBeforeTranslation(t *testing.T) {
	t.Parallel()
	// Arrange: individually valid tools and options may still be incompatible.
	desc := fixtureDescriptor()
	desc.ReasoningSettings = []string{"reasoning_effort"}
	desc.Profiles[0].ModelOptionsSchema = prompty.JSONDocument(fixtureOptionsSchema)
	desc.Profiles[0].ForbiddenCombinations = [][]string{{prompty.CapabilityTools, prompty.CapabilityReasoning}}
	fixture := &checkedFixture{descriptor: desc}
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{ProviderSettings: prompty.JSONDocument(`{"reasoning_effort":"low"}`)}
	exec.Tools = []prompty.ToolDefinition{{Name: "fn", Parameters: prompty.JSONDocument(`{"type":"object"}`)}}
	// Act.
	_, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, ErrUnsupportedCombination)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}
