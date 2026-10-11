package prompty

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func cacheHandleFixture() CacheResourceHandle {
	return CacheResourceHandle{
		Format:         CacheResourceWireFormat,
		Identity:       ProfileIdentity{Provider: "gemini", Endpoint: "https://example.invalid", Model: "fixture"},
		AccountBinding: "account-fixture", ProviderID: "cachedContents/fixture", ContentDigest: strings.Repeat("a", 64),
		ExpiresAt: time.Now().Add(time.Hour).UTC(), State: CacheResourceActive,
	}
}

func TestCacheResourceHandleRoundTripAndScope(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := cacheHandleFixture()
	// Act.
	document, err := EncodeCacheResourceHandle(handle)
	require.NoError(t, err)
	restored, err := DecodeCacheResourceHandle(document)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, handle, restored)
	require.NoError(t, ValidateCacheResourceReuse(restored, handle.Identity, handle.AccountBinding, time.Now()))
	require.ErrorIs(
		t,
		ValidateCacheResourceReuse(restored, handle.Identity, "other-account", time.Now()),
		ErrCacheResourceScope,
	)
	wrongModel := handle.Identity
	wrongModel.Model = "other"
	require.ErrorIs(
		t,
		ValidateCacheResourceReuse(restored, wrongModel, handle.AccountBinding, time.Now()),
		ErrCacheResourceScope,
	)
	require.ErrorIs(
		t,
		ValidateCacheResourceReuse(restored, handle.Identity, handle.AccountBinding, handle.ExpiresAt),
		ErrCacheResourceExpired,
	)
	restored.State = CacheResourceUnknown
	require.ErrorIs(
		t,
		ValidateCacheResourceReuse(restored, handle.Identity, handle.AccountBinding, time.Now()),
		ErrCacheResourceExpired,
	)
	require.Equal(t, CacheResourceActive, handle.State)
	exec := SimplePrompt("question")
	exec.CacheResource = &handle
	cloned := exec.Clone()
	cloned.CacheResource.ProviderID = "cachedContents/changed"
	require.Equal(t, "cachedContents/fixture", handle.ProviderID)
	wire, wireErr := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, wireErr)
	rebound, wireErr := UnmarshalExecution(wire)
	require.NoError(t, wireErr)
	require.Equal(t, handle, *rebound.CacheResource)
	rebound.CacheResource.AccountBinding = "changed"
	require.Equal(t, "account-fixture", wire.CacheResource.AccountBinding)
}

func TestCacheResourceInvalidWireAndExpiry(t *testing.T) {
	t.Parallel()
	// Arrange: missing binding, unknown state and credential-bearing endpoints are not restorable.
	for _, mutate := range []func(*CacheResourceHandle){
		func(h *CacheResourceHandle) { h.Identity.Endpoint = "" },
		func(h *CacheResourceHandle) { h.Identity.Endpoint = "https://user:secret@example.invalid" },
		func(h *CacheResourceHandle) { h.ContentDigest = "bad" },
		func(h *CacheResourceHandle) { h.State = "future" },
		func(h *CacheResourceHandle) { h.AccountBinding = " " },
		func(h *CacheResourceHandle) { h.Format = "legacy" },
	} {
		handle := cacheHandleFixture()
		mutate(&handle)
		// Act.
		_, err := EncodeCacheResourceHandle(handle)
		// Assert.
		require.ErrorIs(t, err, ErrCacheResource)
	}
	now := time.Now()
	for _, expiry := range []CacheResourceExpiry{{}, {TTL: -time.Second}, {TTL: time.Second, ExpiresAt: now.Add(time.Hour)}, {ExpiresAt: now}} {
		require.ErrorIs(t, ValidateCacheResourceExpiry(expiry, now), ErrCacheResourceExpiry)
	}
	require.NoError(t, ValidateCacheResourceExpiry(CacheResourceExpiry{TTL: time.Second}, now))
	require.NoError(t, ValidateCacheResourceExpiry(CacheResourceExpiry{ExpiresAt: now.Add(time.Hour)}, now))
}

func TestUnknownCacheResourceCanPersistUnknownExpiryForReconciliation(t *testing.T) {
	t.Parallel()
	// Arrange: SDK returned identity while expiration remained unknown.
	handle := cacheHandleFixture()
	handle.State = CacheResourceUnknown
	handle.ExpiresAt = time.Time{}
	// Act.
	encoded, err := EncodeCacheResourceHandle(handle)
	require.NoError(t, err)
	restored, err := DecodeCacheResourceHandle(encoded)
	// Assert: persist evidence, never authorize generation reuse or invent an active lease.
	require.NoError(t, err)
	require.Equal(t, handle, restored)
	require.ErrorIs(
		t,
		ValidateCacheResourceReuse(restored, handle.Identity, handle.AccountBinding, time.Now()),
		ErrCacheResourceExpired,
	)
	for _, state := range []CacheResourceState{CacheResourceActive, CacheResourceExpired, CacheResourceDeleted} {
		invalid := handle
		invalid.State = state
		require.ErrorIs(t, ValidateCacheResourceHandle(invalid), ErrCacheResource)
	}
}
