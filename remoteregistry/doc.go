// Package remoteregistry provides a remote prompt registry that loads YAML manifests
// via a Fetcher (HTTP or Git). It caches templates with a configurable TTL and supports
// Bearer token authentication. Use New with an implementation of Fetcher (e.g. NewHTTPFetcher);
// GetTemplate returns a cloned template by name and environment.
package remoteregistry
