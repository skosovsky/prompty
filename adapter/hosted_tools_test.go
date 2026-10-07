package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestHostedToolsRequireAdapterMappingAndProfileBeforeIO(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		mapping   bool
		status    prompty.CapabilitySupport
		forbidden bool
		want      error
	}{
		{"unimplemented", false, prompty.CapabilitySupported, false, ErrUnsupportedCapability},
		{"unknown", true, prompty.CapabilityUnknown, false, ErrUnknownProfile},
		{"unsupported", true, prompty.CapabilityUnsupported, false, ErrUnsupportedCapability},
		{"combination", true, prompty.CapabilitySupported, true, ErrUnsupportedCombination},
		{"supported", true, prompty.CapabilitySupported, false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: profile support cannot manufacture an adapter mapping.
			desc := fixtureDescriptor()
			desc.Profiles[0].Capabilities["hosted_fixture"] = test.status
			if test.mapping {
				desc.HostedToolCapabilities = map[string]string{"fixture.search": "hosted_fixture"}
			}
			if test.forbidden {
				desc.Profiles[0].ForbiddenCombinations = [][]string{{"hosted_fixture"}}
			}
			fixture := &checkedFixture{descriptor: desc}
			exec := prompty.SimplePrompt("fixture")
			exec.HostedTools = []prompty.HostedToolRequest{{Kind: "fixture.search"}}
			// Act.
			_, err := NewClient(fixture).Execute(t.Context(), exec)
			// Assert.
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
				require.Zero(t, fixture.translations)
				require.Zero(t, fixture.calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, fixture.calls)
			}
		})
	}
}

func TestHostedMappingDescriptorOwnership(t *testing.T) {
	t.Parallel()
	// Arrange.
	desc := fixtureDescriptor()
	desc.HostedToolCapabilities = map[string]string{"fixture.search": "hosted_fixture"}
	desc.MessageAnnotationCapabilities = map[string][]string{"fixture.history": {"hosted_fixture"}}
	// Act.
	owned := CloneRuntimeDescriptor(desc)
	owned.HostedToolCapabilities["fixture.search"] = "changed"
	owned.MessageAnnotationCapabilities["fixture.history"][0] = "changed"
	// Assert.
	require.Equal(t, "hosted_fixture", desc.HostedToolCapabilities["fixture.search"])
	require.Equal(t, []string{"hosted_fixture"}, desc.MessageAnnotationCapabilities["fixture.history"])
}

func TestHistoryAnnotationCapabilitiesParticipateInNativeCombinations(t *testing.T) {
	t.Parallel()
	// Arrange: provider history requires a feature even when no new tool is declared.
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities["hosted_fixture"] = prompty.CapabilitySupported
	desc.Profiles[0].ForbiddenCombinations = [][]string{{"hosted_fixture", prompty.CapabilityNativeStream}}
	desc.MessageAnnotationCapabilities = map[string][]string{"fixture.history": {"hosted_fixture"}}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].MessageAnnotations = []prompty.MessageAnnotation{
		{Kind: "fixture.history", Value: prompty.JSONDocument(`{}`)},
	}
	// Act.
	_, err := Preflight(desc, exec, prompty.StreamNative, true)
	// Assert.
	require.ErrorIs(t, err, ErrUnsupportedCombination)
}
