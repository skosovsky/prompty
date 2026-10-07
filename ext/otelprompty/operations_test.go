package otelprompty

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/prompty"
)

type responseOperationFixture struct {
	result   *prompty.RemoteResponseResult
	err      error
	calls    int
	observed bool
}

func (f *responseOperationFixture) Create(
	ctx context.Context,
	_ prompty.RemoteResponseRequest,
) (*prompty.RemoteResponseResult, error) {
	f.calls++
	f.observed = trace.SpanFromContext(ctx).SpanContext().IsValid()
	return f.result, f.err
}

func (f *responseOperationFixture) Get(
	ctx context.Context,
	_ prompty.RemoteResponseHandle,
	_ ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	return f.Create(ctx, prompty.RemoteResponseRequest{})
}

func (f *responseOperationFixture) Cancel(
	ctx context.Context,
	_ prompty.RemoteResponseHandle,
	_ ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	return f.Create(ctx, prompty.RemoteResponseRequest{})
}

type batchOperationFixture struct{ result *prompty.BatchResult }

func (f batchOperationFixture) Submit(context.Context, prompty.BatchSubmitRequest) (*prompty.BatchResult, error) {
	return f.result, nil
}

func (f batchOperationFixture) Get(
	context.Context,
	prompty.BatchHandle,
	...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return f.result, nil
}

func (f batchOperationFixture) Cancel(
	context.Context,
	prompty.BatchHandle,
	...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return f.result, nil
}

func (f batchOperationFixture) Results(
	context.Context,
	prompty.BatchHandle,
	prompty.BatchResultsOptions,
) (*prompty.BatchResult, error) {
	return f.result, nil
}

type cacheOperationFixture struct{ result *prompty.CacheResourceResult }

func (f cacheOperationFixture) Create(
	context.Context,
	prompty.CacheResourceCreateRequest,
) (*prompty.CacheResourceResult, error) {
	return f.result, nil
}

func (f cacheOperationFixture) Get(
	context.Context,
	prompty.CacheResourceHandle,
	...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	return f.result, nil
}

func (f cacheOperationFixture) UpdateExpiry(
	context.Context,
	prompty.CacheResourceHandle,
	prompty.CacheResourceExpiry,
	...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	return f.result, nil
}

func (f cacheOperationFixture) Delete(
	context.Context,
	prompty.CacheResourceHandle,
	...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	return f.result, nil
}

func operationExporter(t *testing.T) (*tracetest.InMemoryExporter, Option) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	return exporter, WithTracerProvider(provider)
}

func TestRemoteOperationsPreserveResponseAndErrorWithoutRetrievalUsage(t *testing.T) {
	t.Parallel()
	// Arrange.
	exporter, option := operationExporter(t)
	result := &prompty.RemoteResponseResult{
		Handle: &prompty.RemoteResponseHandle{
			Identity:       prompty.ProfileIdentity{Provider: "openai", Model: "effective"},
			Status:         prompty.RemoteResponseCompleted,
			ProviderID:     "secret-id",
			AccountBinding: "secret-account",
		},
		Response: &prompty.Response{
			Outcome: prompty.OutcomeCompleted,
			Usage:   prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}},
		},
	}
	failure := errors.New("credential secret-error")
	fixture := &responseOperationFixture{result: result, err: failure}
	manager := WrapRemoteResponses(fixture, option)
	// Act.
	created, createErr := manager.Create(context.Background(), prompty.RemoteResponseRequest{})
	got, getErr := manager.Get(context.Background(), *result.Handle)
	canceled, cancelErr := manager.Cancel(context.Background(), *result.Handle)
	// Assert.
	require.Same(t, result, created)
	require.Same(t, result, got)
	require.Same(t, result, canceled)
	require.ErrorIs(t, createErr, failure)
	require.ErrorIs(t, getErr, failure)
	require.ErrorIs(t, cancelErr, failure)
	require.Equal(t, 3, fixture.calls)
	require.True(t, fixture.observed)
	spans := exporter.GetSpans()
	require.Len(t, spans, 3)
	for index, span := range spans {
		text := fmt.Sprint(span.Attributes, span.Events, span.Status)
		require.NotContains(t, text, "secret")
		hasUsage := false
		for _, attr := range span.Attributes {
			if strings.HasPrefix(string(attr.Key), "gen_ai.usage.") ||
				strings.HasPrefix(string(attr.Key), "prompty.usage.") {
				hasUsage = true
			}
		}
		require.Equal(t, index == 0, hasUsage)
		require.Contains(t, text, "completed")
	}
}

