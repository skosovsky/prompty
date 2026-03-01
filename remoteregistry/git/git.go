package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/skosovsky/prompty/remoteregistry"
)

// Fetcher fetches YAML manifests from a Git repository (clone on first use, then pull).
// Implements remoteregistry.Fetcher. Call Close to remove the local clone.
var _ remoteregistry.Fetcher = (*Fetcher)(nil)

// Fetcher holds repo URL, clone options, and local path.
type Fetcher struct {
	repoURL   string
	branch    string
	dir       string
	depth     int
	authToken string
	localDir  string
	mu        sync.Mutex
	repo      *git.Repository
}

// NewFetcher creates a Fetcher. Repo is cloned on first Fetch. Use Close to cleanup.
// Returns error if repoURL is empty.
func NewFetcher(repoURL string, opts ...Option) (*Fetcher, error) {
	if strings.TrimSpace(repoURL) == "" {
		return nil, fmt.Errorf("remoteregistry/git: repo URL must not be empty")
	}
	g := &Fetcher{
		repoURL: repoURL,
		branch:  "main",
		depth:   1,
	}
	for _, opt := range opts {
		opt(g)
	}
	if strings.TrimSpace(g.branch) == "" {
		return nil, fmt.Errorf("remoteregistry/git: branch must not be empty")
	}
	return g, nil
}

// Fetch reads the manifest from the repo: {dir}/{id}.yaml or {dir}/{id}.yml.
func (g *Fetcher) Fetch(ctx context.Context, id string) ([]byte, error) {
	if err := remoteregistry.ValidateID(id); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.ensureClone(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", remoteregistry.ErrFetchFailed, err)
	}
	baseDir := filepath.Clean(filepath.Join(g.localDir, g.dir))
	for _, rel := range remoteregistry.CandidatePaths(id) {
		path := filepath.Join(g.localDir, g.dir, rel)
		cleanPath := filepath.Clean(path)
		relPath, relErr := filepath.Rel(baseDir, cleanPath)
		if relErr != nil || strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			continue
		}
		data, err := os.ReadFile(cleanPath) // #nosec G304 -- cleanPath is validated via filepath.Rel to prevent path traversal
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("%w: read %s: %w", remoteregistry.ErrFetchFailed, path, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%w: %q", remoteregistry.ErrNotFound, id)
}

func (g *Fetcher) ensureClone(ctx context.Context) error {
	if g.repo != nil {
		// Pull to refresh only for remote URLs (file:// has no remote to pull).
		if !strings.HasPrefix(g.repoURL, "file://") {
			wt, err := g.repo.Worktree()
			if err != nil {
				return fmt.Errorf("worktree: %w", err)
			}
			pullOpts := &git.PullOptions{}
			if g.authToken != "" {
				pullOpts.Auth = &http.BasicAuth{
					Username: "x-access-token",
					Password: g.authToken,
				}
			}
			err = wt.PullContext(ctx, pullOpts)
			if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
				// Do not remove clone on transient errors; next Fetch will retry pull.
				// Proceed to read from existing working tree (stale data).
				slog.Default().Warn("git pull failed, using cached clone", "err", err)
			}
		}
		return nil
	}
	dir, err := os.MkdirTemp("", "prompty-git-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	g.localDir = dir
	cloneOpts := &git.CloneOptions{
		URL:           g.repoURL,
		ReferenceName: plumbing.NewBranchReferenceName(g.branch),
		SingleBranch:  true,
		Progress:      nil,
	}
	if g.depth > 0 {
		cloneOpts.Depth = g.depth
	}
	if g.authToken != "" {
		cloneOpts.Auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: g.authToken,
		}
	}
	repo, err := git.PlainCloneContext(ctx, dir, false, cloneOpts)
	if err != nil {
		_ = os.RemoveAll(dir)
		g.localDir = ""
		return fmt.Errorf("clone: %w", err)
	}
	g.repo = repo
	return nil
}

// Close removes the local clone directory. Safe to call multiple times.
func (g *Fetcher) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.localDir == "" {
		return nil
	}
	dir := g.localDir
	g.localDir = ""
	g.repo = nil
	return os.RemoveAll(dir)
}
