package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

// This wrapper intentionally exposes only Invoker, not descriptor/SDK ports.
type forwardingInvoker struct{ prompty.Invoker }

type helperOutput struct {
	Value int `json:"value"`
}

func TestHighLevelHelpersRetainCheckedPreflightThroughCustomWrapper(t *testing.T) {
	t.Parallel()
	for _, helper := range []struct {
		name string
		run  func(context.Context, prompty.Invoker, *prompty.PromptExecution) error
	}{
		{"structured sync", func(ctx context.Context, invoker prompty.Invoker, exec *prompty.PromptExecution) error {
			_, err := prompty.ExecuteWithStructuredOutput[helperOutput](ctx, invoker, exec)
			return err
		}},
		{"typed tools", func(ctx context.Context, invoker prompty.Invoker, exec *prompty.PromptExecution) error {
			_, err := prompty.ExecuteWithTypedTools(ctx, invoker, exec, prompty.NewTypedToolRegistry())
			return err
		}},
		{"tool validation", func(ctx context.Context, invoker prompty.Invoker, exec *prompty.PromptExecution) error {
			_, err := prompty.ExecuteWithToolValidation(ctx, invoker, exec, prompty.NewTypedToolRegistry())
			return err
		}},
		{"structured native", func(ctx context.Context, invoker prompty.Invoker, exec *prompty.PromptExecution) error {
			for _, err := range prompty.StreamStructuredOutput[helperOutput](ctx, invoker, exec, prompty.StreamNative) {
				return err
			}
			return nil
		}},
		{"structured buffered", func(ctx context.Context, invoker prompty.Invoker, exec *prompty.PromptExecution) error {
			for _, err := range prompty.StreamStructuredOutput[helperOutput](ctx, invoker, exec, prompty.StreamBuffered) {
				return err
			}
			return nil
		}},
	} {
		for _, runtime := range []bool{false, true} {
			for _, rejection := range []struct {
				name   string
				mutate func(*prompty.PromptExecution)
				want   error
			}{
				{"unknown capability", func(exec *prompty.PromptExecution) {
					exec.Requirements.Capabilities = []string{"host.extension.required"}
				}, ErrUnknownProfile},
				{"unsupported output MIME", func(exec *prompty.PromptExecution) {
					exec.Requirements.OutputMIME = []string{"audio/wav"}
				}, ErrUnsupportedCapability},
				{"unknown required cache", func(exec *prompty.PromptExecution) {
					exec.Messages[0].CachePolicy = &prompty.CachePolicy{
						Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired,
					}
				}, prompty.ErrCacheRequirement},
			} {
				t.Run(helper.name+"/"+rejection.name+runtimeFixtureSuffix(runtime), func(t *testing.T) {
					t.Parallel()
					// Arrange: both checked client variants sit behind a minimal custom port.
					fixture := &checkedFixture{descriptor: fixtureDescriptor()}
					invoker := NewClient(nativeCheckedFixture{fixture})
					if runtime {
						invoker = NewRuntimeClient(nativeCheckedFixture{fixture})
					}
					exec := prompty.SimplePrompt("fixture")
					rejection.mutate(exec)
					before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
					require.NoError(t, err)
					// Act.
					err = helper.run(context.Background(), forwardingInvoker{invoker}, exec)
					// Assert: helper preparation never clears guarantees or dispatches a fallback.
					require.ErrorIs(t, err, rejection.want)
					require.Zero(t, fixture.translations)
					require.Zero(t, fixture.calls)
					after, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
					require.NoError(t, err)
					require.Equal(t, before, after)
				})
			}
		}
	}
}

func runtimeFixtureSuffix(runtime bool) string {
	if runtime {
		return "/runtime"
	}
	return "/client"
}
