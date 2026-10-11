package otelprompty

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func memoryTracer(t *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	return provider, exporter
}
func spanAttributes(span tracetest.SpanStub) map[string]any {
	attrs := map[string]any{}
	for _, attr := range span.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}
	return attrs
}

func TestTracingKnownMetadataSurvivesErrorAndDefaultPrivacy(t *testing.T) {
	t.Parallel()
	// Arrange.
	provider, exporter := memoryTracer(t)
	secret := "secret-do-not-export"
	response := &prompty.Response{
		Model:        "observed",
		Outcome:      prompty.OutcomeIncomplete,
		FinishReason: "length",
		Content:      []prompty.ContentPart{prompty.TextPart{Text: secret}},
		Usage: prompty.Usage{
			PromptTokens:              0,
			CompletionTokens:          12,
			TotalTokens:               12,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 3,
			CompletionTokensReasoning: 8,
			Known: []prompty.UsageCounter{
				prompty.UsagePrompt,
				prompty.UsageCompletion,
				prompty.UsageTotal,
				prompty.UsageCacheRead,
				prompty.UsageCacheWrite,
				prompty.UsageReasoning,
			},
		},
		Diagnostics: prompty.ExecutionDiagnostics{
			Profile: prompty.ProfileIdentity{Provider: "fixture", Endpoint: "https://" + secret, Model: "selected"},
		},
	}
	failure := fmt.Errorf("%w: %s", prompty.ErrCapture, secret)
	base := &invokerStub{
		generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) { return response, failure },
	}
	execution := prompty.SimplePrompt(secret)
	execution.Metadata.ID = secret
	execution.ModelOptions = &prompty.ModelOptions{Model: "requested"}
	// Act.
	result, err := WithTracing(WithTracerProvider(provider))(base).Execute(context.Background(), execution)
	// Assert.
	require.Same(t, response, result)
	require.ErrorIs(t, err, failure)
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	attrs := spanAttributes(spans[0])
	require.Equal(t, "requested", attrs["gen_ai.request.model"])
	require.Equal(t, "observed", attrs["gen_ai.response.model"])
	require.Equal(t, "fixture", attrs["gen_ai.provider.name"])
	require.Equal(t, int64(0), attrs["gen_ai.usage.input_tokens"])
	require.Equal(t, int64(0), attrs["gen_ai.usage.cache_read.input_tokens"])
	require.Equal(t, int64(12), attrs["prompty.usage.total_tokens"])
	require.Equal(t, int64(3), attrs["gen_ai.usage.cache_write.input_tokens"])
	require.Equal(t, int64(8), attrs["gen_ai.usage.reasoning.output_tokens"])
	require.Equal(t, "incomplete", attrs["prompty.outcome"])
	require.Equal(t, "capture_failure", attrs["error.type"])
	require.Empty(t, spans[0].Events)
	require.NotContains(t, fmt.Sprint(spans), secret)
}

func TestTracingUnknownCountersAndNoFabricatedResponseModel(t *testing.T) {
	t.Parallel()
	// Arrange.
	provider, exporter := memoryTracer(t)
	base := &invokerStub{generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
		return &prompty.Response{
			Outcome:     prompty.OutcomeCompleted,
			Usage:       prompty.Usage{TotalTokens: 77},
			Diagnostics: prompty.ExecutionDiagnostics{Profile: prompty.ProfileIdentity{Model: "profile"}},
		}, nil
	}}
	// Act.
	_, err := WithTracing(WithTracer(provider.Tracer("owned")))(base).Execute(context.Background(), nil)
	// Assert.
	require.NoError(t, err)
	span := exporter.GetSpans()[0]
	attrs := spanAttributes(span)
	require.Equal(t, "owned", span.InstrumentationScope.Name)
	require.NotContains(t, attrs, "gen_ai.response.model")
	require.NotContains(t, attrs, "prompty.usage.total_tokens")
	require.NotContains(t, attrs, "gen_ai.usage.input_tokens")
}