func TestBatchAndCacheResourceObservationsNeverBillGenerationUsage(t *testing.T) {
	t.Parallel()
	// Arrange.
	exporter, option := operationExporter(t)
	batchResult := &prompty.BatchResult{
		Handle: &prompty.BatchHandle{Provider: "openai", Status: prompty.BatchEnded, ProviderID: "secret-job"},
		Items: []prompty.BatchItemResult{
			{
				ID: "secret-item",
				Response: &prompty.Response{
					Usage: prompty.Usage{TotalTokens: 99, Known: []prompty.UsageCounter{prompty.UsageTotal}},
				},
			},
		},
	}
	tokenCount := int64(0)
	cacheResult := &prompty.CacheResourceResult{
		Handle: &prompty.CacheResourceHandle{
			Identity:   prompty.ProfileIdentity{Provider: "gemini"},
			State:      prompty.CacheResourceActive,
			ProviderID: "secret-cache",
		},
		TokenCount: &tokenCount,
	}
	batch := WrapBatch(batchOperationFixture{batchResult}, option)
	cache := WrapCache(cacheOperationFixture{cacheResult}, option)
	// Act.
	submitted, err := batch.Submit(context.Background(), prompty.BatchSubmitRequest{})
	require.NoError(t, err)
	for range 2 {
		retrieved, retrieveErr := batch.Results(
			context.Background(),
			*batchResult.Handle,
			prompty.BatchResultsOptions{},
		)
		require.NoError(t, retrieveErr)
		require.Same(t, batchResult, retrieved)
	}
	_, err = batch.Get(context.Background(), *batchResult.Handle)
	require.NoError(t, err)
	_, err = batch.Cancel(context.Background(), *batchResult.Handle)
	require.NoError(t, err)
	created, err := cache.Create(context.Background(), prompty.CacheResourceCreateRequest{})
	require.NoError(t, err)
	_, err = cache.Get(context.Background(), *cacheResult.Handle)
	require.NoError(t, err)
	_, err = cache.UpdateExpiry(context.Background(), *cacheResult.Handle, prompty.CacheResourceExpiry{})
	require.NoError(t, err)
	_, err = cache.Delete(context.Background(), *cacheResult.Handle)
	require.NoError(t, err)
	// Assert.
	require.Same(t, batchResult, submitted)
	require.Same(t, cacheResult, created)
	spans := exporter.GetSpans()
	require.Len(t, spans, 9)
	for _, span := range spans {
		text := fmt.Sprint(span.Attributes, span.Events, span.Status)
		require.NotContains(t, text, "secret")
		for _, attr := range span.Attributes {
			require.False(
				t,
				(strings.HasPrefix(string(attr.Key), "gen_ai.usage.") || strings.HasPrefix(string(attr.Key), "prompty.usage.")),
			)
		}
	}
	require.Contains(t, fmt.Sprint(spans[5].Attributes), "prompty.resource.token_count")
}

type responseStreamFixture struct {
	*prompty.Stream

	result *prompty.RemoteResponseResult
}

func (f *responseStreamFixture) RemoteResult() *prompty.RemoteResponseResult { return f.result }

type responseStreamingFixture struct{ stream prompty.RemoteResponseStream }

func (f responseStreamingFixture) CreateStream(
	context.Context,
	prompty.RemoteResponseRequest,
) prompty.RemoteResponseStream {
	return f.stream
}

func TestRemoteStreamingObservesOriginalLazyLifecycle(t *testing.T) {
	t.Parallel()
	for _, consume := range []bool{false, true} {
		t.Run(strconv.FormatBool(consume), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exporter, option := operationExporter(t)
			calls := 0
			original := &responseStreamFixture{
				Stream: prompty.NewStream(
					context.Background(),
					prompty.StreamNative,
					func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
						calls++
						return func(yield func(*prompty.ResponseChunk, error) bool) {
							yield(
								&prompty.ResponseChunk{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeCompleted},
								nil,
							)
						}
					},
				),
				result: &prompty.RemoteResponseResult{
					Handle: &prompty.RemoteResponseHandle{Status: prompty.RemoteResponseCompleted},
				},
			}
			manager := WrapResponseStreaming(responseStreamingFixture{original}, option)
			// Act.
			wrapped := manager.CreateStream(context.Background(), prompty.RemoteResponseRequest{})
			require.Same(t, original, wrapped)
			require.Empty(t, exporter.GetSpans())
			if consume {
				for _, err := range wrapped.Events() {
					require.NoError(t, err)
				}
			}
			require.NoError(t, wrapped.Close())
			// Assert.
			if consume {
				require.Equal(t, 1, calls)
				require.Len(t, exporter.GetSpans(), 1)
			} else {
				require.Zero(t, calls)
				require.Empty(t, exporter.GetSpans())
			}
		})
	}
}

