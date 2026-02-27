package remoteregistry

import "errors"

// Sentinel errors for remote registry operations.
// Callers should use errors.Is to check.
var (
	// ErrFetchFailed indicates the Fetcher could not retrieve the manifest.
	ErrFetchFailed = errors.New("remoteregistry: fetch failed")
	// ErrHTTPStatus indicates an unexpected HTTP status (e.g. 500) when using HTTPFetcher.
	ErrHTTPStatus = errors.New("remoteregistry: unexpected HTTP status")
	// ErrNotFound indicates no manifest was found for the given name/env; registry wraps it in prompty.ErrTemplateNotFound.
	ErrNotFound = errors.New("remoteregistry: no manifest found")
)
