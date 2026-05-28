// Package embedregistry provides an [embed.FS]-based prompt registry that loads
// all YAML manifests at construction (eager). Use New with an [fs.FS] and root path;
// Plan returns a deferred [prompty.RenderPlan] for O(1) lookup by id.
// Template name must not contain ':' (used as cache key separator).
package embedregistry
