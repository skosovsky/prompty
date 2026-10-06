package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestCacheWireAndOwnership(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := SimplePrompt("question")
	exec.RequestCache = &CachePolicy{
		Mechanism: CacheImplicitPrefix, Mode: CacheRequired, Provider: "fixture",
		Controls: JSONDocument(`{"prompt_cache_key":"document","prompt_cache_retention":"in-memory"}`),
	}
	want := cloneCachePolicy(exec.RequestCache)
	// Act.
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	payload, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(payload)
	require.NoError(t, err)
	// Assert.
	require.Equal(t, want, restored.RequestCache)
	require.Contains(t, string(payload), `"request_cache"`)
	for _, independent := range []*PromptExecution{exec.Clone(), exec.Normalize(), restored} {
		require.Equal(t, want, independent.RequestCache)
		independent.RequestCache.Controls[0] = 'x'
		independent.RequestCache.Provider = "changed"
		require.Equal(t, want, exec.RequestCache)
	}
	wire.RequestCache.Controls[0] = 'x'
	require.Equal(t, want, exec.RequestCache)
}

func TestRequestCacheRejectsBlockAndOmissionMechanisms(t *testing.T) {
	t.Parallel()
	for _, mechanism := range []CacheMechanism{CacheBlockMarker, CacheOmit} {
		t.Run(string(mechanism), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exec := SimplePrompt("question")
			exec.RequestCache = &CachePolicy{Mechanism: mechanism}
			// Act.
			_, marshalErr := MarshalExecution(exec, WirePolicy{State: StatePreserve})
			_, restoreErr := UnmarshalExecution(PromptExecutionWire{
				Format: "prompty.execution", RequestCache: exec.RequestCache,
			})
			// Assert.
			require.ErrorIs(t, marshalErr, ErrCachePolicy)
			require.ErrorIs(t, restoreErr, ErrCachePolicy)
		})
	}
}

func TestNormalizePreservesIndependentCacheResourceBinding(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := cacheHandleFixture()
	exec := SimplePrompt("question")
	exec.CacheResource = &handle
	// Act.
	normalized := exec.Normalize()
	// Assert.
	require.Equal(t, exec.CacheResource, normalized.CacheResource)
	normalized.CacheResource.ProviderID = "changed"
	require.Equal(t, "cachedContents/fixture", exec.CacheResource.ProviderID)
}
