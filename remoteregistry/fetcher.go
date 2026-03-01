package remoteregistry

import (
	"context"

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

// ValidateID checks that id is safe for use in paths and cache keys.
// Delegates to prompty.ValidateID so all registries share the same rules.
func ValidateID(id string) error {
	return prompty.ValidateID(id)
}

// CandidatePaths returns manifest filename candidates in resolution order: id.yaml, id.yml.
// Call ValidateID(id) before using the result with filesystem paths.
func CandidatePaths(id string) []string {
	return []string{id + ".yaml", id + ".yml"}
}
