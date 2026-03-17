package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"

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

	for _, pkg := range cfg.Packages {
		files, err := pkg.ResolveSources(configDir)
		if err != nil {
			return fmt.Errorf("package %q: %w", pkg.Name, err)
		}

		outDir := pkg.Path
		if !filepath.IsAbs(outDir) {
			outDir = filepath.Join(configDir, outDir)
		}
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("package %q mkdir: %w", pkg.Name, err)
		}

		if pkg.IsConsts() {
			if err := runConsts(configDir, files, &pkg, outDir); err != nil {
				return fmt.Errorf("package %q: %w", pkg.Name, err)
			}
		} else {
			if err := runTypes(configDir, files, &pkg, outDir); err != nil {
				return fmt.Errorf("package %q: %w", pkg.Name, err)
			}
		}
	}
	return nil
}

// runConsts generates one _consts_gen.go file with PromptID consts and AllPromptIDs.
// Consts mode has no schema dependency; uses loadManifestID to read only id (no input_schema/response_format).
func runConsts(_ string, files []string, pkg *Package, outDir string) error {
	var ids []string
	seenIDs := make(map[string]string) // id -> first fpath
	for _, fpath := range files {
		id, err := loadManifestID(fpath)
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
	fmt.Printf("Generated %s\n", outPath)
	return nil
}

// runTypes generates shared _shared_gen.go plus per-manifest _gen.go (hybrid types mode).
func runTypes(_ string, files []string, pkg *Package, outDir string) error {
	var specs []*gen.PromptSpec
	var ids []string
	seenIDs := make(map[string]string) // id -> first fpath
	for _, fpath := range files {
		spec, err := loadSpec(fpath)
		if err != nil {
			return fmt.Errorf("manifest %s: %w", fpath, err)
		}
		if spec.ID == "" {
			return fmt.Errorf("manifest %s: id is empty", fpath)
		}
		if err := gen.ValidatePromptID(spec.ID); err != nil {
			return fmt.Errorf("manifest %s: %w", fpath, err)
		}
		if prev, ok := seenIDs[spec.ID]; ok {
			return fmt.Errorf("duplicate id %q in %s and %s", spec.ID, prev, fpath)
		}
		seenIDs[spec.ID] = fpath
		specs = append(specs, spec)
		ids = append(ids, spec.ID)
	}

	// Shared file: PromptID, Prompts, NewPrompts, validate, AllPromptIDs
	sharedFile, err := gen.GenerateSharedTypes(pkg.PackageName, ids)
	if err != nil {
		return fmt.Errorf("generate shared: %w", err)
	}
	sharedPath := filepath.Join(outDir, pkg.PackageName+"_shared_gen.go")
	if err := sharedFile.Save(sharedPath); err != nil {
		return fmt.Errorf("write %s: %w", sharedPath, err)
	}
	fmt.Printf("Generated %s\n", sharedPath)

	// Per-manifest files: const, Input/Output, Render<Name>
	for i, fpath := range files {
		manifestFile, err := gen.GenerateManifestTypes(specs[i], pkg.PackageName)
		if err != nil {
			return fmt.Errorf("generate %s: %w", fpath, err)
		}
		base := strings.TrimSuffix(filepath.Base(fpath), filepath.Ext(fpath))
		base = strings.ReplaceAll(base, "-", "_") // normalize full-mode output filename
		outPath := filepath.Join(outDir, base+"_gen.go")
		if err := manifestFile.Save(outPath); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		fmt.Printf("Generated %s\n", outPath)
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
		for _, fpath := range files {
			spec, err := loadSpec(fpath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", fpath, err)
				continue
			}
			fmt.Printf("%s (id=%s)\n", fpath, spec.ID)
		}
	}
	return nil
}

func loadSpec(fpath string) (*gen.PromptSpec, error) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return nil, err
	}

	var u manifest.Unmarshaler
	switch strings.ToLower(filepath.Ext(fpath)) {
	case ".yaml", ".yml":
		u = yaml.New()
	case ".json":
		u = manifest.NewJSONParser()
	default:
		return nil, fmt.Errorf("unsupported manifest format")
	}

	tpl, err := manifest.Parse(data, u)
	if err != nil {
		return nil, err
	}

	return &gen.PromptSpec{
		ID:             tpl.Metadata.ID,
		InputSchema:    tpl.InputSchema,
		ResponseFormat: tpl.ResponseFormat,
	}, nil
}

// loadManifestID reads only the manifest id field (no schema parsing).
// Used by consts mode to support legacy manifests without input_schema.
func loadManifestID(fpath string) (string, error) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return "", err
	}
	var minimal struct {
		ID string `yaml:"id" json:"id"`
	}
	switch strings.ToLower(filepath.Ext(fpath)) {
	case ".yaml", ".yml":
		if err := yamlv3.Unmarshal(data, &minimal); err != nil {
			return "", fmt.Errorf("parse manifest: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &minimal); err != nil {
			return "", fmt.Errorf("parse manifest: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported manifest format")
	}
	if minimal.ID == "" {
		return "", fmt.Errorf("manifest has no id field")
	}
	return minimal.ID, nil
}
