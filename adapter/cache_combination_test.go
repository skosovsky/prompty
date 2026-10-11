package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type cacheMappingFixture struct {
	nativeCheckedFixture

	mutate bool
}

func (f cacheMappingFixture) ValidateCacheControl(policy *prompty.CachePolicy) error {
	if f.mutate {
		policy.Provider = "foreign"
		policy.Mode = prompty.CacheAdvisory
		policy.Controls[0] = 'x'
	}
	return nil
}

func TestSelectedCacheCombinations(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, policyMode := range []prompty.CacheMode{prompty.CacheRequired, prompty.CacheAdvisory} {
			for _, feature := range []string{prompty.CapabilityTools, prompty.CapabilityStructuredOutput, prompty.CapabilityReasoning} {
				t.Run(string(mode)+"/"+string(policyMode)+"/"+feature, func(t *testing.T) {
					t.Parallel()
					// Arrange: individual support does not establish combined support.
					fixture := &checkedFixture{descriptor: fixtureDescriptor()}
					profile := &fixture.descriptor.Profiles[0]
					profile.Capabilities[prompty.CapabilityCacheMarker] = prompty.CapabilitySupported
					profile.Capabilities["cache_control:ttl"] = prompty.CapabilitySupported
					profile.ForbiddenCombinations = [][]string{{feature, "cache_control:ttl"}}
					exec := prompty.SimplePrompt("private prompt")
					exec.Messages[0].CachePolicy = &prompty.CachePolicy{
						Mechanism: prompty.CacheBlockMarker, Mode: policyMode, Provider: "fixture",
						Controls: prompty.JSONDocument(`{"ttl":"private-value"}`),
					}
					switch feature {
					case prompty.CapabilityTools:
						exec.Tools = []prompty.ToolDefinition{
							{Name: "lookup", Parameters: prompty.JSONDocument(`true`)},
						}
					case prompty.CapabilityStructuredOutput:
						exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`true`)}
					case prompty.CapabilityReasoning:
						fixture.descriptor.ReasoningSettings = []string{"thinking_mode"}
						profile.ModelOptionsSchema = prompty.JSONDocument(`true`)
						exec.ModelOptions = &prompty.ModelOptions{
							ProviderSettings: prompty.JSONDocument(`{"thinking_mode":"fixture"}`),
						}
					}
					before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
					require.NoError(t, err)
					// Act.
					prepared, err := Prepare(
						cacheMappingFixture{nativeCheckedFixture: nativeCheckedFixture{fixture}, mutate: false},
						exec,
						mode,
					)
					// Assert.
					require.Zero(t, fixture.calls)
					if policyMode == prompty.CacheRequired {
						require.Nil(t, prepared)
						require.ErrorIs(t, err, prompty.ErrCacheRequirement)
						require.ErrorIs(t, err, ErrUnsupportedCombination)
						require.NotContains(t, err.Error(), "private")
						require.Zero(t, fixture.translations)
					} else {
						require.NoError(t, err)
						require.Equal(t, 1, fixture.translations)
						require.Equal(t, prompty.CacheOmit, prepared.request.Messages[0].CachePolicy.Mechanism)
						require.Equal(t, prompty.CacheUnsupported, prepared.Diagnostics().Cache[0].Disposition)
					}
					after, marshalErr := prompty.MarshalExecution(
						exec,
						prompty.WirePolicy{State: prompty.StatePreserve},
					)
					require.NoError(t, marshalErr)
					require.Equal(t, before, after)
				})
			}
		}
	}
}

func TestCacheMapperCannotRewriteSelectedIntent(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	profile := &fixture.descriptor.Profiles[0]
	profile.Capabilities[prompty.CapabilityCacheMarker] = prompty.CapabilitySupported
	profile.Capabilities["cache_control:ttl"] = prompty.CapabilitySupported
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "fixture",
		Controls: prompty.JSONDocument(`{"ttl":"fixture"}`),
	}
	// Act.
	prepared, err := Prepare(
		cacheMappingFixture{nativeCheckedFixture: nativeCheckedFixture{fixture}, mutate: true},
		exec,
		"",
	)
	// Assert: mapper mutation affects neither the enforcement nor the translated intent/source.
	require.NoError(t, err)
	require.Equal(t, exec.Messages[0].CachePolicy, prepared.request.Messages[0].CachePolicy)
	require.Equal(t, prompty.CacheAccepted, prepared.Diagnostics().Cache[0].Disposition)
	require.Equal(t, 1, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestCacheCombinationAcrossNestedPolicies(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, implicitMode := range []prompty.CacheMode{prompty.CacheRequired, prompty.CacheAdvisory} {
			for _, markerMode := range []prompty.CacheMode{prompty.CacheRequired, prompty.CacheAdvisory} {
				t.Run(string(mode)+"/"+string(implicitMode)+"/"+string(markerMode), func(t *testing.T) {
					t.Parallel()
					// Arrange: controls span messages/nested tool-result parts; one marker is independent.
					fixture := &checkedFixture{descriptor: fixtureDescriptor()}
					profile := &fixture.descriptor.Profiles[0]
					profile.Capabilities[prompty.CapabilityCacheImplicit] = prompty.CapabilitySupported
					profile.Capabilities[prompty.CapabilityCacheMarker] = prompty.CapabilitySupported
					profile.Capabilities["cache_control:ttl"] = prompty.CapabilitySupported
					profile.ForbiddenCombinations = [][]string{{prompty.CapabilityCacheImplicit, "cache_control:ttl"}}
					exec := prompty.SimplePrompt("fixture")
					exec.Messages[0].CachePolicy = &prompty.CachePolicy{
						Mechanism: prompty.CacheImplicitPrefix, Mode: implicitMode, Provider: "fixture", Controls: nil,
					}
					exec.Messages = append(exec.Messages, prompty.ChatMessage{
						Role: prompty.RoleTool, Content: []prompty.ContentPart{prompty.ToolResultPart{
							ToolCallID: "call", Name: "lookup", IsError: false, CachePolicy: nil,
							Content: []prompty.ContentPart{
								prompty.TextPart{Text: "controlled", CachePolicy: &prompty.CachePolicy{
									Mechanism: prompty.CacheBlockMarker, Mode: markerMode, Provider: "fixture",
									Controls: prompty.JSONDocument(`{"ttl":"fixture"}`),
								}},
								prompty.TextPart{Text: "independent", CachePolicy: &prompty.CachePolicy{
									Mechanism: prompty.CacheBlockMarker,
									Mode:      prompty.CacheRequired,
									Provider:  "fixture",
									Controls:  nil,
								}},
							},
						}},
					})
					// Act.
					prepared, err := Prepare(
						cacheMappingFixture{nativeCheckedFixture: nativeCheckedFixture{fixture}, mutate: false},
						exec,
						mode,
					)
					// Assert: omit advisory conflicts first; required independent intent survives.
					assertNestedCacheCombination(t, prepared, err, fixture, implicitMode, markerMode)
					require.NotNil(t, exec.Messages[0].CachePolicy)
					require.NotNil(
						t,
						exec.Messages[1].Content[0].(prompty.ToolResultPart).Content[0].(prompty.TextPart).CachePolicy,
					)
				})
			}
		}
	}
}

