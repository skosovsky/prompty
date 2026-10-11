package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/parser/yaml"

	"github.com/skosovsky/prompty/cmd/prompty-gen/gen"
)

func main() {
	configPath := flag.String("config", "prompty.yaml", "Path to prompty.yaml")
	flag.Parse()

	cmd := "generate"
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
	}

	switch cmd {
	case "generate":
		if err := runGenerate(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "prompty-gen: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := runList(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "prompty-gen: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "prompty-gen: unknown command %q (use generate or list)\n", cmd)
		os.Exit(1)
	}
}

func runGenerate(configPath string) error {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("config path: %w", err)
	}
	configDir := filepath.Dir(absConfig)

	cfg, err := LoadConfig(absConfig)
	if err != nil {
		return err
	}

	return generatePackages(configDir, cfg)
}

// idFromRelativePath computes a fallback PromptID from file path relative to query bases.
// Uses the longest matching base from queries; strips extension, returns slash format (canonical ID).
// Example: base=prompts/, fpath=prompts/workers/image_analyze.yaml -> "workers/image_analyze".
func idFromRelativePath(fpath string, configDir string, queries []string) string {
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
	// No basename fallback: path must resolve relative to queries or callers return error.
	ext := filepath.Ext(bestRel)
	bestRel = strings.TrimSuffix(bestRel, ext)
	return filepath.ToSlash(bestRel)
}

// runConsts generates one _consts_gen.go file with PromptID consts and AllPromptIDs.
// Consts mode has no schema dependency; uses loadManifestID to read only id (no input_schema/response_format).
func runConsts(configDir string, files []string, pkg *Package, outDir string) error {
	var ids []string
	seenIDs := make(map[string]string) // id -> first fpath
	for _, fpath := range files {
		id, err := loadManifestID(fpath, configDir, pkg.Queries)
		if err != nil {
			return fmt.Errorf("manifest %s: %w", fpath, err)
		}
		if err := gen.ValidatePromptID(id); err != nil {
			return fmt.Errorf("manifest %s: %w", fpath, err)
		}
		if prev, ok := seenIDs[id]; ok {
			return fmt.Errorf("duplicate id %q in %s and %s", id, prev, fpath)
		}
		seenIDs[id] = fpath
		ids = append(ids, id)
	}
	outFile, err := gen.GenerateConstsPackage(pkg.PackageName, ids)
	if err != nil {
		return fmt.Errorf("generate consts: %w", err)
	}
	outPath := filepath.Join(outDir, pkg.PackageName+"_consts_gen.go")
	if err := outFile.Save(outPath); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}

// runTypes generates shared _shared_gen.go plus per-manifest _gen.go (hybrid types mode).
type capturedManifestIndex struct {
	loader     manifest.ManifestLoader
	byID       map[string][]byte
	byPath     map[string][]byte
	partials   map[string][]byte
	resolution map[string]string
}

func indexManifests(files []string, configDir string, queries []string) (*capturedManifestIndex, error) {
	index := &capturedManifestIndex{
		loader:     nil,
		byID:       make(map[string][]byte),
		byPath:     make(map[string][]byte),
		partials:   nil,
		resolution: nil,
	}
	owners := make(map[string]string)
	read := func(name string) (*manifest.RawManifest, error) {
		data, ok := index.byPath[name]
		if !ok {
			var err error
			data, err = os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			index.byPath[name] = append([]byte(nil), data...)
		}
		raw, err := decodeRawManifestFile(name, data)
		if err != nil {
			return nil, err
		}
		pathID := manifest.PathIDFromFile(name, configDir, queries)
		id := raw.ID
		if id == "" {
			id = pathID
		}
		for _, alias := range []string{id, pathID} {
			if alias == "" {
				continue
			}
			if previous, exists := owners[alias]; exists && previous != name {
				return nil, fmt.Errorf("manifest alias collision %q in %s and %s", alias, previous, name)
			}
			owners[alias] = name
			index.byID[alias] = data
		}
		return raw, nil
	}
	loader, err := manifest.IndexFileManifests(files, read, manifest.IndexFileOptions{
		IDFromPath: func(name string) string { return manifest.PathIDFromFile(name, configDir, queries) },
	})
	if err != nil {
		return nil, err
	}
	index.loader = loader
	return index, nil
}

