package adapter

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/skosovsky/prompty"
)

// RuntimeDescriptor describes adapter runtime metadata without exposing provider internals.
// Profiles must be conservative and bound to the effective endpoint/model.
// SDK mappings do not establish provider-wide feature support.
type RuntimeDescriptor struct {
	Name              string                      `json:"name"`
	Endpoint          string                      `json:"endpoint"`
	DefaultModel      string                      `json:"default_model"`
	Profiles          []prompty.CapabilityProfile `json:"profiles,omitempty"`
	ReasoningSettings []string                    `json:"reasoning_settings,omitempty"`
	// HostedToolCapabilities binds implemented adapter kinds to profile guarantees.
	HostedToolCapabilities map[string]string `json:"hosted_tool_capabilities,omitempty"`
	// MessageAnnotationCapabilities gates provider history features on the same
	// profile and operation combinations as requested execution features.
	MessageAnnotationCapabilities map[string][]string `json:"message_annotation_capabilities,omitempty"`
}

// RuntimeDescriber is implemented by adapters that expose runtime metadata.
type RuntimeDescriber interface {
	RuntimeDescriptor() RuntimeDescriptor
}

// TokenEstimatorProvider exposes the estimator attached to a runtime wrapper.
type TokenEstimatorProvider interface {
	TokenEstimator() (TokenEstimator, error)
}

// RuntimeDescriptorProvider exposes runtime metadata from a runtime wrapper.
type RuntimeDescriptorProvider interface {
	RuntimeDescriptor() (RuntimeDescriptor, error)
}

// RuntimeClient is an invoker with descriptor and token estimator access.
type RuntimeClient interface {
	prompty.Invoker
	RuntimeDescriptorProvider
	TokenEstimatorProvider
	EstimateTokens(exec *prompty.PromptExecution) (int, error)
}

var ErrNoRuntimeDescriptor = errors.New("adapter: RuntimeDescriptor not implemented")

type runtimeClient[Req any, Resp any] struct {
	invoker prompty.Invoker
	adapter ProviderAdapter[Req, Resp]
}

// NewRuntimeClient creates an invoker wrapper that exposes runtime descriptor and token estimator.
func NewRuntimeClient[Req any, Resp any](
	adp ProviderAdapter[Req, Resp],
	mws ...prompty.Middleware,
) RuntimeClient {
	return &runtimeClient[Req, Resp]{
		invoker: NewClient(adp, mws...),
		adapter: adp,
	}
}

// Execute delegates to the wrapped invoker.
func (c *runtimeClient[Req, Resp]) Execute(
	ctx context.Context,
	exec *prompty.PromptExecution,
) (*prompty.Response, error) {
	return c.invoker.Execute(ctx, exec)
}

// ExecuteStream delegates to the wrapped invoker.
func (c *runtimeClient[Req, Resp]) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	return c.invoker.ExecuteStream(ctx, exec, mode)
}

// RuntimeDescriptor returns adapter runtime metadata or an explicit error.
func (c *runtimeClient[Req, Resp]) RuntimeDescriptor() (RuntimeDescriptor, error) {
	if c == nil || c.adapter == nil {
		return RuntimeDescriptor{}, ErrNoRuntimeDescriptor
	}
	describer, ok := any(c.adapter).(RuntimeDescriber)
	if !ok {
		return RuntimeDescriptor{}, ErrNoRuntimeDescriptor
	}
	desc := describer.RuntimeDescriptor()
	if desc.Name == "" {
		return RuntimeDescriptor{}, ErrNoRuntimeDescriptor
	}
	if err := prompty.ValidateEndpointIdentity(desc.Endpoint); err != nil {
		return RuntimeDescriptor{}, errors.Join(ErrInvalidProfile, err)
	}
	for _, profile := range desc.Profiles {
		if err := prompty.ValidateEndpointIdentity(profile.Identity.Endpoint); err != nil {
			return RuntimeDescriptor{}, errors.Join(ErrInvalidProfile, err)
		}
	}
	return CloneRuntimeDescriptor(desc), nil
}

// CloneRuntimeDescriptor isolates every nested profile.
func CloneRuntimeDescriptor(desc RuntimeDescriptor) RuntimeDescriptor {
	out := desc
	out.ReasoningSettings = slices.Clone(desc.ReasoningSettings)
	out.HostedToolCapabilities = maps.Clone(desc.HostedToolCapabilities)
	out.MessageAnnotationCapabilities = maps.Clone(desc.MessageAnnotationCapabilities)
	for kind, capabilities := range out.MessageAnnotationCapabilities {
		out.MessageAnnotationCapabilities[kind] = slices.Clone(capabilities)
	}
	out.Profiles = make([]prompty.CapabilityProfile, len(desc.Profiles))
	for i, profile := range desc.Profiles {
		out.Profiles[i] = prompty.CloneCapabilityProfile(profile)
	}
	return out
}

// TokenEstimator returns adapter token estimator or an explicit error.
func (c *runtimeClient[Req, Resp]) TokenEstimator() (TokenEstimator, error) {
	if c == nil || c.adapter == nil {
		return nil, ErrNoTokenEstimator
	}
	estimator, ok := any(c.adapter).(TokenEstimator)
	if !ok {
		return nil, ErrNoTokenEstimator
	}
	return estimator, nil
}

// EstimateTokens estimates tokens through the runtime wrapper estimator.
func (c *runtimeClient[Req, Resp]) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	estimator, err := c.TokenEstimator()
	if err != nil {
		return 0, err
	}
	return EstimateTokens(estimator, exec)
}
