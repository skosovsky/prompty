package manifest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skosovsky/prompty"
)

// CaptureSources captures each manifest in the conservative import closure once.
// Partials and settings are supplied by the registry from the same capture operation.
// Multiple mutable reads constitute a captured bundle, not an atomic source revision.
func CaptureSources(ctx context.Context, id string, reader prompty.ManifestBytesReader, parser Unmarshaler,
	partials map[string][]byte, config map[string]string,
) (*prompty.ManifestSnapshot, error) {
	if reader == nil || parser == nil {
		return nil, errors.New("manifest capture: reader and parser are required")
	}
	capture := sourceCapture{
		ctx:     ctx,
		reader:  reader,
		parser:  parser,
		sources: make(map[string][]byte),
		stack:   make(map[string]bool),
		parsed:  make(map[string]*RawManifest),
	}
	if err := capture.visit(id); err != nil {
		return nil, err
	}
	for name, data := range partials {
		capture.sources["partial:"+name] = append([]byte(nil), data...)
	}
	parsed := capture.parsed
	partialSources := partialTemplates(capture.sources)
	return prompty.NewManifestSnapshot(
		id,
		prompty.SourceBundle{Sources: capture.sources, Config: config},
		func(ctx context.Context, bundle prompty.SourceBundle, input prompty.RegistryPlanInput) (*prompty.ChatPromptTemplate, error) {
			return buildParsedTemplate(
				ctx,
				id,
				parsed,
				partialSources,
				bundle.Config,
				input.ComposeValues(),
				false,
			)
		},
	)
}

type sourceCapture struct {
	ctx     context.Context
	reader  prompty.ManifestBytesReader
	parser  Unmarshaler
	sources map[string][]byte
	stack   map[string]bool
	parsed  map[string]*RawManifest
}

func (s *sourceCapture) visit(id string) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.stack[id] {
		return fmt.Errorf("%w: cyclic import %q", prompty.ErrInvalidManifest, id)
	}
	if _, ok := s.sources["manifest:"+id]; ok {
		return nil
	}
	if err := prompty.ValidateID(id); err != nil {
		return err
	}
	data, err := s.reader.ReadManifestBytes(s.ctx, id)
	if err != nil {
		return err
	}
	data = append([]byte(nil), data...)
	if len(data) == 0 {
		return fmt.Errorf("%w: manifest %q bytes are empty", prompty.ErrInvalidManifest, id)
	}
	raw, err := parseCapturedManifest(data, s.parser)
	if err != nil {
		return fmt.Errorf("%w: unmarshal import %q: %w", prompty.ErrInvalidManifest, id, err)
	}
	s.stack[id] = true
	s.sources["manifest:"+id] = data
	s.parsed[id] = raw
	for _, imp := range raw.Imports {
		if err := s.visit(imp.ID); err != nil {
			return fmt.Errorf("read import %q: %w", imp.ID, err)
		}
	}
	delete(s.stack, id)
	return nil
}

func buildCapturedTemplate(ctx context.Context, id string, bundle prompty.SourceBundle, parser Unmarshaler,
	values prompty.ComposeValues, allowMissing bool,
) (*prompty.ChatPromptTemplate, error) {
	byID := make(map[string]*RawManifest)
	partials := make(map[string]string)
	for name, data := range bundle.Sources {
		if partialName, ok := strings.CutPrefix(name, "partial:"); ok {
			partials[partialName] = string(data)
			continue
		}
		sourceID := strings.TrimPrefix(name, "manifest:")
		raw, err := parseCapturedManifest(data, parser)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
		}
		byID[sourceID] = raw
	}
	return buildParsedTemplate(ctx, id, byID, partials, bundle.Config, values, allowMissing)
}

func partialTemplates(sources map[string][]byte) map[string]string {
	partials := make(map[string]string)
	for name, data := range sources {
		if partialName, ok := strings.CutPrefix(name, "partial:"); ok {
			partials[partialName] = string(data)
		}
	}
	return partials
}

func buildParsedTemplate(ctx context.Context, id string, byID map[string]*RawManifest, partials map[string]string,
	config map[string]string, values prompty.ComposeValues, allowMissing bool,
) (*prompty.ChatPromptTemplate, error) {
	raw, err := cloneRawManifest(byID[id])
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("%w: missing captured main manifest", prompty.ErrInvalidManifest)
	}
	compose := ComposeContext{
		Ctx:                         ctx,
		Values:                      values,
		Loader:                      &MemoryLoader{ByID: byID},
		AllowMissingConditionValues: allowMissing,
	}
	options := parseOpts{
		compose:           &compose,
		partialSources:    partials,
		partialsGlob:      "",
		partialsFS:        nil,
		partialsFSPattern: "",
	}
	tpl, err := BuildFromRaw(raw, &options)
	if err != nil {
		return nil, err
	}
	tpl.Metadata.Environment = ""
	if tpl.Metadata.Version == "" {
		tpl.Metadata.Version = config["version"]
	}
	return tpl, nil
}

// ValidateCapturedSources compiles captured bytes conservatively without requiring runtime conditions.
// Used for eager registry construction; actual Plan evaluates conditions strictly.
func ValidateCapturedSources(ctx context.Context, id string, bundle prompty.SourceBundle, parser Unmarshaler) error {
	_, err := buildCapturedTemplate(ctx, id, bundle, parser, prompty.ComposeValues{}, true)
	return err
}

// SourceResolutionConfig describes settings that affect rendering or source binding.
// Default settings have one canonical representation across built-in registries.
func SourceResolutionConfig(environment, partials, partialMode, version, revision string) map[string]string {
	config := make(map[string]string)
	if environment != "" {
		config["environment"] = environment
	}
	if partials != "" {
		config["partials"] = partials
		config["partial_resolution"] = partialMode
	}
	if version != "" {
		config["version"] = version
	}
	if revision != "" {
		config["revision"] = revision
	}
	return config
}

// parseCapturedManifest seals parser output and rejects mutation of its private input.
func parseCapturedManifest(data []byte, parser Unmarshaler) (*RawManifest, error) {
	input := append([]byte(nil), data...)
	var raw RawManifest
	if err := parser.Unmarshal(input, &raw); err != nil {
		return nil, err
	}
	if !bytes.Equal(data, input) {
		return nil, fmt.Errorf("%w: parser mutated source bytes", prompty.ErrInvalidManifest)
	}
	return cloneRawManifest(&raw)
}
