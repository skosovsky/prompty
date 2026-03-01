// Package embedregistry provides an embed.FS-based prompt registry that loads
// all YAML manifests at construction (eager). Use New with an fs.FS and root path;
// GetTemplate performs an O(1) lookup by id.
// Template name must not contain ':' (used as cache key separator).
package embedregistry