func TestTracingClassificationAndProjectionOwnership(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		err      error
		outcome  prompty.Outcome
		category string
		status   codes.Code
	}{
		{"preflight", adapter.ErrUnsupportedCapability, "", "preflight_rejection", codes.Error},
		{"parse_unknown_content", adapter.ErrUnsupportedContentType, "", "unsupported_content", codes.Error},
		{"unknown_role_stage", adapter.ErrUnsupportedRole, "", "unsupported_role", codes.Error},
		{"schema", prompty.ErrSchemaMismatch, prompty.OutcomeCompleted, "schema_failure", codes.Error},
		{"refusal", nil, prompty.OutcomeRefusal, "provider_refusal", codes.Unset},
		{"incomplete", &prompty.OutcomeError{Outcome: prompty.OutcomeIncomplete}, prompty.OutcomeIncomplete, "provider_incomplete", codes.Unset},
		{"cancel", context.Canceled, "", "cancellation", codes.Error},
		{"unknown", errors.New("SECRET"), "", "operation_failure", codes.Error},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			provider, exporter := memoryTracer(t)
			response := &prompty.Response{
				Outcome: test.outcome,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "original"}},
			}
			base := &invokerStub{generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
				if test.outcome == "" {
					return nil, test.err
				}
				return response, test.err
			}}
			execution := prompty.SimplePrompt("original")
			calls := 0
			projection := func(_ context.Context, obs Observation) []attribute.KeyValue {
				calls++
				obs.Execution.Messages[0].Content[0] = prompty.TextPart{Text: "changed"}
				if obs.Response != nil {
					obs.Response.Content[0] = prompty.TextPart{Text: "changed"}
				}
				return []attribute.KeyValue{attribute.String("host.redacted", "explicit")}
			}
			// Act.
			_, _ = WithTracing(
				WithTracerProvider(provider),
				WithProjection(projection),
			)(
				base,
			).Execute(context.Background(), execution)
			// Assert.
			span := exporter.GetSpans()[0]
			attrs := spanAttributes(span)
			require.Equal(t, test.category, attrs["prompty.error.category"])
			require.Equal(t, test.status, span.Status.Code)
			require.Equal(t, "explicit", attrs["host.redacted"])
			require.Equal(t, 1, calls)
			require.Equal(t, "original", execution.Messages[0].Content[0].(prompty.TextPart).Text)
			require.Equal(t, "original", response.Content[0].(prompty.TextPart).Text)
			require.NotContains(t, fmt.Sprint(span), "SECRET")
		})
	}
}

func TestTracingLazyStreamExactlyOneSpanAndOriginalHandle(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unused", "close_before", "terminal", "consumer", "close_running", "parent_cancel", "error"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			runStreamScenario(t, scenario)
		})
	}
}

func runStreamScenario(t *testing.T, scenario string) {
	t.Helper()
	// Arrange.
	provider, exporter := memoryTracer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var handle *prompty.Stream
	calls := 0
	base := &invokerStub{
		generateStream: func(sourceCtx context.Context, _ *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
			calls++
			require.NotNil(t, sourceCtx.Done())
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				if scenario == "error" {
					yield(nil, errors.New("SECRET"))
					return
				}
				if !yield(
					&prompty.ResponseChunk{
						Kind:     prompty.StreamDelta,
						Identity: prompty.StreamIdentity{PartIDs: []string{"part"}},
						Content:  []prompty.ContentPart{prompty.TextPart{Text: "SECRET"}},
					},
					nil,
				) {
					return
				}
				yield(
					&prompty.ResponseChunk{
						Kind:    prompty.StreamFinish,
						Model:   "observed",
						Outcome: prompty.OutcomeCompleted,
						Usage: prompty.Usage{
							TotalTokens: 0,
							Known:       []prompty.UsageCounter{prompty.UsageTotal},
						},
					},
					nil,
				)
			}
		},
	}
	wrapper := WithTracing(WithTracerProvider(provider))(base)
	// Act.
	handle = wrapper.ExecuteStream(ctx, prompty.SimplePrompt("SECRET"), prompty.StreamNative)
	require.Empty(t, exporter.GetSpans())
	if scenario == "unused" {
		require.Zero(t, calls)
		return
	}
	if scenario == "close_before" {
		require.NoError(t, handle.Close())
		require.Empty(t, exporter.GetSpans())
		require.Zero(t, calls)
		return
	}
	for chunk := range handle.Events() {
		if chunk != nil && chunk.Kind == prompty.StreamDelta {
			switch scenario {
			case "consumer":
				break
			case "close_running":
				require.NoError(t, handle.Close())
			case "parent_cancel":
				cancel()
			}
		}
		if scenario == "consumer" {
			break
		}
	}
	require.NoError(t, handle.Close())
	for range handle.Events() {
	}
	// Assert.
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, 1, calls)
	attrs := spanAttributes(spans[0])
	require.NotContains(t, fmt.Sprint(spans), "SECRET")
	switch scenario {
	case "consumer":
		require.Equal(t, "consumer_stop", attrs["prompty.error.category"])
	case "close_running":
		require.Equal(t, "closed", attrs["prompty.error.category"])
	case "parent_cancel":
		require.Equal(t, "cancellation", attrs["prompty.error.category"])
	case "terminal":
		require.Equal(t, int64(0), attrs["prompty.usage.total_tokens"])
		require.Equal(t, "observed", attrs["gen_ai.response.model"])
	}
}

