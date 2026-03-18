package remoteregistry

import (
	"time"

	"github.com/skosovsky/prompty/manifest"
)

// Option configures a Registry (functional options pattern).
type Option func(*Registry)

// WithParser sets the manifest parser (required). Use manifest.NewJSONParser() or parser from github.com/skosovsky/prompty/parser/yaml for YAML.
func WithParser(u manifest.Unmarshaler) Option {
	return func(r *Registry) { r.parser = u }
}

// WithEnvironment sets env for fallback: Fetch tries id.env first, then id.
func WithEnvironment(env string) Option {
	return func(r *Registry) { r.env = env }
}

// WithTTL sets the cache TTL. Templates are refetched after this duration.
// Default is 5 minutes. TTL <= 0 means entries never expire (infinite cache).
// To disable caching, use a very short TTL (e.g. 1 nanosecond).
func WithTTL(d time.Duration) Option {
	return func(r *Registry) {
		r.ttl = d
	}
}
