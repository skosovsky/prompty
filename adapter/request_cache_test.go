package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type requestCacheFixture struct{ *checkedFixture }

func (*requestCacheFixture) ValidateRequestCacheControl(policy *prompty.CachePolicy) error {
	// Deliberate hostile mapper mutation must not affect source or working intent.
	policy.Controls[0] = 'x'
	return nil
}

func TestRequestCacheDispositionAndIsolation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		support     prompty.CapabilitySupport
		mapper      bool
		required    bool
		combination bool
		want        prompty.CacheDisposition
	}{
		{"accepted", prompty.CapabilitySupported, true, true, false, prompty.CacheAccepted},
		{"unknown advisory", prompty.CapabilityUnknown, true, false, false, prompty.CacheUnknown},
		{"unsupported required", prompty.CapabilityUnsupported, true, true, false, prompty.CacheUnsupported},
		{"missing mapper", prompty.CapabilitySupported, false, true, false, prompty.CacheUnsupported},
		{"forbidden advisory", prompty.CapabilitySupported, true, false, true, prompty.CacheUnsupported},
		{"forbidden required", prompty.CapabilitySupported, true, true, true, prompty.CacheUnsupported},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			descriptor := fixtureDescriptor()
			profile := &descriptor.Profiles[0]
			profile.Capabilities[prompty.CapabilityCacheImplicit] = test.support
			profile.Capabilities["cache_control:key"] = prompty.CapabilitySupported
			if test.combination {
				profile.ForbiddenCombinations = [][]string{{prompty.CapabilityCacheImplicit, "operation"}}
			}
			fixture := &checkedFixture{descriptor: descriptor}
			var runtime RuntimeDescriber = fixture
			if test.mapper {
				runtime = &requestCacheFixture{fixture}
			}
			source := prompty.SimplePrompt("question")
			source.RequestCache = &prompty.CachePolicy{
				Mechanism: prompty.CacheImplicitPrefix,
				Provider:  "fixture",
				Controls:  prompty.JSONDocument(`{"key":"value"}`),
			}
			if test.required {
				source.RequestCache.Mode = prompty.CacheRequired
			}
			working := source.Clone()
			// Act.
			decisions, err := PrepareCacheControls(runtime, working, []string{"operation"})
			// Assert.
			require.Len(t, decisions, 1)
			require.Equal(t, requestCachePath, decisions[0].Path)
			require.Equal(t, test.want, decisions[0].Disposition)
			if test.required && test.want != prompty.CacheAccepted {
				require.ErrorIs(t, err, prompty.ErrCacheRequirement)
			} else {
				require.NoError(t, err)
			}
			if test.want == prompty.CacheAccepted {
				require.Equal(t, source.RequestCache, working.RequestCache)
			} else if !test.required {
				require.Nil(t, working.RequestCache)
			}
			require.Equal(t, prompty.JSONDocument(`{"key":"value"}`), source.RequestCache.Controls)
			require.Zero(t, fixture.translations)
			require.Zero(t, fixture.calls)
		})
	}
}