// capturePackageSources seals the configured bundle before any generated file is written.
func capturePackageSources(files []string, configDir string, pkg *Package) (*capturedManifestIndex, error) {
	index, err := indexManifests(files, configDir, pkg.Queries)
	if err != nil {
		return nil, err
	}
	bundle := pkg.SourceBundle
	if bundle == nil {
		return index, nil
	}
	if validationErr := bundle.validate(); validationErr != nil {
		return nil, validationErr
	}
	rootPath := bundle.Root
	if !filepath.IsAbs(rootPath) {
		rootPath = filepath.Join(configDir, rootPath)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("source_bundle root: %w", err)
	}
	defer root.Close()
	index.partials = make(map[string][]byte)
	index.resolution = maps.Clone(bundle.Resolution)
	if bundle.Partials == "" {
		return index, nil
	}
	names, err := fs.Glob(root.FS(), bundle.Partials)
	if err != nil {
		return nil, fmt.Errorf("source_bundle glob: %w", err)
	}
	for _, name := range names {
		data, err := root.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("source_bundle partial %q: %w", name, err)
		}
		index.partials[name] = data
	}
	return index, nil
}

func (i *capturedManifestIndex) LoadByID(ctx context.Context, id string) (*manifest.RawManifest, error) {
	return i.loader.LoadByID(ctx, id)
}
func (i *capturedManifestIndex) ReadManifestBytes(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := i.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", prompty.ErrTemplateNotFound, id)
	}
	return append([]byte(nil), data...), nil
}
func (i *capturedManifestIndex) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	return manifest.CaptureSources(ctx, id, i, yaml.New(), i.partials, i.resolution)
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func runTypes(configDir string, files []string, pkg *Package, outDir string) error {
	// Inventory output paths before generating any file, including the shared file.
	owners := map[string]string{
		strings.ToLower(pkg.PackageName + "_shared_gen.go"): "shared package " + pkg.PackageName,
	}
	for _, source := range files {
		name := strings.ToLower(manifestOutputName(source))
		if previous, exists := owners[name]; exists {
			return fmt.Errorf("output collision %q between %s and %s", name, previous, source)
		}
		owners[name] = source
	}
	loader, err := capturePackageSources(files, configDir, pkg)
	if err != nil {
		return err
	}
	var specs []*gen.PromptSpec
	seenIDs := make(map[string]string) // id -> first fpath
	for _, fpath := range files {
		spec, loadErr := loadSpec(fpath, configDir, pkg.Queries, loader)
		if loadErr != nil {
			return fmt.Errorf("manifest %s: %w", fpath, loadErr)
		}
		if spec.ID == "" {
			return fmt.Errorf("manifest %s: id is empty", fpath)
		}
		if validateErr := gen.ValidatePromptID(spec.ID); validateErr != nil {
			return fmt.Errorf("manifest %s: %w", fpath, validateErr)
		}
		if prev, ok := seenIDs[spec.ID]; ok {
			return fmt.Errorf("duplicate id %q in %s and %s", spec.ID, prev, fpath)
		}
		seenIDs[spec.ID] = fpath
		desc, descErr := manifest.CheckpointRecommend(context.Background(), spec.ID, loader)
		if descErr != nil {
			return fmt.Errorf("manifest %s descriptor: %w", fpath, descErr)
		}
		spec.Descriptor = desc
		specs = append(specs, spec)
	}

	// Shared file: PromptID, PromptCatalog API, constructor, validate, AllPromptIDs
	sharedFile, err := gen.GenerateSharedTypes(pkg.PackageName, specs)
	if err != nil {
		return fmt.Errorf("generate shared: %w", err)
	}
	sharedPath := filepath.Join(outDir, pkg.PackageName+"_shared_gen.go")
	if err := sharedFile.Save(sharedPath); err != nil {
		return fmt.Errorf("write %s: %w", sharedPath, err)
	}

	// Per-manifest files: const, Input/Output, prompt type, recipe API, RequiredTools, ID.
	for i, fpath := range files {
		manifestFile, err := gen.GenerateManifestTypes(specs[i], pkg.PackageName)
		if err != nil {
			return fmt.Errorf("generate %s: %w", fpath, err)
		}
		outPath := filepath.Join(outDir, manifestOutputName(fpath))
		if err := manifestFile.Save(outPath); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
	}
	return nil
}

func runList(configPath string) error {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("config path: %w", err)
	}
	configDir := filepath.Dir(absConfig)

	cfg, err := LoadConfig(absConfig)
	if err != nil {
		return err
	}

	for _, pkg := range cfg.Packages {
		files, err := pkg.ResolveSources(configDir)
		if err != nil {
			return fmt.Errorf("package %q: %w", pkg.Name, err)
		}
		loader, loaderErr := capturePackageSources(files, configDir, &pkg)
		if loaderErr != nil {
			fmt.Fprintf(os.Stderr, "  package %s: %v\n", pkg.Name, loaderErr)
			continue
		}
		for _, fpath := range files {
			spec, err := loadSpec(fpath, configDir, pkg.Queries, loader)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", fpath, err)
				continue
			}
			_, _ = fmt.Fprintf(os.Stdout, "%s (id=%s)\n", fpath, spec.ID)
		}
	}
	return nil
}

func readRawManifestFile(fpath string) (*manifest.RawManifest, error) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return nil, err
	}
	return decodeRawManifestFile(fpath, data)
}