func assertNestedCacheCombination(
	t *testing.T,
	prepared *PreparedRequest[*prompty.PromptExecution],
	err error,
	fixture *checkedFixture,
	implicitMode, markerMode prompty.CacheMode,
) {
	t.Helper()
	require.Zero(t, fixture.calls)
	if implicitMode == prompty.CacheRequired && markerMode == prompty.CacheRequired {
		require.ErrorIs(t, err, prompty.ErrCacheRequirement)
		require.ErrorIs(t, err, ErrUnsupportedCombination)
		require.Nil(t, prepared)
		require.Zero(t, fixture.translations)
		return
	}
	require.NoError(t, err)
	decisions := prepared.Diagnostics().Cache
	require.Len(t, decisions, 3)
	require.Equal(t, "/messages/1/content/0/content/0/cache_policy", decisions[1].Path)
	for index, policyMode := range []prompty.CacheMode{implicitMode, markerMode} {
		if policyMode == prompty.CacheAdvisory {
			require.Equal(t, prompty.CacheUnsupported, decisions[index].Disposition)
		} else {
			require.Equal(t, prompty.CacheAccepted, decisions[index].Disposition)
		}
	}
	require.Equal(t, prompty.CacheAccepted, decisions[2].Disposition)
	nested := prepared.request.Messages[1].Content[0].(prompty.ToolResultPart).Content
	require.NotNil(t, nested[1].(prompty.TextPart).CachePolicy)
	if markerMode == prompty.CacheAdvisory {
		require.Equal(t, prompty.CacheOmit, nested[0].(prompty.TextPart).CachePolicy.Mechanism)
	} else {
		require.NotNil(t, nested[0].(prompty.TextPart).CachePolicy)
	}
}

func TestNativeStreamCacheCombination(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	profile := &fixture.descriptor.Profiles[0]
	profile.Capabilities[prompty.CapabilityCacheImplicit] = prompty.CapabilitySupported
	profile.ForbiddenCombinations = [][]string{{prompty.CapabilityNativeStream, prompty.CapabilityCacheImplicit}}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheImplicitPrefix,
		Mode:      prompty.CacheRequired,
	}
	// Act.
	prepared, err := Prepare(nativeCheckedFixture{fixture}, exec, prompty.StreamNative)
	// Assert.
	require.Nil(t, prepared)
	require.ErrorIs(t, err, ErrUnsupportedCombination)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestUnselectedAdvisoryCacheDoesNotBlockTools(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"no mapper", "unknown TTL", "foreign provider"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: omitted intent is not a selected feature in the combination graph.
			fixture := &checkedFixture{descriptor: fixtureDescriptor()}
			profile := &fixture.descriptor.Profiles[0]
			profile.Capabilities[prompty.CapabilityCacheMarker] = prompty.CapabilitySupported
			profile.ForbiddenCombinations = [][]string{{prompty.CapabilityTools, prompty.CapabilityCacheMarker}}
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`true`)}}
			exec.Messages[0].CachePolicy = &prompty.CachePolicy{
				Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheAdvisory, Provider: "fixture",
				Controls: prompty.JSONDocument(`{"ttl":"fixture"}`),
			}
			var adp ProviderAdapter[*prompty.PromptExecution, *prompty.Response] = nativeCheckedFixture{fixture}
			want := prompty.CacheUnsupported
			if scenario != "no mapper" {
				adp = cacheMappingFixture{nativeCheckedFixture: nativeCheckedFixture{fixture}, mutate: false}
				want = prompty.CacheUnknown
			}
			if scenario == "foreign provider" {
				exec.Messages[0].CachePolicy.Provider = "foreign"
				want = prompty.CacheNotApplicable
			}
			// Act.
			prepared, err := Prepare(adp, exec, prompty.StreamNative)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, 1, fixture.translations)
			require.Zero(t, fixture.calls)
			require.Equal(t, prompty.CacheOmit, prepared.request.Messages[0].CachePolicy.Mechanism)
			require.Equal(t, want, prepared.Diagnostics().Cache[0].Disposition)
		})
	}
}
