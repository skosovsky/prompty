package otelprompty

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const tracerName = "github.com/skosovsky/prompty/ext/otelprompty"

var errCallerPanic = errors.New("prompty: caller port panic")

// SemanticConventionsRevision pins the Development GenAI conventions used here.
const SemanticConventionsRevision = "4f85037ef86e92c510d2ef881a58f1076f6fc0e4"

// Observation is a projection input. DTO collections are isolated from the
// invocation. Err preserves identity; the host must not mutate error objects.
// Resource is an owned operation-specific DTO supplied by explicit wrappers.
type Observation struct {
	// SuppressGenerationUsage retains response metadata for resource observations
	// without billing a previously observed generation again.
	SuppressGenerationUsage bool
	Operation               string
	Provider                string
	RequestModel            string
	ResponseModel           string
	Outcome                 string
	ErrorCategory           string
	Execution               *prompty.PromptExecution
	Response                *prompty.Response
	Err                     error
	StreamStatus            *prompty.StreamStatus
	Resource                any
	ResourceStatus          string
	ResourceKind            string
	ResourceTokenCount      *int64
}

// Projection explicitly selects/redacts additional telemetry. Returned values
// may contain sensitive information; exporter policy remains host-owned.
type Projection func(context.Context, Observation) []attribute.KeyValue

// Option configures invocation and explicit operation wrappers.
type Option func(*config)
type config struct {
	tracerName string
	tracer     trace.Tracer
	provider   trace.TracerProvider
	projection Projection
}

// WithTracerName selects the instrumentation scope name.
func WithTracerName(name string) Option { return func(c *config) { c.tracerName = name } }

// WithTracer selects a caller-owned tracer without global configuration.
func WithTracer(tracer trace.Tracer) Option { return func(c *config) { c.tracer = tracer } }

// WithTracerProvider selects a caller-owned provider without global configuration.
func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(c *config) { c.provider = provider }
}

// WithProjection enables explicit host projection of content and sensitive metadata.
func WithProjection(projection Projection) Option {
	return func(c *config) { c.projection = projection }
}

func resolveConfig(opts []Option) *config {
	cfg := new(config)
	cfg.tracerName = tracerName
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	if cfg.tracer == nil {
		if cfg.provider != nil {
			cfg.tracer = cfg.provider.Tracer(cfg.tracerName)
		} else {
			cfg.tracer = otel.Tracer(cfg.tracerName)
		}
	}
	return cfg
}

type operationSpan struct {
	ctx  context.Context
	span trace.Span
	cfg  *config
	once sync.Once
}

// begin transfers span ownership to operationSpan.finish, including lazy lifecycle closeout.
//
//nolint:spancheck // Ownership is returned explicitly and ended exactly once by finish.
func begin(ctx context.Context, cfg *config, obs Observation) (context.Context, *operationSpan) {
	name := obs.Operation
	if obs.RequestModel != "" {
		name += " " + obs.RequestModel
	}
	spanCtx, span := cfg.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(requestAttributes(obs)...)
	return spanCtx, &operationSpan{ctx: spanCtx, span: span, cfg: cfg, once: sync.Once{}}
}
func (s *operationSpan) finish(obs Observation) {
	s.once.Do(func() {
		defer s.span.End()
		obs = normalizeObservation(obs)
		s.span.SetAttributes(resultAttributes(obs)...)
		if obs.Err != nil {
			s.span.SetAttributes(attribute.String("error.type", obs.ErrorCategory))
			if obs.ErrorCategory != "provider_refusal" && obs.ErrorCategory != "provider_incomplete" &&
				obs.ErrorCategory != "consumer_stop" &&
				obs.ErrorCategory != "closed" {
				s.span.SetStatus(codes.Error, obs.ErrorCategory)
			}
		}
		if s.cfg.projection != nil {
			s.span.SetAttributes(s.cfg.projection(s.ctx, cloneObservation(obs))...)
		}
	})
}

// finishOnReturn is deferred directly by synchronous wrappers. The panic value
// stays on the caller's stack and is never supplied to telemetry projection.
func (s *operationSpan) finishOnReturn(obs *Observation) {
	if value := recover(); value != nil {
		obs.Err, obs.ErrorCategory = errCallerPanic, "caller_panic"
		s.finish(*obs)
		panic(value)
	}
	s.finish(*obs)
}

