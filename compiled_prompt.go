package prompty

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// DigestSource describes how ManifestDigest was computed.
type DigestSource string

const (
	// DigestSourceManifestBytes hashes raw manifest file bytes (preferred).
	DigestSourceManifestBytes DigestSource = "manifest_bytes"
	// DigestSourceCanonicalSnapshot hashes a canonical JSON snapshot when raw bytes are unavailable.
	DigestSourceCanonicalSnapshot DigestSource = "canonical_snapshot"
)

// CompiledPrompt is an immutable, serializable snapshot ready for LLM invocation without re-rendering.
// Use accessor methods; internal fields are not exported to prevent post-compile mutation.
type CompiledPrompt struct {
	execution      PromptExecution
	manifestID     string
	manifestDigest string
	digestSource   DigestSource
}

// NewCompiledPrompt builds an immutable artifact from a rendered execution and raw manifest bytes digest.
func NewCompiledPrompt(
	exec *PromptExecution,
	manifestID string,
	manifestBytes []byte,
) (*CompiledPrompt, error) {
	if exec == nil {
		return nil, errors.New("compiled prompt: execution is nil")
	}
	if len(manifestBytes) == 0 {
		return nil, ErrManifestBytesRequired
	}
	return &CompiledPrompt{
		execution:      *clonePromptExecution(exec),
		manifestID:     manifestID,
		digestSource:   DigestSourceManifestBytes,
		manifestDigest: ManifestDigestSHA256(manifestBytes),
	}, nil
}

// NewCompiledPromptWithCanonicalSnapshot builds a compiled prompt when raw manifest bytes are unavailable.
// Digest is computed from a canonical template snapshot (explicit opt-in; not the strict default).
func NewCompiledPromptWithCanonicalSnapshot(
	exec *PromptExecution,
	manifestID string,
	tpl *ChatPromptTemplate,
) (*CompiledPrompt, error) {
	if exec == nil {
		return nil, errors.New("compiled prompt: execution is nil")
	}
	snap, err := canonicalManifestSnapshot(manifestID, tpl)
	if err != nil {
		return nil, err
	}
	return &CompiledPrompt{
		execution:      *clonePromptExecution(exec),
		manifestID:     manifestID,
		digestSource:   DigestSourceCanonicalSnapshot,
		manifestDigest: ManifestDigestSHA256(snap),
	}, nil
}

func canonicalManifestSnapshot(manifestID string, tpl *ChatPromptTemplate) ([]byte, error) {
	if tpl == nil {
		return nil, errors.New("compiled prompt: template source is nil for canonical digest")
	}
	// Stable JSON of manifest/template contract (not rendered messages with runtime input).
	type snap struct {
		ManifestID     string
		Messages       []MessageTemplate
		Tools          []ToolDefinition
		RequiredTools  []string
		ModelOptions   *ModelOptions
		Metadata       PromptMetadata
		ResponseFormat *SchemaDefinition
		InputSchema    *SchemaDefinition
		RequiredVars   []string
	}
	src := CloneTemplate(tpl)
	return json.Marshal(snap{ //nolint:musttag // digest snapshot uses internal types only
		ManifestID:     manifestID,
		Messages:       append([]MessageTemplate(nil), src.Messages...),
		Tools:          append([]ToolDefinition(nil), src.Tools...),
		RequiredTools:  append([]string(nil), src.RequiredTools...),
		ModelOptions:   src.ModelOptions,
		Metadata:       src.Metadata,
		ResponseFormat: src.ResponseFormat,
		InputSchema:    src.InputSchema,
		RequiredVars:   append([]string(nil), src.RequiredVars...),
	})
}

// ManifestDigestSHA256 returns hex-encoded SHA-256 of raw manifest bytes.
func ManifestDigestSHA256(manifestBytes []byte) string {
	if len(manifestBytes) == 0 {
		return ""
	}
	sum := sha256.Sum256(manifestBytes)
	return hex.EncodeToString(sum[:])
}

// ManifestID returns the manifest identifier bound at compile time.
func (c *CompiledPrompt) ManifestID() string {
	if c == nil {
		return ""
	}
	return c.manifestID
}

// ManifestDigest returns the hex digest of the manifest source used at compile time.
func (c *CompiledPrompt) ManifestDigest() string {
	if c == nil {
		return ""
	}
	return c.manifestDigest
}

// DigestSource describes how ManifestDigest was computed.
func (c *CompiledPrompt) DigestSource() DigestSource {
	if c == nil {
		return ""
	}
	return c.digestSource
}

// PromptExecution returns a deep copy of the compiled execution for adapter invocation.
func (c *CompiledPrompt) PromptExecution() *PromptExecution {
	if c == nil {
		return nil
	}
	return clonePromptExecution(&c.execution)
}

// MarshalJSON serializes the compiled artifact with discriminated content parts.
func (c *CompiledPrompt) MarshalJSON() ([]byte, error) {
	if c == nil {
		return nil, errors.New("compiled prompt: nil")
	}
	data, err := marshalCompiledPromptJSON(c)
	if err != nil {
		return nil, fmt.Errorf("compiled prompt: %w", err)
	}
	return data, nil
}

// UnmarshalJSON deserializes a compiled artifact.
func (c *CompiledPrompt) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("compiled prompt: nil receiver")
	}
	decoded, err := unmarshalCompiledPromptJSON(data)
	if err != nil {
		return fmt.Errorf("compiled prompt: %w", err)
	}
	*c = *decoded
	return nil
}

// Compile renders the plan and freezes the result as a CompiledPrompt.
// manifestBytes should be the raw manifest source when available (registry fetch).
func (p *RenderPlan) Compile(ctx context.Context, manifestID string, manifestBytes []byte) (*CompiledPrompt, error) {
	exec, err := p.Execute(ctx)
	if err != nil {
		return nil, err
	}
	if manifestID == "" && exec.Metadata.ID != "" {
		manifestID = exec.Metadata.ID
	}
	return NewCompiledPrompt(exec, manifestID, manifestBytes)
}

// CompileFromRegistry renders and compiles using raw manifest bytes from the registry (strict).
func (p *RenderPlan) CompileFromRegistry(
	ctx context.Context,
	registry DigestRegistry,
	id string,
) (*CompiledPrompt, error) {
	manifestBytes, err := registry.ReadManifestBytes(ctx, id)
	if err != nil {
		return nil, err
	}
	return p.Compile(ctx, id, manifestBytes)
}

// CompileFromRegistryWithCanonicalSnapshot compiles when the registry cannot supply raw manifest bytes.
func (p *RenderPlan) CompileFromRegistryWithCanonicalSnapshot(ctx context.Context, id string) (*CompiledPrompt, error) {
	exec, err := p.Execute(ctx)
	if err != nil {
		return nil, err
	}
	manifestID := id
	if manifestID == "" && exec.Metadata.ID != "" {
		manifestID = exec.Metadata.ID
	}
	return NewCompiledPromptWithCanonicalSnapshot(exec, manifestID, p.Template())
}
