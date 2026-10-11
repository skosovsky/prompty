package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/require"
)

func TestCanceledSourceOperations(t *testing.T) {
	t.Parallel()
	for _, persistent := range []bool{false, true} {
		t.Run(map[bool]string{false: "uninitialized", true: "persistent"}[persistent], func(t *testing.T) {
			t.Parallel()
			// Arrange: no cancellation fixture may contact a remote.
			dir := t.TempDir()
			initRepo(t, dir, map[string]string{"agent.yaml": "trusted"})
			fetcher, err := NewFetcher("file://"+dir, WithCloneDir(dir))
			require.NoError(t, err)
			defer fetcher.Close()
			if persistent {
				data, e := fetcher.Fetch(t.Context(), "agent")
				require.NoError(t, e)
				require.Equal(t, "trusted", string(data))
			}
			for _, test := range []struct {
				ctx  func(context.Context) context.Context
				want error
			}{
				{func(parent context.Context) context.Context {
					c, cancel := context.WithCancel(parent)
					cancel()
					return c
				}, context.Canceled},
				{func(parent context.Context) context.Context {
					c, cancel := context.WithDeadline(parent, time.Unix(0, 0))
					cancel()
					return c
				}, context.DeadlineExceeded},
			} {
				ctx := test.ctx(t.Context())
				// Act / Assert: bytes, metadata and bound handles remain absent.
				data, err := fetcher.Fetch(ctx, "agent")
				require.ErrorIs(t, err, test.want)
				require.Nil(t, data)
				ids, err := fetcher.ListIDs(ctx)
				require.ErrorIs(t, err, test.want)
				require.Nil(t, ids)
				info, err := fetcher.Stat(ctx, "agent")
				require.ErrorIs(t, err, test.want)
				require.Empty(t, info.ID)
				bound, err := fetcher.BindRevision(ctx, "0000000000000000000000000000000000000000")
				require.ErrorIs(t, err, test.want)
				require.Nil(t, bound)
			}
		})
	}
}

type admittedContext struct {
	context.Context

	once     sync.Once
	admitted chan struct{}
}

func (c *admittedContext) Err() error {
	c.once.Do(func() { close(c.admitted) })
	return c.Context.Err()
}
func TestCanceledWhileWaitingForSourceLock(t *testing.T) {
	t.Parallel()
	// Arrange.
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"agent.yaml": "trusted"})
	fetcher, err := NewFetcher("file://"+dir, WithCloneDir(dir))
	require.NoError(t, err)
	defer fetcher.Close()
	fetcher.mu.Lock()
	ctx, cancel := context.WithCancel(t.Context())
	admitted := make(chan struct{})
	callCtx := &admittedContext{Context: ctx, admitted: admitted}
	done := make(chan error, 1)
	go func() {
		data, e := fetcher.Fetch(callCtx, "agent")
		if data != nil {
			done <- context.DeadlineExceeded
			return
		}
		done <- e
	}()
	<-admitted
	// Act.
	cancel()
	fetcher.mu.Unlock()
	// Assert.
	require.ErrorIs(t, <-done, context.Canceled)
	require.Nil(t, fetcher.repo)
}

func TestRefreshFailureNeverReturnsStaleBytes(t *testing.T) {
	t.Parallel()
	// Arrange: a persistent working tree exists but its configured origin is absent.
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"agent.yaml": "stale"})
	fetcher, err := NewFetcher("https://fixture.invalid/repo", WithCloneDir(dir))
	require.NoError(t, err)
	defer fetcher.Close()
	// Act.
	data, err := fetcher.Fetch(t.Context(), "agent")
	// Assert.
	require.Error(t, err)
	require.Nil(t, data)
}

func TestCancellationDuringPull(t *testing.T) {
	t.Parallel()
	// Arrange: the only remote is a local HTTP server, gated until its request is canceled.
	started := make(chan struct{}, 1)
	server := httptest.NewServer(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() }),
	)
	defer server.Close()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"agent.yaml": "stale"})
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{server.URL}})
	require.NoError(t, err)
	fetcher, err := NewFetcher(server.URL, WithCloneDir(dir))
	require.NoError(t, err)
	defer fetcher.Close()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		data, e := fetcher.Fetch(ctx, "agent")
		if len(data) > 0 {
			done <- os.ErrInvalid
			return
		}
		done <- e
	}()
	<-started
	// Act.
	cancel()
	// Assert.
	require.ErrorIs(t, <-done, context.Canceled)
	data, err := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	require.NoError(t, err)
	require.Equal(t, "stale", string(data))
}

func TestBoundRevisionCancellation(t *testing.T) {
	t.Parallel()
	// Arrange: a valid bound immutable tree also honors operation cancellation.
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"agent.yaml": "trusted"})
	fetcher, err := NewFetcher("file://"+dir, WithCloneDir(dir))
	require.NoError(t, err)
	defer fetcher.Close()
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	bound, err := fetcher.BindRevision(t.Context(), head.Hash().String())
	require.NoError(t, err)
	valid, err := bound.Fetch(t.Context(), "agent")
	require.NoError(t, err)
	require.Equal(t, "trusted", string(valid))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Act.
	data, err := bound.Fetch(ctx, "agent")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, data)
}

func TestCancellationDuringListTraversal(t *testing.T) {
	t.Parallel()
	// Arrange: cancel exactly on the first manifest visit after admission/root walk.
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"agent.yaml": "trusted"})
	fetcher, err := NewFetcher("file://"+dir, WithCloneDir(dir))
	require.NoError(t, err)
	defer fetcher.Close()
	_, err = fetcher.Fetch(t.Context(), "agent")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	controlled := &walkCancellationContext{Context: ctx, cancel: cancel}
	// Act.
	ids, err := fetcher.ListIDs(controlled)
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
	if c.calls == 4 {
		c.cancel()
	}
	return c.Context.Err()
}
