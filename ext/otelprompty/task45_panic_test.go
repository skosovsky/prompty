package otelprompty

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/prompty"
)

const privatePanic = "private caller panic payload"

type panicResponsePort struct{ prompty.RemoteResponseManager }

func (panicResponsePort) Create(context.Context, prompty.RemoteResponseRequest) (*prompty.RemoteResponseResult, error) {
	panic(privatePanic)
}

func (panicResponsePort) Get(
	context.Context,
	prompty.RemoteResponseHandle,
	...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	panic(privatePanic)
}

type panicBatchPort struct{ prompty.BatchManager }

func (panicBatchPort) Submit(context.Context, prompty.BatchSubmitRequest) (*prompty.BatchResult, error) {
	panic(privatePanic)
}

type panicCachePort struct{ prompty.CacheResourceManager }

func (panicCachePort) Create(
	context.Context,
	prompty.CacheResourceCreateRequest,
) (*prompty.CacheResourceResult, error) {
	panic(privatePanic)
}

func TestCallerPanicEndsLogicalSpanWithoutPayload(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"generation", "response.create", "response.get", "batch.submit", "cache.create"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			// Arrange: private caller-port failure with an isolated recorder.
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			option := WithTracerProvider(provider)
			ctx := context.Background()
			// Act / Assert: the original panic propagates after cleanup.
			require.PanicsWithValue(t, privatePanic, func() {
				switch operation {
				case "generation":
					port := &invokerStub{
						generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
							panic(privatePanic)
						},
					}
					_, _ = WithTracing(option)(port).Execute(ctx, prompty.SimplePrompt("known"))
				case "response.create":
					_, _ = WrapRemoteResponses(panicResponsePort{}, option).Create(ctx, prompty.RemoteResponseRequest{})
				case "response.get":
					_, _ = WrapRemoteResponses(panicResponsePort{}, option).Get(ctx, prompty.RemoteResponseHandle{})
				case "batch.submit":
					_, _ = WrapBatch(panicBatchPort{}, option).Submit(ctx, prompty.BatchSubmitRequest{})
				case "cache.create":
					_, _ = WrapCache(panicCachePort{}, option).Create(ctx, prompty.CacheResourceCreateRequest{})
				}
			})
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			require.Equal(t, codes.Error, spans[0].Status().Code)
			require.Equal(t, "caller_panic", spans[0].Status().Description)
			require.NotContains(t, fmt.Sprint(spans[0].Attributes(), spans[0].Events()), privatePanic)
		})
	}
}
