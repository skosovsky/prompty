package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func requiredAnnotationExecution(desc RuntimeDescriptor) *prompty.PromptExecution {
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].MessageAnnotations = []prompty.MessageAnnotation{
		{
			Kind:     "fixture.history",
			Value:    prompty.JSONDocument(`{"owned":"snapshot"}`),
			Required: true,
			Scope:    new(desc.Profiles[0].Identity),
		},
	}
	return exec
}

type annotationStateFixture struct {
	*checkedFixture

	stateChecks int
}

func (a *annotationStateFixture) ValidateProviderState(exec *prompty.PromptExecution, _ prompty.ProfileIdentity) error {
	a.stateChecks++
	if string(exec.Messages[0].MessageAnnotations[0].Value) != `{"owned":"snapshot"}` {
		return prompty.ErrProviderState
	}
	return nil
}

func TestRequiredMessageMetadataNeedsActualMappingAndStatePort(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		mapping bool
		support prompty.CapabilitySupport
		want    error
	}{
		{"no mapping", false, prompty.CapabilitySupported, ErrUnsupportedCapability},
		{"unknown profile", true, prompty.CapabilityUnknown, ErrUnknownProfile},
		{"no state port", true, prompty.CapabilitySupported, prompty.ErrProviderState},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: caller profile knowledge cannot manufacture a translator.
			desc := fixtureDescriptor()
			desc.Profiles[0].Capabilities[prompty.CapabilityProviderState] = test.support
			if test.mapping {
				desc.MessageAnnotationCapabilities = map[string][]string{"fixture.history": {}}
			}
			fixture := &checkedFixture{descriptor: desc}
			exec := requiredAnnotationExecution(desc)
			// Act.
			_, err := NewClient(fixture).Execute(t.Context(), exec)
			// Assert.
			require.ErrorIs(t, err, test.want)
			require.Zero(t, fixture.translations)
			require.Zero(t, fixture.calls)
			if !test.mapping {
				require.ErrorIs(t, ValidateMessageAnnotationMappings(exec, desc), ErrUnsupportedCapability)
			}
		})
	}
}

func TestMessageOnlyStateHasOwnedDispatchSnapshot(t *testing.T) {
	t.Parallel()
	// Arrange: there is no PartState, but the provider's message payload is required.
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	desc.MessageAnnotationCapabilities = map[string][]string{"fixture.history": {}}
	fixture := &annotationStateFixture{checkedFixture: &checkedFixture{descriptor: desc}}
	exec := requiredAnnotationExecution(desc)
	prepared, err := Prepare(fixture, exec, prompty.StreamBuffered)
	require.NoError(t, err)
	// Act: mutating original inputs cannot replace prepared state; dispatch revalidates it.
	exec.Messages[0].MessageAnnotations[0].Value = prompty.JSONDocument(`{"changed":true}`)
	exec.Messages[0].MessageAnnotations[0].Scope.Endpoint = "https://changed.invalid"
	err = validatePreparedDispatch(fixture, prepared)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, 2, fixture.stateChecks)
	require.Equal(
		t,
		"https://fixture.invalid",
		prepared.stateExecution.Messages[0].MessageAnnotations[0].Scope.Endpoint,
	)
	require.Zero(t, fixture.calls)
}
