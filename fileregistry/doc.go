// Package fileregistry provides a filesystem-based prompt registry that loads
// YAML manifests on demand (lazy) and caches them. Use New to create a Registry;
// GetTemplate resolves name and environment to files like {dir}/{name}.{env}.yaml
// with fallback to {dir}/{name}.yaml.
package fileregistry
