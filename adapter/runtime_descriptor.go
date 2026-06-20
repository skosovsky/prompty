package adapter

import (
	"context"
	"errors"
	"iter"

	"github.com/skosovsky/prompty"
)

// RuntimeDescriptor describes adapter runtime metadata without exposing provider internals.
// Capabilities must be conservative: omit provider/model-dependent features unless
// the runtime can guarantee them for the configured adapter instance.
type RuntimeDescriptor struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities,omitempty"`
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
) iter.Seq2[*prompty.ResponseChunk, error] {
	return c.invoker.ExecuteStream(ctx, exec)
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
	return RuntimeDescriptor{
		Name:         desc.Name,
		Capabilities: append([]string(nil), desc.Capabilities...),
	}, nil
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
