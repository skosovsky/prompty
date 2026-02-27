// Package git provides a Fetcher that reads YAML manifests from a Git repository.
// It clones (or pulls) the repo on first use and reads files from the working tree.
// Use NewFetcher with the repo URL; the returned Fetcher implements remoteregistry.Fetcher for use with remoteregistry.New.
package git
