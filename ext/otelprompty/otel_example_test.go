package otelprompty

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/prompty"
)

func ExampleWithTracing() {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	next := &invokerStub{generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
		return &prompty.Response{
			Model:   "fixture-observed",
			Outcome: prompty.OutcomeCompleted,
			Usage:   prompty.Usage{TotalTokens: 0, Known: []prompty.UsageCounter{prompty.UsageTotal}},
		}, nil
	}}
	traced := WithTracing(
		WithTracerProvider(provider),
		WithProjection(func(_ context.Context, _ Observation) []attribute.KeyValue {
			// The host selects a low-cardinality label. No prompt or error text is exported.
			return []attribute.KeyValue{attribute.String("host.workload", "example")}
		}),
	)(
		next,
	)
	_, err := traced.Execute(context.Background(), prompty.SimplePrompt("private prompt"))
	if err != nil {
		panic(err)
	}
	spans := exporter.GetSpans()
	fmt.Println(len(spans), spanAttributes(spans[0])["prompty.usage.total_tokens"])
	// Output: 1 0
}
