// Package compiled holds adapter-internal immutable prompt snapshots (not application state).
package compiled

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/prompty"
)

// DigestSource describes how ManifestDigest was computed.
type DigestSource string

const DigestSourceCapturedSources DigestSource = "captured_sources"

// Prompt is an immutable, serializable snapshot for adapter/otel extensions.
type Prompt struct {
	execution      prompty.PromptExecution
	manifestID     string
	manifestDigest string
	digestSource   DigestSource
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

// FromRenderPlan renders a captured plan and retains its own source identity.
// A registry lookup after planning could describe a different generation and is forbidden.
func FromRenderPlan(ctx context.Context, plan *prompty.RenderPlan) (*Prompt, error) {
	desc, ok := plan.SourceDescriptor()
	if !ok {
		return nil, errors.New("compiled prompt: captured source plan is required")
	}
	exec, err := plan.Execute(ctx)
	if err != nil {
		return nil, err
	}
	return &Prompt{
		execution:      *exec.Clone(),
		manifestID:     desc.ID,
		manifestDigest: desc.Digest,
		digestSource:   DigestSourceCapturedSources,
	}, nil
}
