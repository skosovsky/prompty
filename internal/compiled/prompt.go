// Package compiled holds adapter-internal immutable prompt snapshots (not application state).
package compiled

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	yamlparser "github.com/skosovsky/prompty/parser/yaml"
)

// DigestSource describes how ManifestDigest was computed.
type DigestSource string

const (
	DigestSourceManifestBytes     DigestSource = "manifest_bytes"
	DigestSourceCanonicalSnapshot DigestSource = "canonical_snapshot"
)

// Prompt is an immutable, serializable snapshot for adapter/otel extensions.
type Prompt struct {
	execution      prompty.PromptExecution
	manifestID     string
	manifestDigest string
	digestSource   DigestSource
}

// New builds a prompt snapshot from a rendered execution and raw manifest bytes digest.
func New(exec *prompty.PromptExecution, manifestID string, manifestBytes []byte) (*Prompt, error) {
	return newWithManifestDigest(exec, manifestID, manifestBytes, "")
}

func newWithManifestDigest(
	exec *prompty.PromptExecution,
	manifestID string,
	manifestBytes []byte,
	digest string,
) (*Prompt, error) {
	if exec == nil {
		return nil, errors.New("compiled prompt: execution is nil")
	}
	if len(manifestBytes) == 0 {
		return nil, prompty.ErrManifestBytesRequired
	}
	if digest == "" {
		digest = prompty.ManifestDigestSHA256(manifestBytes)
	}
	cloned := exec.Clone()
	return &Prompt{
		execution:      *cloned,
		manifestID:     manifestID,
		digestSource:   DigestSourceManifestBytes,
		manifestDigest: digest,
	}, nil
}

// NewWithCanonicalSnapshot builds when raw manifest bytes are unavailable.
func NewWithCanonicalSnapshot(
	exec *prompty.PromptExecution,
	manifestID string,
	tpl *prompty.ChatPromptTemplate,
) (*Prompt, error) {
	if exec == nil {
		return nil, errors.New("compiled prompt: execution is nil")
	}
	snap, err := canonicalManifestSnapshot(manifestID, tpl)
	if err != nil {
		return nil, err
	}
	cloned := exec.Clone()
	return &Prompt{
		execution:      *cloned,
		manifestID:     manifestID,
		digestSource:   DigestSourceCanonicalSnapshot,
		manifestDigest: prompty.ManifestDigestSHA256(snap),
	}, nil
}

func canonicalManifestSnapshot(manifestID string, tpl *prompty.ChatPromptTemplate) ([]byte, error) {
	if tpl == nil {
		return nil, errors.New("compiled prompt: template source is nil for canonical digest")
	}
	type snap struct {
		ManifestID     string
		Messages       []prompty.MessageTemplate
		Tools          []prompty.ToolDefinition
		RequiredTools  []string
		ModelOptions   *prompty.ModelOptions
		Metadata       prompty.PromptMetadata
		ResponseFormat *prompty.SchemaDefinition
		InputSchema    *prompty.SchemaDefinition
		RequiredVars   []string
	}
	src := prompty.CloneTemplate(tpl)
	return json.Marshal(snap{ //nolint:musttag // digest snapshot uses internal types only
		ManifestID:     manifestID,
		Messages:       append([]prompty.MessageTemplate(nil), src.Messages...),
		Tools:          append([]prompty.ToolDefinition(nil), src.Tools...),
		RequiredTools:  append([]string(nil), src.RequiredTools...),
		ModelOptions:   src.ModelOptions,
		Metadata:       src.Metadata,
		ResponseFormat: src.ResponseFormat,
		InputSchema:    src.InputSchema,
		RequiredVars:   append([]string(nil), src.RequiredVars...),
	})
}

func (c *Prompt) ManifestID() string {
	if c == nil {
		return ""
	}
	return c.manifestID
}

func (c *Prompt) ManifestDigest() string {
	if c == nil {
		return ""
	}
	return c.manifestDigest
}

func (c *Prompt) DigestSource() DigestSource {
	if c == nil {
		return ""
	}
	return c.digestSource
}

func (c *Prompt) PromptExecution() *prompty.PromptExecution {
	if c == nil {
		return nil
	}
	return c.execution.Clone()
}

func (c *Prompt) MarshalJSON() ([]byte, error) {
	if c == nil {
		return nil, errors.New("compiled prompt: nil")
	}
	data, err := marshalJSON(c)
	if err != nil {
		return nil, fmt.Errorf("compiled prompt: %w", err)
	}
	return data, nil
}

func (c *Prompt) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("compiled prompt: nil receiver")
	}
	decoded, err := unmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("compiled prompt: %w", err)
	}
	*c = *decoded
	return nil
}

// FromRenderPlan renders and freezes the plan into an adapter-internal snapshot.
func FromRenderPlan(
	ctx context.Context,
	plan *prompty.RenderPlan,
	manifestID string,
	manifestBytes []byte,
) (*Prompt, error) {
	exec, err := plan.Execute(ctx)
	if err != nil {
		return nil, err
	}
	if manifestID == "" && exec.Metadata.ID != "" {
		manifestID = exec.Metadata.ID
	}
	return New(exec, manifestID, manifestBytes)
}

// FromRenderPlanRegistry renders and compiles using raw manifest bytes from the registry.
// When registry implements ManifestCheckpointRegistry, the snapshot digest matches checkpoint recommend.
func FromRenderPlanRegistry(
	ctx context.Context,
	plan *prompty.RenderPlan,
	registry prompty.ManifestBytesReader,
	id string,
) (*Prompt, error) {
	manifestBytes, err := registry.ReadManifestBytes(ctx, id)
	if err != nil {
		return nil, err
	}
	exec, err := plan.Execute(ctx)
	if err != nil {
		return nil, err
	}
	manifestID := id
	if manifestID == "" && exec.Metadata.ID != "" {
		manifestID = exec.Metadata.ID
	}
	var digest string
	if cp, ok := registry.(prompty.ManifestCheckpointRegistry); ok {
		desc, descErr := cp.RecommendManifestDescriptor(ctx, id)
		if descErr != nil {
			return nil, descErr
		}
		digest = desc.Digest
	} else {
		usesCompose, peekErr := peekComposeManifestBytes(manifestBytes)
		if peekErr != nil {
			return nil, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
		}
		if usesCompose {
			return nil, errors.New("compiled prompt: compose manifests require ManifestCheckpointRegistry")
		}
		digest = prompty.ManifestDigestSHA256(manifestBytes)
	}
	return newWithManifestDigest(exec, manifestID, manifestBytes, digest)
}

func peekComposeManifestBytes(data []byte) (bool, error) {
	uses, err := manifest.PeekComposeOrError(data, manifest.NewJSONParser())
	if err == nil {
		return uses, nil
	}
	return manifest.PeekComposeOrError(data, yamlparser.New())
}

// FromRenderPlanCanonicalSnapshot compiles when the registry cannot supply raw manifest bytes.
func FromRenderPlanCanonicalSnapshot(ctx context.Context, plan *prompty.RenderPlan, id string) (*Prompt, error) {
	exec, err := plan.Execute(ctx)
	if err != nil {
		return nil, err
	}
	manifestID := id
	if manifestID == "" && exec.Metadata.ID != "" {
		manifestID = exec.Metadata.ID
	}
	return NewWithCanonicalSnapshot(exec, manifestID, plan.Template())
}
