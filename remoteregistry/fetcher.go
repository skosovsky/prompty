package remoteregistry

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/skosovsky/prompty"
)

// Fetcher fetches raw YAML manifest bytes by template id.
// Registry uses it to obtain manifest content; HTTP and Git are typical implementations.
//
// Return ErrNotFound when the template does not exist; Registry translates it to prompty.ErrTemplateNotFound.
// Wrap other errors in ErrFetchFailed so callers can use errors.Is.
type Fetcher interface {
	Fetch(ctx context.Context, id string) ([]byte, error)
}

// Lister is optional. When implemented by Fetcher, Registry.List uses it to return available ids.
type Lister interface {
	ListIDs(ctx context.Context) ([]string, error)
}

// Statter is optional. When implemented by Fetcher, Registry.Stat uses it to return metadata without parsing body.
type Statter interface {
	Stat(ctx context.Context, id string) (prompty.TemplateInfo, error)
}

// ValidateID checks that id is a valid user-facing prompt id (slash-only, no extension).
// Delegates to prompty.ValidateID. Use for user input validation.
func ValidateID(id string) error {
	return prompty.ValidateID(id)
}

// ValidatePathForFetch validates path for internal fetcher use. Allows env suffix (id.env).
// Used when registry passes candidate ids like "internal/router.prod" for env fallback.
func ValidatePathForFetch(id string) error {
	if id == "" {
		return fmt.Errorf("%w: id must not be empty", prompty.ErrInvalidName)
	}
	if !fs.ValidPath(id) {
		return fmt.Errorf("%w: invalid path %q", prompty.ErrInvalidName, id)
	}
	return nil
}

// CandidatePaths returns manifest filename candidates in resolution order: id.yaml, id.yml, id.json.
func CandidatePaths(id string) []string {
	return []string{id + ".yaml", id + ".yml", id + ".json"}
}