func decodeRawManifestFile(fpath string, data []byte) (*manifest.RawManifest, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%s: %w: manifest bytes are empty", fpath, prompty.ErrInvalidManifest)
	}
	var u manifest.Unmarshaler
	switch strings.ToLower(filepath.Ext(fpath)) {
	case extYAML, extYML:
		u = yaml.New()
	case extJSON:
		u = manifest.NewJSONParser()
	default:
		return nil, fmt.Errorf("%s: unsupported manifest format", fpath)
	}
	var raw manifest.RawManifest
	if unmarshalErr := u.Unmarshal(data, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("%s: %w", fpath, wrapRawManifestUnmarshalErr(unmarshalErr))
	}
	return &raw, nil
}

// wrapRawManifestUnmarshalErr aligns prompty-gen with registry LoadByID error taxonomy.
// YAML parser already wraps ErrInvalidManifest; JSON and rare YAML paths may not.
func wrapRawManifestUnmarshalErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, prompty.ErrInvalidManifest) {
		return err
	}
	return fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
}

func loadSpec(
	fpath string,
	configDir string,
	queries []string,
	loader manifest.ManifestLoader,
) (*gen.PromptSpec, error) {
	var raw *manifest.RawManifest
	var err error
	if captured, ok := loader.(*capturedManifestIndex); ok {
		raw, err = decodeRawManifestFile(fpath, captured.byPath[fpath])
	} else {
		raw, err = readRawManifestFile(fpath)
	}
	if err != nil {
		return nil, err
	}
	if raw.ID == "" {
		raw.ID = idFromRelativePath(fpath, configDir, queries)
		if raw.ID == "" {
			return nil, errors.New("manifest has no id field and could not derive id from path")
		}
	}
	if raw.InputSchema == nil {
		return nil, errors.New("manifest missing inputs block")
	}
	if len(raw.Messages) == 0 && len(raw.Layers) == 0 {
		return nil, errors.New("manifest missing messages or layers block")
	}
	composeConditions, err := gen.ComposeConditionsFromRawManifest(context.Background(), raw, loader)
	if err != nil {
		return nil, err
	}
	effectiveSchema, err := manifest.ResolveEffectiveInputSchema(context.Background(), raw, loader)
	if err != nil {
		return nil, err
	}
	raw.InputSchema = effectiveSchema
	if expandErr := manifest.ExpandRawManifest(raw, manifest.ComposeContext{
		Ctx:                         context.Background(),
		Loader:                      loader,
		Values:                      prompty.ComposeValues{},
		AllowMissingConditionValues: true,
	}); expandErr != nil {
		return nil, expandErr
	}
	if len(raw.Messages) == 0 {
		return nil, errors.New("manifest missing messages after composition")
	}

	tpl, err := manifest.BuildFromRaw(raw, nil)
	if err != nil {
		return nil, err
	}

	requiredTools := tpl.RequiredTools
	if requiredTools == nil {
		requiredTools = []string{}
	}
	return &gen.PromptSpec{
		ID:                tpl.Metadata.ID,
		Metadata:          tpl.Metadata,
		Descriptor:        prompty.ManifestDescriptor{ID: tpl.Metadata.ID, Digest: ""},
		RequiredTools:     requiredTools,
		InputSchema:       tpl.InputSchema,
		ResponseFormat:    tpl.ResponseFormat,
		ComposeConditions: composeConditions,
	}, nil
}

// loadManifestID reads the manifest id field and validates the current manifest shape.
// Priority 1: explicit id from YAML/JSON. Priority 2: fallback from relative path.
// Consts flow rejects manifests missing messages or inputs.
func loadManifestID(fpath string, configDir string, queries []string) (string, error) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return "", err
	}
	var manifestShape struct {
		ID       string `json:"id"       yaml:"id"`
		Messages []any  `json:"messages" yaml:"messages"`
		Inputs   any    `json:"inputs"   yaml:"inputs"`
	}
	switch strings.ToLower(filepath.Ext(fpath)) {
	case extYAML, extYML:
		if err := yamlv3.Unmarshal(data, &manifestShape); err != nil {
			return "", fmt.Errorf("parse manifest: %w", err)
		}
	case extJSON:
		if err := json.Unmarshal(data, &manifestShape); err != nil {
			return "", fmt.Errorf("parse manifest: %w", err)
		}
	default:
		return "", errors.New("unsupported manifest format")
	}
	if len(manifestShape.Messages) == 0 {
		return "", errors.New("manifest missing messages block")
	}
	if manifestShape.Inputs == nil {
		return "", errors.New("manifest missing inputs block")
	}
	if manifestShape.ID != "" {
		return manifestShape.ID, nil
	}
	id := idFromRelativePath(fpath, configDir, queries)
	if id == "" {
		return "", errors.New("manifest has no id field and path not under queries (add to queries or set id)")
	}
	return id, nil
}
