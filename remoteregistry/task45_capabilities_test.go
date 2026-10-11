package remoteregistry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type minimalCapabilityRegistry struct{}

func (minimalCapabilityRegistry) Plan(context.Context, string, prompty.RegistryPlanInput) (*prompty.RenderPlan, error) {
	return nil, prompty.ErrTemplateNotFound
}

type emptyCatalogRegistry struct{ minimalCapabilityRegistry }

func (emptyCatalogRegistry) List(context.Context) ([]string, error) { return []string{}, nil }
func (emptyCatalogRegistry) Stat(context.Context, string) (prompty.TemplateInfo, error) {
	return prompty.TemplateInfo{}, prompty.ErrTemplateNotFound
}

func TestCacheOptionalCapabilitiesAreExplicit(t *testing.T) {
	t.Parallel()
	// Arrange.
	registry := WithCache(minimalCapabilityRegistry{}, time.Minute)
	// Act.
	ids, listErr := registry.List(t.Context())
	_, statErr := registry.Stat(t.Context(), "known")
	_, captureErr := registry.CaptureManifest(t.Context(), "known")
	_, describeErr := registry.DescribePrompt(t.Context(), "known")
	_, resolveErr := registry.ResolveManifest(t.Context(), "known")
	// Assert: missing capability differs from an empty supported catalog/missing ID.
	require.Nil(t, ids)
	for _, err := range []error{listErr, statErr, captureErr, describeErr, resolveErr} {
		require.ErrorIs(t, err, prompty.ErrRegistryCapabilityUnavailable)
		require.NotErrorIs(t, err, prompty.ErrTemplateNotFound)
	}
	supported := WithCache(emptyCatalogRegistry{}, time.Minute)
	ids, err := supported.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, ids)
	_, err = supported.Stat(t.Context(), "missing")
	require.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestCacheCapabilityAdmissionPreservesCancellation(t *testing.T) {
	t.Parallel()
	// Arrange.
	registry := WithCache(minimalCapabilityRegistry{}, time.Minute)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Act.
	_, listErr := registry.List(ctx)
	_, statErr := registry.Stat(ctx, "known")
	_, captureErr := registry.CaptureManifest(ctx, "known")
	_, describeErr := registry.DescribePrompt(ctx, "known")
	_, resolveErr := registry.ResolveManifest(ctx, "known")
	_, bytesErr := registry.ReadManifestBytes(ctx, "known")
	// Assert.
	for _, err := range []error{listErr, statErr, captureErr, describeErr, resolveErr, bytesErr} {
		require.ErrorIs(t, err, context.Canceled)
	}
}
