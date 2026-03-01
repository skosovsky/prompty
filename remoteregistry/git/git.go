package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/skosovsky/prompty"
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
	cloneDir  string // if set, clone/open here and do not remove on Close
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
	absPath, _, err := g.resolvePath(id)
	if err != nil {
		if errors.Is(err, remoteregistry.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", remoteregistry.ErrFetchFailed, err)
	}
	data, err := os.ReadFile(absPath) // #nosec G304 -- absPath from resolvePath (validated path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", remoteregistry.ErrFetchFailed, absPath, err)
	}
	return data, nil
}

// resolvePath finds the physical file for id (tries .yaml then .yml). Returns absolute path and path from repo root (forward slashes).
// Caller must hold g.mu and have called ensureClone. Uses os.Stat only (no ReadFile).
func (g *Fetcher) resolvePath(id string) (absPath, relPathFromRepoRoot string, err error) {
	baseDir := filepath.Clean(filepath.Join(g.localDir, g.dir))
	for _, rel := range remoteregistry.CandidatePaths(id) {
		path := filepath.Join(g.localDir, g.dir, rel)
		cleanPath := filepath.Clean(path)
		relPath, relErr := filepath.Rel(baseDir, cleanPath)
		if relErr != nil || strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			continue
		}
		if _, statErr := os.Stat(cleanPath); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return "", "", statErr
		}
		relPathFromRepoRoot = filepath.ToSlash(filepath.Join(g.dir, relPath))
		return cleanPath, relPathFromRepoRoot, nil
	}
	return "", "", fmt.Errorf("%w: %q", remoteregistry.ErrNotFound, id)
}

// ListIDs returns all template ids (relative path without .yaml/.yml, forward slashes) under the manifest dir.
// Implements remoteregistry.Lister.
var _ remoteregistry.Lister = (*Fetcher)(nil)

func (g *Fetcher) ListIDs(ctx context.Context) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.ensureClone(ctx); err != nil {
		return nil, err
	}
	baseDir := filepath.Clean(filepath.Join(g.localDir, g.dir))
	seen := make(map[string]bool)
	var ids []string
	err := fs.WalkDir(os.DirFS(baseDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		id := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
		if id == base {
			return nil
		}
		rel, relErr := filepath.Rel(".", path)
		if relErr != nil {
			return nil
		}
		id = filepath.ToSlash(rel)
		id = strings.TrimSuffix(strings.TrimSuffix(id, ".yaml"), ".yml")
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list ids: %w", err)
	}
	slices.Sort(ids)
	return ids, nil
}

// Stat returns template metadata without parsing the manifest body. Version is the commit hash; UpdatedAt is commit time.
// Implements remoteregistry.Statter.
var _ remoteregistry.Statter = (*Fetcher)(nil)

func (g *Fetcher) Stat(ctx context.Context, id string) (prompty.TemplateInfo, error) {
	if err := remoteregistry.ValidateID(id); err != nil {
		return prompty.TemplateInfo{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.ensureClone(ctx); err != nil {
		return prompty.TemplateInfo{}, err
	}
	_, relPathFromRepoRoot, err := g.resolvePath(id)
	if err != nil {
		if errors.Is(err, remoteregistry.ErrNotFound) {
			return prompty.TemplateInfo{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
		}
		return prompty.TemplateInfo{}, err
	}
	ref, err := g.repo.Head()
	if err != nil {
		return prompty.TemplateInfo{}, fmt.Errorf("head: %w", err)
	}
	cIter, err := g.repo.Log(&git.LogOptions{
		From: ref.Hash(),
		PathFilter: func(p string) bool {
			return p == relPathFromRepoRoot
		},
	})
	if err != nil {
		return prompty.TemplateInfo{}, fmt.Errorf("log: %w", err)
	}
	commit, err := cIter.Next()
	if err != nil {
		cIter.Close()
		if errors.Is(err, io.EOF) {
			return prompty.TemplateInfo{ID: id, Version: "", UpdatedAt: time.Time{}}, nil
		}
		return prompty.TemplateInfo{}, err
	}
	cIter.Close()
	return prompty.TemplateInfo{
		ID:        id,
		Version:   commit.Hash.String(),
		UpdatedAt: commit.Committer.When,
	}, nil
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
	var repo *git.Repository
	var err error
	if g.cloneDir != "" {
		g.localDir = g.cloneDir
		gitDir := filepath.Join(g.cloneDir, ".git")
		if _, statErr := os.Stat(gitDir); statErr == nil {
			repo, err = git.PlainOpen(g.cloneDir)
		} else {
			if mkErr := os.MkdirAll(g.cloneDir, 0755); mkErr != nil {
				return fmt.Errorf("mkdir clone dir: %w", mkErr)
			}
			cloneOpts := g.buildCloneOptions()
			repo, err = git.PlainCloneContext(ctx, g.cloneDir, false, cloneOpts)
			if err != nil {
				g.localDir = ""
				return fmt.Errorf("clone: %w", err)
			}
		}
	} else {
		dir, mkErr := os.MkdirTemp("", "prompty-git-*")
		if mkErr != nil {
			return fmt.Errorf("temp dir: %w", mkErr)
		}
		g.localDir = dir
		cloneOpts := g.buildCloneOptions()
		repo, err = git.PlainCloneContext(ctx, dir, false, cloneOpts)
		if err != nil {
			_ = os.RemoveAll(dir)
			g.localDir = ""
			return fmt.Errorf("clone: %w", err)
		}
	}
	if err != nil {
		if g.cloneDir == "" {
			g.localDir = ""
		}
		return fmt.Errorf("clone/open: %w", err)
	}
	g.repo = repo
	// Run pull once after open or clone so persistent cache is refreshed.
	if !strings.HasPrefix(g.repoURL, "file://") {
		wt, wtErr := g.repo.Worktree()
		if wtErr != nil {
			return fmt.Errorf("worktree: %w", wtErr)
		}
		pullOpts := &git.PullOptions{}
		if g.authToken != "" {
			pullOpts.Auth = &http.BasicAuth{
				Username: "x-access-token",
				Password: g.authToken,
			}
		}
		if pullErr := wt.PullContext(ctx, pullOpts); pullErr != nil && !errors.Is(pullErr, git.NoErrAlreadyUpToDate) {
			slog.Default().Warn("git pull failed, using cached clone", "err", pullErr)
		}
	}
	return nil
}

func (g *Fetcher) buildCloneOptions() *git.CloneOptions {
	opts := &git.CloneOptions{
		URL:           g.repoURL,
		ReferenceName: plumbing.NewBranchReferenceName(g.branch),
		SingleBranch:  true,
		Progress:      nil,
	}
	if g.depth > 0 {
		opts.Depth = g.depth
	}
	if g.authToken != "" {
		opts.Auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: g.authToken,
		}
	}
	return opts
}

// Close removes the local clone directory (or only releases repo when using WithCloneDir). Safe to call multiple times.
func (g *Fetcher) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.localDir == "" {
		return nil
	}
	g.repo = nil
	dir := g.localDir
	g.localDir = ""
	if g.cloneDir != "" {
		// Persistent clone: do not remove directory.
		return nil
	}
	return os.RemoveAll(dir)
}
