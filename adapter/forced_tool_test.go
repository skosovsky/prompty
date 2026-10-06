package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type forcedToolFixture struct {
	nativeCheckedFixture

	supported bool
}

func (f forcedToolFixture) SupportsForcedTool() bool { return f.supported }

func TestForcedToolKnowledgeAndMappingAreSeparate(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, state := range []prompty.CapabilitySupport{
			prompty.CapabilityUnknown, prompty.CapabilityUnsupported, prompty.CapabilitySupported,
		} {
			t.Run(string(mode)+"/"+string(state), func(t *testing.T) {
				t.Parallel()
				// Arrange: an override can supply knowledge, never an actual mapping port.
				fixture := &checkedFixture{descriptor: fixtureDescriptor()}
				exec := prompty.SimplePrompt("private prompt")
				exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`true`)}}
				exec.ForcedTool = "lookup"
				profile := fixtureDescriptor().Profiles[0]
				profile.Capabilities[prompty.CapabilityForcedToolSelection] = state
				exec.Requirements.Override = &profile
				// Act: native fixture has a stream method, but no forced selection mapping.
				prepared, err := Prepare(nativeCheckedFixture{fixture}, exec, mode)
				// Assert.
				require.Nil(t, prepared)
				require.ErrorIs(t, err, ErrCapabilityRequirement)
				if state == prompty.CapabilityUnknown {
					require.ErrorIs(t, err, ErrUnknownProfile)
				} else {
					require.ErrorIs(t, err, ErrUnsupportedCapability)
				}
				require.NotContains(t, err.Error(), "private prompt")
				require.Zero(t, fixture.translations)
				require.Zero(t, fixture.calls)
			})
		}
	}
}

func TestForcedToolImplementedMappingNeedsProfile(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, supported := range []bool{false, true} {
			t.Run(
				string(mode)+map[bool]string{false: "/port declines", true: "/port maps"}[supported],
				func(t *testing.T) {
					t.Parallel()
					// Arrange.
					fixture := &checkedFixture{descriptor: fixtureDescriptor()}
					fixture.descriptor.Profiles[0].Capabilities[prompty.CapabilityForcedToolSelection] = prompty.CapabilitySupported
					exec := prompty.SimplePrompt("fixture")
					exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`true`)}}
					exec.ForcedTool = "lookup"
					// Act.
					prepared, err := Prepare(
						forcedToolFixture{nativeCheckedFixture: nativeCheckedFixture{fixture}, supported: supported},
						exec,
						mode,
					)
					// Assert: only the combination of declared knowledge and implementation translates.
					require.Zero(t, fixture.calls)
					if !supported {
						require.ErrorIs(t, err, ErrUnsupportedCapability)
						require.Nil(t, prepared)
						require.Zero(t, fixture.translations)
						return
					}
					require.NoError(t, err)
					require.Equal(t, 1, fixture.translations)
					require.Equal(t, "lookup", prepared.request.ForcedTool)
					require.Contains(t, prepared.Diagnostics().Checks, prompty.RequirementCheck{
						Requirement: prompty.CapabilityForcedToolSelection,
						Status:      prompty.CapabilitySupported,
						Reason:      "",
					})
				},
			)
		}
	}
}
