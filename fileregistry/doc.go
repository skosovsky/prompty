// Package fileregistry provides a filesystem-based prompt registry that loads
// YAML manifests on demand (lazy) and caches them. Use New to create a Registry;
// GetTemplate resolves id to files {dir}/{id}.yaml or {dir}/{id}.yml
// with fallback to {dir}/{name}.yaml.
package fileregistry