func requestAttributes(obs Observation) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("prompty.operation.name", obs.Operation),
		attribute.String("prompty.operation.scope", "logical"),
	}
	if obs.Operation == "response.get" {
		attrs = append(attrs, attribute.String("gen_ai.operation.name", "fetch_response"))
	}
	if obs.Operation == "generate_content" || obs.Operation == "chat" {
		attrs = append(attrs, attribute.String("gen_ai.operation.name", obs.Operation))
	}
	if obs.Provider != "" {
		attrs = append(attrs, attribute.String("gen_ai.provider.name", obs.Provider))
	}
	if obs.RequestModel != "" {
		attrs = append(attrs, attribute.String("gen_ai.request.model", obs.RequestModel))
	}
	return attrs
}

func normalizeObservation(obs Observation) Observation {
	if obs.Response != nil {
		if obs.Provider == "" {
			obs.Provider = obs.Response.Diagnostics.Profile.Provider
		}
		if obs.RequestModel == "" {
			obs.RequestModel = obs.Response.Diagnostics.Profile.Model
		}
		if obs.ResponseModel == "" {
			obs.ResponseModel = obs.Response.Model
		}
		if obs.Outcome == "" {
			obs.Outcome = string(obs.Response.Outcome)
		}
	}
	if obs.ErrorCategory == "" {
		obs.ErrorCategory = classifyError(obs.Err, obs.Response)
	}
	if obs.StreamStatus != nil && obs.StreamStatus.State == prompty.StreamCanceled {
		switch obs.StreamStatus.StopReason {
		case prompty.StreamStopConsumer:
			obs.ErrorCategory = "consumer_stop"
		case prompty.StreamStopClosed:
			obs.ErrorCategory = "closed"
		default:
			obs.ErrorCategory = "cancellation"
		}
	}
	return obs
}

func resultAttributes(obs Observation) []attribute.KeyValue {
	attrs := requestAttributes(obs)
	if obs.ResponseModel != "" {
		attrs = append(attrs, attribute.String("gen_ai.response.model", obs.ResponseModel))
	}
	if obs.Outcome != "" {
		attrs = append(attrs, attribute.String("prompty.outcome", obs.Outcome))
	}
	if obs.ErrorCategory != "" {
		attrs = append(attrs, attribute.String("prompty.error.category", obs.ErrorCategory))
	}
	if obs.ResourceKind != "" {
		attrs = append(attrs, attribute.String("prompty.resource.kind", obs.ResourceKind))
	}
	if obs.ResourceStatus != "" {
		attrs = append(attrs, attribute.String("prompty.resource.status", obs.ResourceStatus))
	}
	if obs.ResourceTokenCount != nil {
		attrs = append(attrs, attribute.Int64("prompty.resource.token_count", *obs.ResourceTokenCount))
	}
	if obs.StreamStatus != nil {
		attrs = append(
			attrs,
			attribute.String("prompty.stream.state", string(obs.StreamStatus.State)),
			attribute.String("prompty.stream.mode", string(obs.StreamStatus.Mode)),
		)
	}
	if obs.Response != nil {
		if obs.Response.FinishReason != "" {
			attrs = append(
				attrs,
				attribute.StringSlice("gen_ai.response.finish_reasons", []string{obs.Response.FinishReason}),
			)
		}
		if !obs.SuppressGenerationUsage {
			attrs = append(attrs, usageAttributes(obs.Response.Usage)...)
		}
	}
	return attrs
}

func usageAttributes(u prompty.Usage) []attribute.KeyValue {
	counters := []struct {
		counter prompty.UsageCounter
		key     string
		value   int
	}{
		{prompty.UsagePrompt, "gen_ai.usage.input_tokens", u.PromptTokens},
		{prompty.UsageCompletion, "gen_ai.usage.output_tokens", u.CompletionTokens},
		{prompty.UsageTotal, "prompty.usage.total_tokens", u.TotalTokens},
		{prompty.UsageCacheRead, "gen_ai.usage.cache_read.input_tokens", u.PromptTokensCached},
		{prompty.UsageCacheWrite, "gen_ai.usage.cache_write.input_tokens", u.PromptTokensCacheCreation},
		{prompty.UsageReasoning, "gen_ai.usage.reasoning.output_tokens", u.CompletionTokensReasoning},
	}
	var attrs []attribute.KeyValue
	for _, counter := range counters {
		if slices.Contains(u.Known, counter.counter) {
			attrs = append(attrs, attribute.Int(counter.key, counter.value))
		}
	}
	return attrs
}

