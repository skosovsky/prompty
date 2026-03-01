package embedregistry

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

// Ensures Registry implements prompty.Registry.
var _ prompty.Registry = (*Registry)(nil)

// Registry loads all YAML manifests from an fs.FS at construction (eager). No mutex. Holds parsed templates by id.
type Registry struct {
	cache           map[string]*prompty.ChatPromptTemplate
	ids             []string // ordered list of ids for List()
	root            string
	partialsPattern string // e.g. "partials/*.tmpl"; relative to root
	version         string // optional build/git version from WithVersion
}

// New walks fsys, parses every .yaml/.yml file under the given root, and returns a Registry.
// id = basename without extension (e.g. "agent", "agent.prod").
func New(fsys fs.FS, root string, opts ...Option) (*Registry, error) {
	r := &Registry{cache: make(map[string]*prompty.ChatPromptTemplate), root: root}
	for _, opt := range opts {
		opt(r)
	}
	seen := make(map[string]bool)
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		var tpl *prompty.ChatPromptTemplate
		if r.partialsPattern != "" {
			partialsPath := filepath.Join(r.root, r.partialsPattern)
			tpl, err = manifest.ParseFSWithOptions(fsys, path, manifest.WithPartialsFS(fsys, partialsPath))
		} else {
			tpl, err = manifest.ParseFS(fsys, path)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		base := filepath.Base(path)
		id := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
		tpl.Metadata.Environment = ""
		r.cache[id] = tpl
		if !seen[id] {
			seen[id] = true
			r.ids = append(r.ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Option configures a Registry.
type Option func(*Registry)

// WithPartials sets a pattern relative to root for partials (e.g. "partials/*.tmpl"); one shared partials dir for all manifests.
func WithPartials(pattern string) Option {
	return func(r *Registry) { r.partialsPattern = pattern }
}

// WithVersion sets a build or git version (e.g. from -ldflags). Stat returns it as TemplateInfo.Version.
func WithVersion(version string) Option {
	return func(r *Registry) { r.version = version }
}

// GetTemplate returns a template by id. O(1) map lookup. Enriches tpl.Metadata.Version from Stat if empty.
func (r *Registry) GetTemplate(ctx context.Context, id string) (*prompty.ChatPromptTemplate, error) {
	if err := prompty.ValidateID(id); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	tpl, ok := r.cache[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
	}
	clone := prompty.CloneTemplate(tpl)
	info, _ := r.Stat(ctx, id)
	if info.Version != "" && clone.Metadata.Version == "" {
		clone.Metadata.Version = info.Version
	}
	return clone, nil
}

// List returns all template ids (order from walk).
func (r *Registry) List(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return append([]string(nil), r.ids...), nil
}

// Stat returns metadata for id without parsing. Version from WithVersion; UpdatedAt is zero for embed.
func (r *Registry) Stat(_ context.Context, id string) (prompty.TemplateInfo, error) {
	if err := prompty.ValidateID(id); err != nil {
		return prompty.TemplateInfo{}, err
	}
	if _, ok := r.cache[id]; !ok {
		return prompty.TemplateInfo{}, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
	}
	return prompty.TemplateInfo{
		ID:        id,
		Version:   r.version,
		UpdatedAt: time.Time{},
	}, nil
}
