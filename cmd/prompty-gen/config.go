package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the prompty-gen configuration (prompty.yaml).
type Config struct {
	Version  string    `yaml:"version"`
	Packages []Package `yaml:"packages"`
}

// Package defines one output package with its manifest sources and options.
// Strict break: max_retries/enable_validation removed; unknown fields fail via KnownFields.
type Package struct {
	Name        string   `yaml:"name"`     // e.g. "prompts"
	Path        string   `yaml:"path"`     // output directory for generated code
	Queries     []string `yaml:"queries"`  // paths or globs for manifest files
	PackageName string   `yaml:"package"`  // Go package name (default: name)
	Mode        string   `yaml:"mode"`     // "consts" | "types" (default: "types")
}

// IsConsts returns true when mode is "consts".
func (p *Package) IsConsts() bool {
	return strings.ToLower(p.Mode) == "consts"
}

// IsTypes returns true when mode is "types" or empty (default).
func (p *Package) IsTypes() bool {
	m := strings.ToLower(p.Mode)
	return m == "" || m == "types"
}

// LoadConfig reads and parses prompty.yaml from path.
// Uses KnownFields(true) so legacy fields (max_retries, enable_validation) cause hard-fail.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Version == "" {
		c.Version = "1"
	}
	for i := range c.Packages {
		p := &c.Packages[i]
		if p.PackageName == "" {
			p.PackageName = p.Name
		}
		if p.Mode == "" {
			p.Mode = "types"
		}
		m := strings.ToLower(p.Mode)
		if m != "consts" && m != "types" {
			return nil, fmt.Errorf("package %q: invalid mode %q (use consts or types)", p.Name, p.Mode)
		}
	}
	return &c, nil
}

// ResolveSources expands globs (queries) into manifest file paths relative to configDir.
// If a path is a directory (ends with / or exists as dir), it recursively finds all *.yaml, *.yml, *.json (case-insensitive).
func (p *Package) ResolveSources(configDir string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	for _, glob := range p.Queries {
		base := filepath.Join(configDir, glob)
		info, err := os.Stat(base)
		isDir := err == nil && info != nil && info.IsDir()
		trimmed := strings.TrimSuffix(glob, "/")
		isDirPath := trimmed != glob || isDir
		if isDirPath || isDir {
			err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if ext != ".yaml" && ext != ".yml" && ext != ".json" {
					return nil
				}
				if !seen[path] {
					seen[path] = true
					out = append(out, path)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("walk %q: %w", base, err)
			}
			continue
		}
		matches, err := filepath.Glob(base)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", base, err)
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(m))
			if ext != ".yaml" && ext != ".yml" && ext != ".json" {
				continue
			}
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out, nil
}
