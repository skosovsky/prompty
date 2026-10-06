// Package otelprompty provides OpenTelemetry tracing middleware for prompty.
// Use WithTracing to wrap an Invoker and record spans for Execute and ExecuteStream.
package otelprompty

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/prompty"
)

const tracerName = "github.com/skosovsky/prompty/ext/otelprompty"

// WithTracing returns a Middleware that records spans for each invocation.
// Uses the global tracer provider; call otel.SetTracerProvider before use if needed.
func WithTracing(opts ...Option) prompty.Middleware {
	cfg := &config{tracerName: tracerName}
	for _, opt := range opts {
		opt(cfg)
	}
	tracer := otel.Tracer(cfg.tracerName)
	return func(next prompty.Invoker) prompty.Invoker {
		return &tracingInvoker{next: next, tracer: tracer}
	}
}

type config struct {
	tracerName string
}

// Option configures WithTracing.
type Option func(*config)

// WithTracerName sets the tracer name. Default is the package path.
func WithTracerName(name string) Option {
	return func(c *config) { c.tracerName = name }
}

type tracingInvoker struct {
	next   prompty.Invoker
	tracer trace.Tracer
}

func (t *tracingInvoker) Execute(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	ctx, span := t.tracer.Start(ctx, "prompty.Execute")
	defer span.End()
	attrs := execAttrs(exec)
	span.SetAttributes(attrs...)
	start := time.Now()
	defer func() {
		span.SetAttributes(attribute.Int64("prompty.latency_ms", time.Since(start).Milliseconds()))
	}()
	if exec != nil {
		if scopeErr := prompty.ValidateExecutionToolScope(exec); scopeErr != nil {
			span.RecordError(scopeErr)
			span.SetStatus(codes.Error, scopeErr.Error())
			return nil, scopeErr
		}
	}
	resp, err := t.next.Execute(ctx, exec)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, err
	}
	if resp != nil {
		if resp.Usage.TotalTokens > 0 {
			span.SetAttributes(attribute.Int("prompty.tokens_total", resp.Usage.TotalTokens))
		}
		if resp.FinishReason != "" {
			span.SetAttributes(attribute.String("prompty.finish_reason", resp.FinishReason))
		}
	}
	return resp, nil
}

func (t *tracingInvoker) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	stream := t.scopedStream(ctx, exec, mode)
	if stream == nil {
		return prompty.ErrorStream(ctx, mode, prompty.ErrStreamProtocol)
	}
	err := stream.ObserveLifecycle(func(ctx context.Context) (context.Context, func(prompty.StreamStatus)) {
		streamCtx, span := t.tracer.Start(ctx, "prompty.ExecuteStream")
		span.SetAttributes(execAttrs(exec)...)
		start := time.Now()
		return streamCtx, func(status prompty.StreamStatus) {
			defer span.End()
			span.SetAttributes(attribute.Int64("prompty.latency_ms", time.Since(start).Milliseconds()))
			if status.Err != nil {
				span.RecordError(status.Err)
				span.SetStatus(codes.Error, status.Err.Error())
			}
			if status.Result.Usage.TotalTokens > 0 {
				span.SetAttributes(attribute.Int("prompty.tokens_total", status.Result.Usage.TotalTokens))
			}
			if status.Result.FinishReason != "" {
				span.SetAttributes(attribute.String("prompty.finish_reason", status.Result.FinishReason))
			}
		}
	})
	if err != nil {
		return prompty.ErrorStream(ctx, mode, err)
	}
	return stream
}

func execAttrs(exec *prompty.PromptExecution) []attribute.KeyValue {
	if exec == nil {
		return nil
	}
	attrs := []attribute.KeyValue{
		attribute.String("prompty.prompt_id", exec.Metadata.ID),
	}
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		attrs = append(attrs, attribute.String("prompty.model", exec.ModelOptions.Model))
	}
	return attrs
}

// Rejected declarations still enter the same observed stream lifecycle.
func (t *tracingInvoker) scopedStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	if exec != nil {
		if scopeErr := prompty.ValidateExecutionToolScope(exec); scopeErr != nil {
			return prompty.ErrorStream(ctx, mode, scopeErr)
		}
	}
	return t.next.ExecuteStream(ctx, exec, mode)
}
