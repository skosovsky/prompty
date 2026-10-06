package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestShadowedMessageCacheDoesNotRequireUnusedControls(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: no block inherits the unknown required default.
			fixture := &checkedFixture{descriptor: fixtureDescriptor()}
			exec := prompty.SimplePrompt("fixture")
			exec.Messages[0].CachePolicy = &prompty.CachePolicy{
				Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "fixture",
				Controls: prompty.JSONDocument(`{"ttl":"private-value"}`),
			}
			exec.Messages[0].Content[0] = prompty.TextPart{Text: "fixture", CachePolicy: &prompty.CachePolicy{
				Mechanism: prompty.CacheOmit, Mode: "", Provider: "", Controls: nil,
			}}
			// Act.
			prepared, err := Prepare(nativeCheckedFixture{fixture}, exec, mode)
			// Assert: unsupported mapping/knowledge of an unused default cannot stop the request.
			require.NoError(t, err)
			require.Equal(t, 1, fixture.translations)
			require.Zero(t, fixture.calls)
			require.Equal(t, prompty.CacheNotApplicable, prepared.Diagnostics().Cache[0].Disposition)
			require.Equal(t, "/messages/0/cache_policy", prepared.Diagnostics().Cache[0].Path)
			require.Equal(t, prompty.CacheOmit, prepared.request.Messages[0].CachePolicy.Mechanism)
			require.JSONEq(t, `{"ttl":"private-value"}`, string(exec.Messages[0].CachePolicy.Controls))
		})
	}
}

func TestPartiallyInheritedRequiredCacheStillFails(t *testing.T) {
	t.Parallel()
	// Arrange: a second block still uses the message default.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "fixture", Controls: nil,
	}
	exec.Messages[0].Content = []prompty.ContentPart{
		prompty.TextPart{Text: "override", CachePolicy: &prompty.CachePolicy{Mechanism: prompty.CacheOmit}},
		prompty.TextPart{Text: "inherit", CachePolicy: nil},
	}
	// Act.
	prepared, err := Prepare(nativeCheckedFixture{fixture}, exec, prompty.StreamNative)
	// Assert.
	require.Nil(t, prepared)
	require.ErrorIs(t, err, prompty.ErrCacheRequirement)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestShadowedCacheControlDoesNotCreateCombinationFailure(t *testing.T) {
	t.Parallel()
	// Arrange: supported but unused TTL must not conflict with requested tools.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	profile := &fixture.descriptor.Profiles[0]
	profile.Capabilities[prompty.CapabilityCacheMarker] = prompty.CapabilitySupported
	profile.Capabilities["cache_control:ttl"] = prompty.CapabilitySupported
	profile.ForbiddenCombinations = [][]string{{prompty.CapabilityTools, "cache_control:ttl"}}
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`true`)}}
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "fixture",
		Controls: prompty.JSONDocument(`{"ttl":"fixture"}`),
	}
	exec.Messages[0].Content[0] = prompty.TextPart{
		Text:        "override",
		CachePolicy: &prompty.CachePolicy{Mechanism: prompty.CacheOmit},
	}
	// Act.
	prepared, err := Prepare(
		cacheMappingFixture{nativeCheckedFixture: nativeCheckedFixture{fixture}, mutate: false},
		exec,
		"",
	)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, 1, fixture.translations)
	require.Equal(t, prompty.CacheNotApplicable, prepared.Diagnostics().Cache[0].Disposition)
	require.Zero(t, fixture.calls)
}

func TestShadowedMalformedCachePolicyStillFailsShapeValidation(t *testing.T) {
	t.Parallel()
	// Arrange: omission of an unused default does not permit corrupt portable input.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "fixture",
		Controls: prompty.JSONDocument(`null`),
	}
	exec.Messages[0].Content[0] = prompty.TextPart{
		Text:        "override",
		CachePolicy: &prompty.CachePolicy{Mechanism: prompty.CacheOmit},
	}
	// Act.
	prepared, err := Prepare(nativeCheckedFixture{fixture}, exec, "")
	// Assert.
	require.Nil(t, prepared)
	require.ErrorIs(t, err, prompty.ErrCachePolicy)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}
