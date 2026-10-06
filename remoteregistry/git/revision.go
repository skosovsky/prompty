package git

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/skosovsky/prompty/remoteregistry"
)

var _ remoteregistry.RevisionFetcher = (*Fetcher)(nil)

// BindRevision binds a full commit hash and reads its blobs, independent of checkout.
// Moving branch/tag names and commits unavailable in the clone are rejected.
func (g *Fetcher) BindRevision(ctx context.Context, revision string) (remoteregistry.Fetcher, error) {
	decoded, err := hex.DecodeString(revision)
	if err != nil || len(decoded) != 20 {
		return nil, fmt.Errorf("%w: immutable full commit hash is required", remoteregistry.ErrFetchFailed)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if cloneErr := g.ensureClone(ctx); cloneErr != nil {
		return nil, cloneErr
	}
	commit, err := g.repo.CommitObject(plumbing.NewHash(revision))
	if err != nil {
		return nil, fmt.Errorf("%w: unavailable commit %q: %w", remoteregistry.ErrFetchFailed, revision, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	return &revisionFetcher{tree: tree, dir: g.dir}, nil
}

type revisionFetcher struct {
	tree *object.Tree
	dir  string
}

func (r *revisionFetcher) Fetch(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := remoteregistry.ValidatePathForFetch(id); err != nil {
		return nil, err
	}
	for _, candidate := range remoteregistry.CandidatePaths(id) {
		name := path.Clean(path.Join(r.dir, candidate))
		if name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
			return nil, fmt.Errorf("%w: invalid revision source path", remoteregistry.ErrFetchFailed)
		}
		file, err := r.tree.File(name)
		if err != nil {
			if errors.Is(err, object.ErrFileNotFound) {
				continue
			}
			return nil, err
		}
		if file.Mode != filemode.Regular && file.Mode != filemode.Executable {
			return nil, fmt.Errorf("%w: revision source is not a regular blob", remoteregistry.ErrFetchFailed)
		}
		reader, err := file.Reader()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("%w: %q", remoteregistry.ErrNotFound, id)
}
