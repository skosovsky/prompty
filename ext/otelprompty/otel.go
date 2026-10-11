// Package otelprompty records logical prompty operations with caller-owned OTel
// tracing. Default telemetry excludes content, identifiers, URLs and error text.
package otelprompty

import (
	"context"

	"github.com/skosovsky/prompty"
)

// WithTracing observes one logical invocation, not physical SDK attempts.
func WithTracing(opts ...Option) prompty.Middleware {
	cfg := resolveConfig(opts)
	return func(next prompty.Invoker) prompty.Invoker { return &tracingInvoker{next: next, cfg: cfg} }
}

type tracingInvoker struct {
	next prompty.Invoker
	cfg  *config
}

func (t *tracingInvoker) Execute(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	obs := invocationObservation(exec)
	spanCtx, span := begin(ctx, t.cfg, obs)
	defer span.finishOnReturn(&obs)
	if exec != nil {
		if err := prompty.ValidateExecutionToolScope(exec); err != nil {
			obs.Err = err
			obs.ErrorCategory = "preflight_rejection"
			return nil, err
		}
	}
	resp, err := t.next.Execute(spanCtx, exec)
	obs.Response, obs.Err = resp, err
	return resp, err
}

func (t *tracingInvoker) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	obs := invocationObservation(exec)
	stream := t.scopedStream(ctx, exec, mode)
	if stream == nil {
		stream = prompty.ErrorStream(ctx, mode, prompty.ErrStreamProtocol)
	}
	err := stream.ObserveLifecycle(func(startCtx context.Context) (context.Context, func(prompty.StreamStatus)) {
		spanCtx, span := begin(startCtx, t.cfg, obs)
		return spanCtx, func(status prompty.StreamStatus) {
			result := obs
			result.Response, result.Err, result.StreamStatus = status.Result, status.Err, &status
			span.finish(result)
		}
	})
	if err != nil {
		return prompty.ErrorStream(ctx, mode, err)
	}
	return stream
}

func invocationObservation(exec *prompty.PromptExecution) Observation {
	var obs Observation
	obs.Operation = "generate_content"
	if exec != nil {
		obs.Execution = exec.Clone()
		if exec.ModelOptions != nil {
			obs.RequestModel = exec.ModelOptions.Model
		}
		if exec.Requirements.Override != nil {
			obs.Provider = exec.Requirements.Override.Identity.Provider
		}
	}
	return obs
}

func (t *tracingInvoker) scopedStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	if exec != nil {
		if err := prompty.ValidateExecutionToolScope(exec); err != nil {
			return prompty.ErrorStream(ctx, mode, err)
		}
	}
	return t.next.ExecuteStream(ctx, exec, mode)
}