func classifyError(err error, resp *prompty.Response) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancellation"
	}
	if errors.Is(err, prompty.ErrCapture) || errors.Is(err, prompty.ErrCaptureUnsupported) ||
		errors.Is(err, prompty.ErrCaptureReport) {
		return "capture_failure"
	}
	if errors.Is(err, prompty.ErrSchemaMismatch) || errors.Is(err, prompty.ErrSchemaJSONDecode) {
		return "schema_failure"
	}
	if errors.Is(err, adapter.ErrUnsupportedContentType) {
		return "unsupported_content"
	}
	if errors.Is(err, adapter.ErrUnsupportedRole) {
		return "unsupported_role"
	}
	if isPreflightError(err) {
		return "preflight_rejection"
	}
	if errors.Is(err, prompty.ErrStreamProtocol) {
		return "protocol_failure"
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return "transport_failure"
	}
	// A retained provider outcome is a separate fact. Only a matching outcome
	// error (or no error) classifies the provider outcome itself; an unrelated
	// unknown error remains an operation failure without guessed transport stage.
	if err != nil && (resp == nil || !onlyOutcomeCauses(err, resp.Outcome, 0)) {
		return "operation_failure"
	}
	if resp != nil {
		if resp.Outcome == prompty.OutcomeRefusal {
			return "provider_refusal"
		}
		if resp.Outcome == prompty.OutcomeIncomplete {
			return "provider_incomplete"
		}
	}
	if err != nil {
		return "operation_failure"
	}
	return ""
}

func cloneObservation(obs Observation) Observation {
	if obs.Execution != nil {
		obs.Execution = obs.Execution.Clone()
	}
	obs.Response = ownedResponse(obs.Response)
	if obs.StreamStatus != nil {
		status := *obs.StreamStatus
		status.Result = ownedResponse(status.Result)
		status.Identity.PartIDs = slices.Clone(status.Identity.PartIDs)
		obs.StreamStatus = &status
	}
	if obs.ResourceTokenCount != nil {
		value := *obs.ResourceTokenCount
		obs.ResourceTokenCount = &value
	}
	return obs
}

func ownedResponse(resp *prompty.Response) *prompty.Response {
	return prompty.CloneResponse(resp)
}

func isPreflightError(err error) bool {
	return errors.Is(err, adapter.ErrUnsupportedCapability) || errors.Is(err, adapter.ErrUnsupportedCombination) ||
		errors.Is(err, adapter.ErrInvalidProfileOverride) ||
		errors.Is(err, adapter.ErrCapabilityRequirement) ||
		errors.Is(err, adapter.ErrUnknownProfile) ||
		errors.Is(err, adapter.ErrInvalidProfile) ||
		errors.Is(err, adapter.ErrNilExecution) ||
		errors.Is(err, adapter.ErrInvalidProviderSettings) ||
		errors.Is(err, adapter.ErrStructuredOutputNotSupported) ||
		errors.Is(err, prompty.ErrToolScope) ||
		errors.Is(err, prompty.ErrCapabilityProfile) ||
		errors.Is(err, prompty.ErrSchemaUnsupported) ||
		errors.Is(err, prompty.ErrSchemaValidatorRequired)
}

// onlyOutcomeCauses checks every leaf, rather than finding one matching sentinel
// among unrelated joined failures. Wrappers preserve their single cause; joins
// qualify only when each branch ends in a matching typed provider outcome.
//
//nolint:errorlint // Inspect each direct node: errors.As would skip unrelated joined leaves.
func onlyOutcomeCauses(err error, outcome prompty.Outcome, depth int) bool {
	// Arbitrary host error trees are not a reason to recurse without a bound.
	if err == nil || depth >= 64 {
		return false
	}
	if provider, ok := err.(*prompty.OutcomeError); ok {
		return provider != nil && provider.Outcome == outcome
	}
	switch cause := err.(type) {
	case interface{ Unwrap() []error }:
		children := cause.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !onlyOutcomeCauses(child, outcome, depth+1) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return onlyOutcomeCauses(cause.Unwrap(), outcome, depth+1)
	default:
		return false
	}
}
