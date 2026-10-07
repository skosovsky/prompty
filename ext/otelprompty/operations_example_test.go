package otelprompty

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/prompty"
)

// This offline fixture uses the same explicit ports as provider adapters. The
// host owns exporter lifetime and chooses which resource metadata is exported.
func ExampleWrapRemoteResponses() {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	result := &prompty.RemoteResponseResult{
		Handle: &prompty.RemoteResponseHandle{
			Identity: prompty.ProfileIdentity{Provider: "openai", Model: "selected"},
			Status:   prompty.RemoteResponseCompleted,
		},
		Response: &prompty.Response{
			Model:   "observed",
			Outcome: prompty.OutcomeCompleted,
			Usage: prompty.Usage{
				PromptTokens:     4,
				CompletionTokens: 2,
				TotalTokens:      6,
				Known: []prompty.UsageCounter{
					prompty.UsagePrompt,
					prompty.UsageCompletion,
					prompty.UsageTotal,
				},
			},
		},
	}
	manager := WrapRemoteResponses(&responseOperationFixture{result: result}, WithTracerProvider(provider))
	created, err := manager.Create(context.Background(), prompty.RemoteResponseRequest{})
	if err != nil {
		panic(err)
	}
	// Retrieval is an explicitly observed resource operation; its response usage
	// remains on the DTO and is not emitted as a second generation's token usage.
	retrieved, err := manager.Get(context.Background(), *created.Handle)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(exporter.GetSpans()), retrieved.Response.Usage.TotalTokens)
	// Output: 2 6
}

func ExampleWrapBatch() {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	result := &prompty.BatchResult{
		Handle: &prompty.BatchHandle{Provider: "anthropic", Status: prompty.BatchEnded},
		Items:  []prompty.BatchItemResult{{Outcome: prompty.BatchItemSuccess}},
	}
	manager := WrapBatch(
		batchOperationFixture{result},
		WithTracerProvider(provider),
		WithProjection(func(_ context.Context, obs Observation) []attribute.KeyValue {
			// Explicit aggregate item observation, without IDs, content, or billing usage.
			resource, ok := obs.Resource.(*prompty.BatchResult)
			if !ok || resource == nil || obs.Operation != "batch.results" {
				return nil
			}
			return []attribute.KeyValue{attribute.Int("host.batch.items_observed", len(resource.Items))}
		}),
	)
	for range 2 {
		if _, err := manager.Results(context.Background(), *result.Handle, prompty.BatchResultsOptions{}); err != nil {
			panic(err)
		}
	}
	fmt.Println(len(exporter.GetSpans()))
	// Output: 2
}

func ExampleWrapCache() {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	size := int64(0)
	manager := WrapCache(
		cacheOperationFixture{
			&prompty.CacheResourceResult{
				Handle: &prompty.CacheResourceHandle{
					Identity: prompty.ProfileIdentity{Provider: "gemini"},
					State:    prompty.CacheResourceActive,
				},
				TokenCount: &size,
			},
		},
		WithTracerProvider(provider),
	)
	resource, err := manager.Create(context.Background(), prompty.CacheResourceCreateRequest{})
	if err != nil {
		panic(err)
	}
	fmt.Println(*resource.TokenCount, len(exporter.GetSpans())) // Known zero resource size, not generation tokens.
	// Output: 0 1
}
