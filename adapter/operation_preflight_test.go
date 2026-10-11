package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestOperationPreflightRetainsProfileWithoutGenerationValidation(t *testing.T) {
	t.Parallel()
	// Arrange: an effective profile requiring actual generation options.
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities[prompty.CapabilityBatchResults] = prompty.CapabilitySupported
	desc.Profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	model := desc.Profiles[0].Identity.Model
	capabilities := []string{prompty.CapabilityBatchResults, prompty.CapabilityBatchResults}
	original := CloneRuntimeDescriptor(desc)
	exec := prompty.SimplePrompt("generation")
	exec.ModelOptions = &prompty.ModelOptions{Model: model}
	// Act.
	operation, err := PreflightOperation(desc, model, capabilities, nil)
	generation, generationErr := Preflight(desc, exec, "", false)
	// Assert: full snapshot retained, requirements deduplicated, generation gate intact.
	require.NoError(t, err)
	require.ErrorIs(t, generationErr, ErrUnsupportedCapability)
	require.Equal(t, generation.Snapshot, operation.Snapshot)
	require.Equal(t, generation.Profile, operation.Profile)
	require.Len(t, operation.Checks, 1)
	require.Equal(t, prompty.CapabilityBatchResults, operation.Checks[0].Requirement)
	require.Equal(t, prompty.CapabilitySupported, operation.Checks[0].Status)
	require.Equal(t, original, desc)
	require.Len(t, capabilities, 2)
	changed := CloneRuntimeDescriptor(desc)
	changed.Profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["temperature"]}`)
	next, err := PreflightOperation(changed, model, capabilities, nil)
	require.NoError(t, err)
	require.NotEqual(t, operation.Snapshot, next.Snapshot)
}

func TestOperationPreflightStillRejectsUnsupportedBindingsAndCombinations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		edit func(*RuntimeDescriptor, *prompty.CapabilityProfile)
		caps []string
		want error
	}{
		{"unknown", func(d *RuntimeDescriptor, _ *prompty.CapabilityProfile) {
			delete(d.Profiles[0].Capabilities, prompty.CapabilityBatchResults)
		}, []string{prompty.CapabilityBatchResults}, ErrUnknownProfile},
		{"unsupported", func(d *RuntimeDescriptor, _ *prompty.CapabilityProfile) {
			d.Profiles[0].Capabilities[prompty.CapabilityBatchResults] = prompty.CapabilityUnsupported
		}, []string{prompty.CapabilityBatchResults}, ErrUnsupportedCapability},
		{"combination", func(d *RuntimeDescriptor, _ *prompty.CapabilityProfile) {
			d.Profiles[0].ForbiddenCombinations = [][]string{{prompty.CapabilityBatchResults, prompty.CapabilityBatchInspect}}
		}, []string{prompty.CapabilityBatchResults, prompty.CapabilityBatchInspect}, ErrUnsupportedCombination},
		{"ambiguous_profiles", func(d *RuntimeDescriptor, _ *prompty.CapabilityProfile) {
			d.Profiles = append(d.Profiles, d.Profiles[0])
		}, []string{prompty.CapabilityBatchResults}, ErrInvalidProfile},
		{"invalid_endpoint", func(d *RuntimeDescriptor, _ *prompty.CapabilityProfile) {
			d.Endpoint = "https://user:secret@example.invalid"
		}, []string{prompty.CapabilityBatchResults}, ErrInvalidProfile},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			desc := fixtureDescriptor()
			for _, capability := range []string{prompty.CapabilityBatchResults, prompty.CapabilityBatchInspect} {
				desc.Profiles[0].Capabilities[capability] = prompty.CapabilitySupported
			}
			test.edit(&desc, nil)
			// Act.
			_, err := PreflightOperation(desc, desc.DefaultModel, test.caps, nil)
			// Assert.
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestOperationPreflightScopedOverrideAndMissingImplementation(t *testing.T) {
	t.Parallel()
	// Arrange: explicit operation knowledge cannot change destination or create stream IO.
	desc := fixtureDescriptor()
	profile := prompty.CloneCapabilityProfile(desc.Profiles[0])
	profile.Capabilities[prompty.CapabilityBatchResults] = prompty.CapabilitySupported
	profile.Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
	// Act.
	accepted, err := PreflightOperation(
		desc,
		profile.Identity.Model,
		[]string{prompty.CapabilityBatchResults},
		&profile,
	)
	// Assert.
	require.NoError(t, err)
	require.True(t, accepted.Override)
	_, err = PreflightOperation(desc, profile.Identity.Model, []string{prompty.CapabilityNativeStream}, &profile)
	require.ErrorIs(t, err, ErrUnsupportedCapability)
	profile.Identity.Model = "foreign"
	_, err = PreflightOperation(desc, desc.DefaultModel, []string{prompty.CapabilityBatchResults}, &profile)
	require.ErrorIs(t, err, ErrInvalidProfileOverride)
}

func TestInputOperationPreservesContentCapabilitiesAndMIME(t *testing.T) {
	t.Parallel()
	// Arrange: resource creation has input semantics, not a generation options object.
	desc := fixtureDescriptor()
	profile := &desc.Profiles[0]
	profile.ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	profile.InputMIME = []string{"image/png"}
	for _, capability := range []string{prompty.CapabilityCacheCreate, prompty.CapabilityTools,
		prompty.CapabilityForcedToolSelection, prompty.CapabilityProviderState} {
		profile.Capabilities[capability] = prompty.CapabilitySupported
	}
	exec := prompty.NewExecution([]prompty.ChatMessage{{Role: prompty.RoleUser,
		Content: []prompty.ContentPart{prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1}}},
		ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
			Format: prompty.ProviderStateFormat, Scope: profile.Identity, Codec: "fixture.input",
			Payload: []byte(`{"opaque":true}`), Required: true,
		}}},
	}})
	exec.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
	exec.ForcedTool = "lookup"
	exec.Requirements.Capabilities = []string{prompty.CapabilityCacheCreate}
	original := exec.Clone()
	// Act.
	report, err := PreflightInputOperation(desc, exec)
	// Assert: all content guarantees stay checked, no settings are fabricated.
	require.NoError(t, err)
	require.Equal(t, original, exec)
	checks := make(map[string]prompty.CapabilitySupport)
	for _, check := range report.Checks {
		checks[check.Requirement] = check.Status
	}
	require.Len(t, checks, 5)
	for _, name := range []string{prompty.CapabilityCacheCreate, prompty.CapabilityTools,
		prompty.CapabilityForcedToolSelection, prompty.CapabilityProviderState, "input_mime:image/png"} {
		require.Equal(t, prompty.CapabilitySupported, checks[name])
	}
	// Each independently unsupported requirement remains a rejection.
	for _, name := range []string{prompty.CapabilityTools, prompty.CapabilityProviderState} {
		changed := CloneRuntimeDescriptor(desc)
		changed.Profiles[0].Capabilities[name] = prompty.CapabilityUnsupported
		_, err = PreflightInputOperation(changed, exec)
		require.ErrorIs(t, err, ErrUnsupportedCapability)
	}
	changed := CloneRuntimeDescriptor(desc)
	changed.Profiles[0].InputMIME = nil
	_, err = PreflightInputOperation(changed, exec)
	require.ErrorIs(t, err, ErrUnsupportedCapability)
}

func TestInputOperationRejectsGenerationIntentWithoutOptionsSchema(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		edit func(*prompty.PromptExecution)
	}{
		{"max_tokens", func(e *prompty.PromptExecution) { e.ModelOptions = &prompty.ModelOptions{MaxTokens: new(int64(10))} }},
		{"temperature", func(e *prompty.PromptExecution) { e.ModelOptions = &prompty.ModelOptions{Temperature: new(0.5)} }},
		{"top_p", func(e *prompty.PromptExecution) { e.ModelOptions = &prompty.ModelOptions{TopP: new(0.5)} }},
		{"stop", func(e *prompty.PromptExecution) { e.ModelOptions = &prompty.ModelOptions{Stop: []string{"stop"}} }},
		{"provider_settings", func(e *prompty.PromptExecution) {
			e.ModelOptions = &prompty.ModelOptions{ProviderSettings: []byte(`{"reasoning":"on"}`)}
		}},
		{"output_schema", func(e *prompty.PromptExecution) { e.ResponseFormat = &prompty.SchemaDefinition{Schema: []byte(`{}`)} }},
		{"output_mime", func(e *prompty.PromptExecution) { e.Requirements.OutputMIME = []string{"image/png"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exec := prompty.SimplePrompt("resource input")
			test.edit(exec)
			// Act.
			_, err := PreflightInputOperation(fixtureDescriptor(), exec)
			// Assert: generation fields do not disappear merely because schema is absent.
			require.ErrorIs(t, err, ErrUnsupportedCapability)
		})
	}
	_, err := PreflightInputOperation(fixtureDescriptor(), nil)
	require.ErrorIs(t, err, ErrNilExecution)
}
