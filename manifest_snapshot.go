package prompty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
)

const sourceDigestPrefix = "source-bundle.v1:sha256:"

// SourceBundle contains captured sources and declarative resolution settings.
// Sources are opaque named bytes; the host owns their interpretation and trust.
type SourceBundle struct {
	Sources map[string][]byte `json:"sources"`
	Config  map[string]string `json:"config"`
}

// SnapshotMaterializer must build exclusively from the supplied captured sources.
// It must not read mutable source storage again. Runtime Go functions remain host-owned.
type SnapshotMaterializer func(context.Context, SourceBundle, RegistryPlanInput) (*ChatPromptTemplate, error)

// ManifestSnapshot is an immutable source bundle with its materializer.
// It is not a durable agent checkpoint or a serialized Go implementation.
type ManifestSnapshot struct {
	descriptor  ManifestDescriptor
	bundle      SourceBundle
	materialize SnapshotMaterializer
}

// ManifestSnapshotRegistry captures one source generation for verification and planning.
// A plain Registry does not imply this stronger contract.
type ManifestSnapshotRegistry interface {
	CaptureManifest(context.Context, string) (*ManifestSnapshot, error)
}

// NewManifestSnapshot seals caller-owned bytes and computes a versioned identity.
// Custom materializers are trusted implementations of the captured-source contract.
func NewManifestSnapshot(id string, bundle SourceBundle, materialize SnapshotMaterializer) (*ManifestSnapshot, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	if len(bundle.Sources) == 0 || materialize == nil {
		return nil, errors.New("manifest snapshot: sources and materializer are required")
	}
	for name := range bundle.Sources {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("manifest snapshot: source names must be nonempty")
		}
	}
	bundle = cloneSourceBundle(bundle)
	identity, err := json.Marshal(struct {
		ID     string       `json:"id"`
		Bundle SourceBundle `json:"bundle"`
	}{ID: id, Bundle: bundle})
	if err != nil {
		return nil, err
	}
	return &ManifestSnapshot{
		descriptor: ManifestDescriptor{ID: id, Digest: sourceDigestPrefix + ManifestDigestSHA256(identity)},
		bundle:     bundle, materialize: materialize,
	}, nil
}

func cloneSourceBundle(in SourceBundle) SourceBundle {
	out := SourceBundle{Sources: make(map[string][]byte, len(in.Sources)), Config: maps.Clone(in.Config)}
	if out.Config == nil {
		out.Config = make(map[string]string)
	}
	for name, data := range in.Sources {
		out.Sources[name] = append([]byte(nil), data...)
	}
	return out
}

// Descriptor returns the identity of exactly the sources used by Plan.
func (s *ManifestSnapshot) Descriptor() ManifestDescriptor {
	if s == nil {
		return ManifestDescriptor{}
	}
	return s.descriptor
}

// Sources returns an isolated copy for host persistence or inspection.
func (s *ManifestSnapshot) Sources() SourceBundle {
	if s == nil {
		return SourceBundle{}
	}
	return cloneSourceBundle(s.bundle)
}

// Plan builds from captured bytes, even when the original source has disappeared.
func (s *ManifestSnapshot) Plan(ctx context.Context, input RegistryPlanInput) (*RenderPlan, error) {
	if s == nil || s.materialize == nil {
		return nil, errors.New("manifest snapshot: nil snapshot")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	working := cloneSourceBundle(s.bundle)
	tpl, err := s.materialize(ctx, working, input)
	if err != nil {
		return nil, err
	}
	if !maps.Equal(working.Config, s.bundle.Config) || !maps.EqualFunc(working.Sources, s.bundle.Sources, bytes.Equal) {
		return nil, fmt.Errorf("%w: materializer changed captured source contract", ErrManifestDigestMismatch)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	plan, err := NewRenderPlanFromPlanInput(tpl, input)
	if err != nil {
		return nil, err
	}
	plan.sourceDescriptor = s.descriptor
	return plan, nil
}

// VerifiedManifestPlan captures, verifies, and plans one immutable generation.
func VerifiedManifestPlan(
	ctx context.Context,
	registry ManifestSnapshotRegistry,
	desc ManifestDescriptor,
	input RegistryPlanInput,
) (*RenderPlan, error) {
	if registry == nil {
		return nil, errors.New("manifest snapshot: registry is required")
	}
	if err := ValidateManifestDescriptor(desc); err != nil {
		return nil, err
	}
	snapshot, err := registry.CaptureManifest(ctx, desc.ID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.Descriptor() != desc {
		return nil, fmt.Errorf("%w: %q", ErrManifestDigestMismatch, desc.ID)
	}
	return snapshot.Plan(ctx, input)
}

// SourceDescriptor reports the captured source identity, or false for an unverified plan.
func (p *RenderPlan) SourceDescriptor() (ManifestDescriptor, bool) {
	if p == nil || p.sourceDescriptor.Digest == "" {
		return ManifestDescriptor{}, false
	}
	return p.sourceDescriptor, true
}