func TestOperationProjectionOwnsReturnedResources(t *testing.T) {
	t.Parallel()
	// Arrange.
	_, option := operationExporter(t)
	result := &prompty.BatchResult{
		Handle: &prompty.BatchHandle{Items: []prompty.BatchItemBinding{{ID: "original"}}},
		Items: []prompty.BatchItemResult{
			{
				SourceJSON: prompty.JSONDocument(`{"owned":true}`),
				Response:   &prompty.Response{Content: []prompty.ContentPart{prompty.TextPart{Text: "original"}}},
			},
		},
		Completeness: prompty.BatchCompleteness{MissingIDs: []string{"original"}},
	}
	manager := WrapBatch(
		batchOperationFixture{result},
		option,
		WithProjection(func(_ context.Context, obs Observation) []attribute.KeyValue {
			owned := obs.Resource.(*prompty.BatchResult)
			owned.Handle.Items[0].ID = "changed"
			owned.Items[0].SourceJSON[0] = '!'
			owned.Items[0].Response.Content[0] = prompty.TextPart{Text: "changed"}
			owned.Completeness.MissingIDs[0] = "changed"
			return nil
		}),
	)
	// Act.
	returned, err := manager.Results(context.Background(), prompty.BatchHandle{}, prompty.BatchResultsOptions{})
	// Assert.
	require.NoError(t, err)
	require.Same(t, result, returned)
	require.Equal(t, "original", result.Handle.Items[0].ID)
	require.JSONEq(t, `{"owned":true}`, string(result.Items[0].SourceJSON))
	require.Equal(t, "original", result.Items[0].Response.Content[0].(prompty.TextPart).Text)
	require.Equal(t, []string{"original"}, result.Completeness.MissingIDs)
}

func TestRemoteStreamingSnapshotsRequestBeforeConsumption(t *testing.T) {
	t.Parallel()
	// Arrange.
	exporter, option := operationExporter(t)
	execution := &prompty.PromptExecution{ModelOptions: &prompty.ModelOptions{Model: "selected"}}
	original := &responseStreamFixture{
		Stream: prompty.NewStream(
			context.Background(),
			prompty.StreamNative,
			func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
				return func(yield func(*prompty.ResponseChunk, error) bool) {
					yield(
						&prompty.ResponseChunk{
							Kind:    prompty.StreamFinish,
							Outcome: prompty.OutcomeCompleted,
							Model:   "observed",
						},
						nil,
					)
				}
			},
		),
		result: nil,
	}
	manager := WrapResponseStreaming(responseStreamingFixture{original}, option)
	// Act.
	stream := manager.CreateStream(context.Background(), prompty.RemoteResponseRequest{Execution: execution})
	execution.ModelOptions.Model = "mutated"
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert.
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	text := fmt.Sprint(spans[0].Attributes)
	require.Contains(t, text, "selected")
	require.Contains(t, text, "observed")
	require.NotContains(t, text, "mutated")
}

func TestRemoteStreamingMissingPortHandleFailsAtConsumption(t *testing.T) {
	t.Parallel()
	// Arrange.
	exporter, option := operationExporter(t)
	manager := WrapResponseStreaming(responseStreamingFixture{stream: nil}, option)
	// Act.
	stream := manager.CreateStream(context.Background(), prompty.RemoteResponseRequest{})
	require.Empty(t, exporter.GetSpans())
	for _, err := range stream.Events() {
		require.ErrorIs(t, err, prompty.ErrStreamProtocol)
	}
	// Assert.
	require.Len(t, exporter.GetSpans(), 1)
	require.Equal(t, prompty.StreamFailed, stream.Status().State)
	require.NoError(t, stream.Close())
	require.Len(t, exporter.GetSpans(), 1)
}

func TestRemoteRetrievalKeepsGenerationMetadataWithoutRecountingUsage(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"get", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exporter, option := operationExporter(t)
			result := &prompty.RemoteResponseResult{
				Handle: &prompty.RemoteResponseHandle{
					Identity: prompty.ProfileIdentity{Provider: "openai", Model: "selected"},
					Status:   prompty.RemoteResponseCompleted,
				},
				Response: &prompty.Response{
					Model:        "observed",
					Outcome:      prompty.OutcomeRefusal,
					FinishReason: "refusal",
					Usage: prompty.Usage{
						PromptTokens:              4,
						CompletionTokens:          2,
						TotalTokens:               6,
						PromptTokensCached:        1,
						PromptTokensCacheCreation: 0,
						CompletionTokensReasoning: 1,
						Known: []prompty.UsageCounter{
							prompty.UsagePrompt,
							prompty.UsageCompletion,
							prompty.UsageTotal,
							prompty.UsageCacheRead,
							prompty.UsageCacheWrite,
							prompty.UsageReasoning,
						},
					},
				},
			}
			fixture := &responseOperationFixture{result: result, err: prompty.ErrCapture}
			manager := WrapRemoteResponses(fixture, option)
			// Act.
			var returned *prompty.RemoteResponseResult
			var err error
			if operation == "get" {
				returned, err = manager.Get(context.Background(), *result.Handle)
			} else {
				returned, err = manager.Cancel(context.Background(), *result.Handle)
			}
			// Assert.
			require.Same(t, result, returned)
			require.ErrorIs(t, err, prompty.ErrCapture)
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			attrs := spanAttributes(spans[0])
			require.Equal(t, "observed", attrs["gen_ai.response.model"])
			require.Equal(t, "refusal", attrs["prompty.outcome"])
			require.Equal(t, []string{"refusal"}, attrs["gen_ai.response.finish_reasons"])
			require.Equal(t, "completed", attrs["prompty.resource.status"])
			require.Equal(t, "capture_failure", attrs["error.type"])
			for key := range attrs {
				require.False(
					t,
					strings.HasPrefix(key, "gen_ai.usage.") || strings.HasPrefix(key, "prompty.usage."),
					key,
				)
			}
		})
	}
}
