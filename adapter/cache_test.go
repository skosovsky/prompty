package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestCacheRequiredTTLWithoutMapperFailsBeforeDispatch(t *testing.T) {
	t.Parallel()
	// Arrange: profile overrides cannot invent a request mapping implementation.
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities[prompty.CapabilityCacheMarker] = prompty.CapabilitySupported
	desc.Profiles[0].Capabilities["cache_control:ttl"] = prompty.CapabilitySupported
	fixture := &checkedFixture{descriptor: desc}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired,
		Provider: desc.Name, Controls: prompty.JSONDocument(`{"ttl":"1h"}`),
	}
	// Act.
	_, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrCacheRequirement)
	var cacheErr *prompty.CacheError
	require.ErrorAs(t, err, &cacheErr)
	require.Equal(t, prompty.CacheUnsupported, cacheErr.Decision.Disposition)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestAdvisoryMarkerIsNotImplicitCacheHit(t *testing.T) {
	t.Parallel()
	// Arrange.
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities[prompty.CapabilityCacheImplicit] = prompty.CapabilitySupported
	fixture := &checkedFixture{descriptor: desc}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheBlockMarker}
	// Act.
	response, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert: immutable source intent remains, but no unsupported marker is mapped.
	require.NoError(t, err)
	require.Equal(t, 1, fixture.calls)
	require.Equal(t, []prompty.CacheDecision{
		{
			Path:        "/messages/0/cache_policy",
			Mechanism:   prompty.CacheBlockMarker,
			Disposition: prompty.CacheUnsupported,
			Reason:      "adapter has no block marker mapper",
		},
	}, response.Diagnostics.Cache)
	require.Equal(t, prompty.UsageUnknown, response.Usage.Availability())
	require.NotNil(t, exec.Messages[0].CachePolicy)
	prepared, err := Prepare(fixture, exec, "")
	require.NoError(t, err)
	require.Equal(t, prompty.CacheOmit, prepared.request.Messages[0].CachePolicy.Mechanism)
	response.Diagnostics.Cache[0].Path = "mutated"
	require.Equal(t, "/messages/0/cache_policy", prepared.Diagnostics().Cache[0].Path)
}

func TestCacheImplicitRequiredKnowledgeAndScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		provider string
		support  prompty.CapabilitySupport
		want     prompty.CacheDisposition
	}{
		{"accepted", "fixture", prompty.CapabilitySupported, prompty.CacheAccepted},
		{"unknown", "fixture", prompty.CapabilityUnknown, prompty.CacheUnknown},
		{"unsupported", "fixture", prompty.CapabilityUnsupported, prompty.CacheUnsupported},
		{"foreign", "other", prompty.CapabilitySupported, prompty.CacheNotApplicable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			desc := fixtureDescriptor()
			desc.Profiles[0].Capabilities[prompty.CapabilityCacheImplicit] = tc.support
			fixture := &checkedFixture{descriptor: desc}
			exec := prompty.SimplePrompt("fixture")
			exec.Messages[0].CachePolicy = &prompty.CachePolicy{
				Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: tc.provider,
			}
			// Act.
			prepared, err := Prepare(fixture, exec, "")
			// Assert.
			if tc.want == prompty.CacheAccepted {
				require.NoError(t, err)
				require.Equal(t, prompty.CacheOmit, prepared.request.Messages[0].CachePolicy.Mechanism)
				require.Equal(t, tc.want, prepared.Diagnostics().Cache[0].Disposition)
				return
			}
			require.ErrorIs(t, err, prompty.ErrCacheRequirement)
			var cacheErr *prompty.CacheError
			require.ErrorAs(t, err, &cacheErr)
			require.Equal(t, tc.want, cacheErr.Decision.Disposition)
			require.Zero(t, fixture.translations)
			require.Zero(t, fixture.calls)
		})
	}
}
