package fileregistry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/manifest"
)

func TestStatAndListCancellation(t *testing.T) {
	t.Parallel()
	// Arrange.
	dir := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "agent.json"),
			[]byte(`{"id":"agent","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"ok"}]}]}`),
			0600,
		),
	)
	registry, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	info, err := registry.Stat(t.Context(), "agent")
	require.NoError(t, err)
	require.Equal(t, "agent", info.ID)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Act / Assert.
	info, err = registry.Stat(ctx, "agent")
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, info.ID)
	ids, err := registry.List(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, ids)
}

func TestCancellationDuringListWalk(t *testing.T) {
	t.Parallel()
	// Arrange: cancellation occurs on the first entry after the root directory.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{}`), 0600))
	registry, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	valid, err := registry.List(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"agent"}, valid)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	controlled := &walkCancellationContext{Context: ctx, cancel: cancel}
	// Act.
	ids, err := registry.List(controlled)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, ids)
}

type walkCancellationContext struct {
	context.Context

	cancel context.CancelFunc
	calls  int
}

func (c *walkCancellationContext) Err() error {
	c.calls++
	if c.calls == 3 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestMetadataDeadline(t *testing.T) {
	t.Parallel()
	// Arrange: an already-expired deadline requires no scheduling sleeps.
	dir := t.TempDir()
	registry, err := New(dir, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()
	// Act / Assert.
	info, err := registry.Stat(ctx, "agent")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, info.ID)
	ids, err := registry.List(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, ids)
}

func TestCancellationDuringMetadataLookup(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"agent", "missing"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			// Arrange: the admission read succeeds, then the context becomes canceled.
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{}`), 0600))
			registry, err := New(dir, WithParser(manifest.NewJSONParser()))
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			controlled := &admissionCancellationContext{Context: ctx, cancel: cancel}
			// Act.
			info, err := registry.Stat(controlled, id)
			// Assert: both success and missing paths retain cancellation, without metadata.
			require.ErrorIs(t, err, context.Canceled)
			require.Empty(t, info.ID)
		})
	}
}

type admissionCancellationContext struct {
	context.Context

	cancel context.CancelFunc
}

func (c *admissionCancellationContext) Err() error { err := c.Context.Err(); c.cancel(); return err }
