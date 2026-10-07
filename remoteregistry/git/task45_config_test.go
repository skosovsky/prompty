package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/require"
)

func TestGitSubdirectoryAdmission(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"../outside", "/outside", "a/../b", "a//b", "a/", "a\\b"} {
		// Arrange / Act.
		fetcher, err := NewFetcher("file:///unused", WithDir(dir))
		// Assert: invalid configuration never creates or reads a checkout.
		require.Error(t, err)
		require.Nil(t, fetcher)
	}
	for _, dir := range []string{"", ".", "prompts", "nested/prompts"} {
		fetcher, err := NewFetcher("file:///unused", WithDir(dir))
		require.NoError(t, err)
		require.NoError(t, fetcher.Close())
	}
}

func TestPersistentGitRejectsForeignConfigurationBeforeIO(t *testing.T) {
	t.Parallel()
	for _, mismatch := range []string{"origin", "branch"} {
		t.Run(mismatch, func(t *testing.T) {
			t.Parallel()
			// Arrange: configured credentials must never reach a foreign origin.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer server.Close()
			dir := t.TempDir()
			initRepo(t, dir, map[string]string{"agent.yaml": "known"})
			repo, err := gogit.PlainOpen(dir)
			require.NoError(t, err)
			_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{server.URL}})
			require.NoError(t, err)
			url, branch := server.URL, "main"
			if mismatch == "origin" {
				url = "https://configured.invalid/repo"
			} else {
				branch = "other"
			}
			fetcher, err := NewFetcher(url, WithBranch(branch), WithCloneDir(dir), WithAuth("private"))
			require.NoError(t, err)
			defer fetcher.Close()
			// Act.
			data, err := fetcher.Fetch(context.Background(), "agent")
			// Assert.
			require.ErrorContains(t, err, "persistent checkout")
			require.Nil(t, data)
			require.Zero(t, calls.Load())
			require.Nil(t, fetcher.repo)
		})
	}
}