func TestTracingRetainedOutcomeCannotMaskObservedFailure(t *testing.T) {
	t.Parallel()
	for _, outcome := range []prompty.Outcome{prompty.OutcomeRefusal, prompty.OutcomeIncomplete} {
		for _, test := range []struct {
			name     string
			err      error
			category string
			status   codes.Code
		}{
			{"transport", &net.DNSError{Err: "SECRET", Name: "SECRET", IsTimeout: true}, "transport_failure", codes.Error},
			{"protocol", fmt.Errorf("%w: SECRET", prompty.ErrStreamProtocol), "protocol_failure", codes.Error},
			{"unknown", errors.New("SECRET"), "operation_failure", codes.Error},
			{"outcome", &prompty.OutcomeError{Outcome: outcome}, "provider_" + string(outcome), codes.Unset},
			{"nil_error", nil, "provider_" + string(outcome), codes.Unset},
			{"joined_unknown", errors.Join(&prompty.OutcomeError{Outcome: outcome}, errors.New("SECRET-independent")), "operation_failure", codes.Error},
			{"joined_usage", errors.Join(&prompty.OutcomeError{Outcome: outcome}, prompty.ErrUsage), "operation_failure", codes.Error},
			{"nested_wrapped_unknown", fmt.Errorf("SECRET: %w", errors.Join(fmt.Errorf("SECRET: %w", &prompty.OutcomeError{Outcome: outcome}), errors.Join(errors.New("SECRET-independent"), &prompty.OutcomeError{Outcome: outcome}))), "operation_failure", codes.Error},
			{"pure_join", errors.Join(fmt.Errorf("SECRET: %w", &prompty.OutcomeError{Outcome: outcome}), errors.Join(&prompty.OutcomeError{Outcome: outcome}, fmt.Errorf("SECRET: %w", &prompty.OutcomeError{Outcome: outcome}))), "provider_" + string(outcome), codes.Unset},
			{"wrapped_outcome", fmt.Errorf("SECRET: %w", &prompty.OutcomeError{Outcome: outcome}), "provider_" + string(outcome), codes.Unset},
			{"mismatched_outcome", &prompty.OutcomeError{Outcome: prompty.OutcomeCompleted}, "operation_failure", codes.Error},
			{"mixed_outcomes", errors.Join(&prompty.OutcomeError{Outcome: outcome}, &prompty.OutcomeError{Outcome: prompty.OutcomeCompleted}), "operation_failure", codes.Error},
			{"bare_sentinel", prompty.ErrResponseOutcome, "operation_failure", codes.Error},
			{"joined_protocol", errors.Join(&prompty.OutcomeError{Outcome: outcome}, prompty.ErrStreamProtocol), "protocol_failure", codes.Error},
			{"joined_transport", errors.Join(&prompty.OutcomeError{Outcome: outcome}, &net.DNSError{Err: "SECRET", Name: "SECRET", IsTimeout: true}), "transport_failure", codes.Error},
			{"joined_capture", errors.Join(&prompty.OutcomeError{Outcome: outcome}, prompty.ErrCapture), "capture_failure", codes.Error},
			{"joined_schema", errors.Join(&prompty.OutcomeError{Outcome: outcome}, prompty.ErrSchemaMismatch), "schema_failure", codes.Error},
		} {
			t.Run(string(outcome)+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				// Arrange.
				provider, exporter := memoryTracer(t)
				response := &prompty.Response{
					Model:        "observed",
					Outcome:      outcome,
					FinishReason: "known",
					Usage: prompty.Usage{
						PromptTokens:     0,
						CompletionTokens: 0,
						TotalTokens:      0,
						Known: []prompty.UsageCounter{
							prompty.UsagePrompt,
							prompty.UsageCompletion,
							prompty.UsageTotal,
						},
					},
				}
				base := &invokerStub{
					generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) { return response, test.err },
				}
				// Act.
				result, err := WithTracing(
					WithTracerProvider(provider),
				)(
					base,
				).Execute(context.Background(), prompty.SimplePrompt("SECRET"))
				// Assert.
				require.Same(t, response, result)
				require.ErrorIs(t, err, test.err)
				spans := exporter.GetSpans()
				require.Len(t, spans, 1)
				attrs := spanAttributes(spans[0])
				require.Equal(t, test.category, attrs["prompty.error.category"])
				if test.err != nil {
					require.Equal(t, test.category, attrs["error.type"])
				} else {
					require.NotContains(t, attrs, "error.type")
				}
				require.Equal(t, test.status, spans[0].Status.Code)
				require.Equal(t, string(outcome), attrs["prompty.outcome"])
				require.Equal(t, "observed", attrs["gen_ai.response.model"])
				require.Equal(t, []string{"known"}, attrs["gen_ai.response.finish_reasons"])
				require.Equal(t, int64(0), attrs["gen_ai.usage.input_tokens"])
				require.Equal(t, int64(0), attrs["gen_ai.usage.output_tokens"])
				require.Equal(t, int64(0), attrs["prompty.usage.total_tokens"])
				require.NotContains(t, fmt.Sprint(spans), "SECRET")
			})
		}
	}
}
