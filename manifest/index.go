package manifest

import (
	"fmt"
	"path/filepath"
	"strings"
)

// IndexFileOptions configures IndexFileManifests path-id fallback.
type IndexFileOptions struct {
	// IDFromPath supplies a cache key when raw.ID is empty. When nil, entries without id are skipped.
	IDFromPath func(fpath string) string
}

// IndexFileManifests builds a MemoryLoader from manifest file paths.
// When raw.ID is empty, IDFromPath must return a non-empty path-derived id (aligned with embedregistry indexing).
func IndexFileManifests(
	files []string,
	read func(fpath string) (*RawManifest, error),
	opts IndexFileOptions,
) (*MemoryLoader, error) {
	byID := make(map[string]*RawManifest, len(files))
	for _, fpath := range files {
		if err := indexOneFile(byID, fpath, read, opts); err != nil {
			return nil, err
		}
	}
	return &MemoryLoader{ByID: byID}, nil
}

func indexOneFile(
	byID map[string]*RawManifest,
	fpath string,
	read func(fpath string) (*RawManifest, error),
	opts IndexFileOptions,
) error {
	raw, err := read(fpath)
	if err != nil {
		return fmt.Errorf("%s: %w", fpath, err)
	}
	cacheID := raw.ID
	if cacheID == "" {
		if opts.IDFromPath == nil {
			return nil
		}
		cacheID = opts.IDFromPath(fpath)
		if cacheID == "" {
			return nil
		}
	}
	if prev, ok := byID[cacheID]; ok && prev != nil {
		return fmt.Errorf("duplicate manifest id %q (%s)", cacheID, fpath)
	}
	cloned := *raw
	if cloned.ID == "" {
		cloned.ID = cacheID
	}
	byID[cacheID] = &cloned
	if raw.ID != "" && raw.ID != cacheID {
		if _, ok := byID[raw.ID]; !ok {
			alias := cloned
			byID[raw.ID] = &alias
		}
	}
	return nil
}

// PathIDFromFile derives a slash-separated manifest id from a file path relative to query bases.
func PathIDFromFile(fpath string, configDir string, queries []string) string {
	fpath = filepath.Clean(fpath)
	configDir = filepath.Clean(configDir)
	var bestRel string
	var bestBaseLen int
	for _, q := range queries {
		base := filepath.Join(configDir, q)
		base = filepath.Clean(base)
		if strings.Contains(q, "*") {
			base = filepath.Dir(base)
		}
		rel, err := filepath.Rel(base, fpath)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(base) > bestBaseLen {
			bestBaseLen = len(base)
			bestRel = rel
		}
	}
	ext := filepath.Ext(bestRel)
	bestRel = strings.TrimSuffix(bestRel, ext)
	return filepath.ToSlash(bestRel)
}
