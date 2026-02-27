package remoteregistry

import "time"

// Option configures a Registry (functional options pattern).
type Option func(*Registry)

// WithTTL sets the cache TTL. Templates are refetched after this duration.
// Default is 5 minutes. TTL <= 0 means entries never expire (infinite cache).
// To disable caching, use a very short TTL (e.g. 1 nanosecond).
func WithTTL(d time.Duration) Option {
	return func(r *Registry) {
		r.ttl = d
	}
}
