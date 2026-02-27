package remoteregistry

import (
	"context"

	"github.com/skosovsky/prompty"
)

// Fetcher fetches raw YAML manifest bytes by template name and environment.
// Registry uses it to obtain manifest content; HTTP and Git are typical implementations.
//
// Return ErrNotFound when the template does not exist; Registry translates it to prompty.ErrTemplateNotFound.
// Wrap other errors in ErrFetchFailed so callers can use errors.Is.
type Fetcher interface {
	Fetch(ctx context.Context, name, env string) ([]byte, error)
}

// ValidateName checks that name and env are safe for use in paths and cache keys.
// Delegates to prompty.ValidateName so all registries share the same rules.
func ValidateName(name, env string) error {
	return prompty.ValidateName(name, env)
}

// CandidatePaths returns manifest filename candidates in resolution order:
// {name}.{env}.yaml, {name}.{env}.yml (if env != ""), then {name}.yaml, {name}.yml.
// Call ValidateName(name, env) before using the result with filesystem paths.
func CandidatePaths(name, env string) []string {
	var out []string
	if env != "" {
		out = append(out, name+"."+env+".yaml", name+"."+env+".yml")
	}
	out = append(out, name+".yaml", name+".yml")
	return out
}
