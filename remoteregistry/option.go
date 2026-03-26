package remoteregistry

import (
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
