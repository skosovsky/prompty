package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestSelectedMalformedProfileRejectsBeforeTranslationAndDispatch(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name   string
		mutate func(*prompty.CapabilityProfile)
	}{
		{"provenance", func(p *prompty.CapabilityProfile) { p.Provenance = " " }},
		{"unrequested state", func(p *prompty.CapabilityProfile) { p.Capabilities["extension"] = "private-invalid-state" }},
		{"blank key", func(p *prompty.CapabilityProfile) { p.Capabilities[" "] = prompty.CapabilitySupported }},
		{"empty combination", func(p *prompty.CapabilityProfile) { p.ForbiddenCombinations = [][]string{{}} }},
		{"duplicate combination", func(p *prompty.CapabilityProfile) { p.ForbiddenCombinations = [][]string{{"tools", "tools"}} }},
		{"blank MIME", func(p *prompty.CapabilityProfile) { p.InputMIME = []string{" "} }},
		{"invalid schema shape", func(p *prompty.CapabilityProfile) { p.ModelOptionsSchema = prompty.JSONDocument(`null`) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			for _, override := range []bool{false, true} {
				for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
					profile := fixtureDescriptor().Profiles[0]
					fixture.mutate(&profile)
					assertMalformedSelectedProfile(t, profile, override, mode)
				}
			}
		})
	}
}

func assertMalformedSelectedProfile(
	t *testing.T,
	profile prompty.CapabilityProfile,
	override bool,
	mode prompty.StreamMode,
) {
	t.Helper()
	// Arrange: selected knowledge is invalid even when its bad feature is not requested.
	adapter := &checkedFixture{descriptor: fixtureDescriptor()}
	execution := prompty.SimplePrompt("private prompt")
	want := ErrInvalidProfile
	if override {
		execution.Requirements.Override, want = &profile, ErrInvalidProfileOverride
	} else {
		adapter.descriptor.Profiles[0] = profile
	}
	// Act.
	client := NewClient(adapter)
	var err error
	if mode == "" {
		_, err = client.Execute(context.Background(), execution)
	} else {
		stream := client.ExecuteStream(context.Background(), execution, mode)
		for _, streamErr := range stream.Events() {
			err = streamErr
		}
	}
	// Assert.
	require.ErrorIs(t, err, want)
	require.ErrorIs(t, err, prompty.ErrCapabilityProfile)
	require.ErrorIs(t, err, ErrCapabilityRequirement)
	require.NotContains(t, err.Error(), "private")
	require.Zero(t, adapter.translations)
	require.Zero(t, adapter.calls)
}

func TestDuplicateDestinationProfilesAreNotSelectedByOrder(t *testing.T) {
	t.Parallel()
	for _, reversed := range []bool{false, true} {
		// Arrange: contradictory knowledge must never be selected by slice order.
		descriptor := fixtureDescriptor()
		other := prompty.CloneCapabilityProfile(descriptor.Profiles[0])
		other.Capabilities[prompty.CapabilityTools] = prompty.CapabilityUnsupported
		descriptor.Profiles = append(descriptor.Profiles, other)
		if reversed {
			descriptor.Profiles[0], descriptor.Profiles[1] = descriptor.Profiles[1], descriptor.Profiles[0]
		}
		adapter := &checkedFixture{descriptor: descriptor}
		execution := prompty.SimplePrompt("fixture")
		execution.Requirements.Capabilities = []string{prompty.CapabilityTools}
		// Act.
		_, err := NewClient(adapter).Execute(context.Background(), execution)
		// Assert.
		require.ErrorIs(t, err, ErrInvalidProfile)
		require.Zero(t, adapter.translations)
		require.Zero(t, adapter.calls)
	}
}

func TestExplicitProfileOverrideReplacesAmbiguousDescriptorKnowledge(t *testing.T) {
	t.Parallel()
	// Arrange: override is an explicit choice, not another matching candidate.
	descriptor := fixtureDescriptor()
	profile := prompty.CloneCapabilityProfile(descriptor.Profiles[0])
	descriptor.Profiles = append(descriptor.Profiles, prompty.CloneCapabilityProfile(profile))
	adapter := &checkedFixture{descriptor: descriptor}
	execution := prompty.SimplePrompt("fixture")
	execution.Requirements.Override = &profile
	execution.Requirements.Capabilities = []string{prompty.CapabilityTools}
	// Act.
	response, err := NewClient(adapter).Execute(context.Background(), execution)
	// Assert: one checked dispatch, with override explicitly recorded.
	require.NoError(t, err)
	require.True(t, response.Diagnostics.Override)
	require.Equal(t, 1, adapter.translations)
	require.Equal(t, 1, adapter.calls)
}

func TestAmbiguousMalformedProfilesHaveOrderIndependentDiagnostics(t *testing.T) {
	t.Parallel()
	// Arrange: detect ambiguity before inspecting either candidate's knowledge.
	descriptor := fixtureDescriptor()
	malformed := prompty.CloneCapabilityProfile(descriptor.Profiles[0])
	malformed.Provenance = ""
	descriptor.Profiles = append(descriptor.Profiles, malformed)
	execution := prompty.SimplePrompt("fixture")
	// Act.
	first, firstErr := Preflight(descriptor, execution, "", false)
	descriptor.Profiles[0], descriptor.Profiles[1] = descriptor.Profiles[1], descriptor.Profiles[0]
	second, secondErr := Preflight(descriptor, execution, "", false)
	// Assert: both orders produce the same decision, not first-match behavior.
	require.ErrorIs(t, firstErr, ErrInvalidProfile)
	require.ErrorIs(t, secondErr, ErrInvalidProfile)
	require.Equal(t, first.Checks, second.Checks)
	require.Equal(t, firstErr.Error(), secondErr.Error())
}

func TestUnrelatedProfilesDoNotEstablishOrInvalidateDestinationKnowledge(t *testing.T) {
	t.Parallel()
	// Arrange: caller-owned descriptors are not global provider catalogs.
	descriptor := fixtureDescriptor()
	descriptor.Profiles[0].Provenance = ""
	execution := prompty.SimplePrompt("fixture")
	execution.ModelOptions = &prompty.ModelOptions{Model: "unknown"}
	execution.Requirements.Capabilities = []string{prompty.CapabilityTools}
	// Act.
	report, err := Preflight(descriptor, execution, "", false)
	// Assert: unrelated malformed data cannot supply this destination's guarantees.
	require.ErrorIs(t, err, ErrUnknownProfile)
	require.NotErrorIs(t, err, ErrInvalidProfile)
	require.Equal(t, "unknown", report.Profile.Model)
}

func TestUnknownEffectiveModelRejectsNativeStreamBeforeDispatch(t *testing.T) {
	t.Parallel()
	// Arrange: adapter has native transport, but no knowledge about this effective model.
	adapter := &checkedFixture{descriptor: fixtureDescriptor()}
	execution := prompty.SimplePrompt("fixture")
	execution.ModelOptions = &prompty.ModelOptions{Model: "unknown"}
	stream := NewClient(
		nativeCheckedFixture{adapter},
	).ExecuteStream(context.Background(), execution, prompty.StreamNative)
	// Act.
	var err error
	for _, streamErr := range stream.Events() {
		err = streamErr
	}
	// Assert: actual transport availability is not model capability evidence.
	require.ErrorIs(t, err, ErrUnknownProfile)
	require.Equal(t, prompty.StreamFailed, stream.Status().State)
	require.Zero(t, adapter.translations)
	require.Zero(t, adapter.calls)
}
