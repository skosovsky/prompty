package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func functionEvent(call string) string {
	return `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":` + call + `}]}}]}`
}

func TestNativePartialIDlessCallsAndExactProgress(t *testing.T) {
	t.Parallel()
	// Arrange: the provider starts one ID-less call and emits path/value fragments.
	fragment := `{"partialArgs":[{"jsonPath":"$.n","stringValue":"large-id"},{"jsonPath":"$.text","stringValue":"warm ","willContinue":true},{"jsonPath":"$.flag","boolValue":false},{"jsonPath":"$.nil","nullValue":"NULL_VALUE"}],"willContinue":true}`
	client, calls := geminiNativeFixture(t,
		functionEvent(`{"name":"lookup","willContinue":true}`), functionEvent(fragment),
		functionEvent(`{"partialArgs":[{"jsonPath":"$.text","stringValue":"weather"}],"willContinue":true}`),
		functionEvent(`{}`), functionEvent(`{"name":"lookup","partialArgs":[{"jsonPath":"$.n","stringValue":"two"}]}`),
		`{"candidates":[{"finishReason":"STOP"}]}`,
	)
	// Act: progress is provisional and visible before complete argument snapshots.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	progress := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if frame.Kind == prompty.StreamTerminal {
			continue
		}
		for _, annotation := range frame.Annotations {
			if annotation.Kind == "gemini.partial_function_call" {
				progress++
				if string(annotation.Value) == fragment {
					call := frame.Content[annotation.PartIndex].(prompty.ToolCallPart)
					require.Empty(t, call.ID)
					require.Empty(t, call.Args)
					require.Empty(t, call.ArgsChunk)
				}
			}
		}
	}
	status := stream.Status()
	// Assert: separate equal-name calls, exact numeric source and no invented IDs.
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 5, progress)
	require.Len(t, status.Result.Content, 2)
	first := status.Result.Content[0].(prompty.ToolCallPart)
	second := status.Result.Content[1].(prompty.ToolCallPart)
	require.Empty(t, first.ID)
	require.Empty(t, second.ID)
	require.Equal(
		t,
		geminiJSONValue([]byte(`{"n":"large-id","text":"warm weather","flag":false,"nil":null}`)),
		geminiJSONValue([]byte(first.Args)),
	)
	require.JSONEq(t, `{"n":"two"}`, second.Args)
	require.NotEqual(t, status.Identity.PartIDs[0], status.Identity.PartIDs[1])
	require.Len(t, status.Result.Annotations, 10)
}

func TestNativePartialProviderIDsInterleaveAndRetainOrder(t *testing.T) {
	t.Parallel()
	// Arrange: two equal-name calls, source IDs establish unambiguous interleaving.
	client, calls := geminiNativeFixture(t,
		functionEvent(`{"id":"a","name":"lookup","willContinue":true}`),
		functionEvent(`{"id":"b","name":"lookup","willContinue":true}`),
		functionEvent(`{"id":"b","partialArgs":[{"jsonPath":"$.n","stringValue":"two"}]}`),
		functionEvent(`{"id":"a","partialArgs":[{"jsonPath":"$.n","stringValue":"decimal"}]}`),
		`{"candidates":[{"finishReason":"STOP"}]}`,
	)
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert: first-delta position wins, not completion order or function name.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, []prompty.ContentPart{
		prompty.ToolCallPart{ID: "a", Name: "lookup", Args: `{"n":"decimal"}`},
		prompty.ToolCallPart{ID: "b", Name: "lookup", Args: `{"n":"two"}`},
	}, stream.Status().Result.Content)
}

func TestNativePartialInvalidContinuationRetainsProvisionalCall(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{
		`{"name":"lookup","willContinue":true}`,
		`{"partialArgs":[{"jsonPath":"$.open","numberValue":1}]}`,
		`{"args":{},"willContinue":true}`,
		`{"partialArgs":[{"jsonPath":"$.bad","stringValue":null}],"willContinue":true}`,
		`{"partialArgs":[{"jsonPath":"$.bad","numberValue":1}],"willContinue":null}`,
		`{}`,
	} {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			// Arrange: an open string cannot be finalized, overwritten or discarded.
			client, calls := geminiNativeFixture(
				t,
				functionEvent(
					`{"name":"lookup","partialArgs":[{"jsonPath":"$.open","stringValue":"partial","willContinue":true}],"willContinue":true}`,
				),
				functionEvent(bad),
				`{"candidates":[{"finishReason":"STOP"}]}`,
			)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert: retained provisional evidence, never a completed fake {} call.
			status := stream.Status()
			require.Equal(t, prompty.StreamFailed, status.State)
			require.Error(t, status.Err)
			require.Equal(t, int32(1), calls.Load())
			if bad == `{"args":{},"willContinue":true}` {
				require.Len(t, status.Result.Content, 2)
			} else {
				require.Len(t, status.Result.Content, 1)
			}
			require.Empty(t, status.Result.Content[0].(prompty.ToolCallPart).Args)
			if bad == `{"args":{},"willContinue":true}` {
				require.Len(t, status.Result.Annotations, 4)
			} else {
				require.Len(t, status.Result.Annotations, 2)
			}
		})
	}
}

func TestNativePartialEOFRetainsIdentityAndOriginalFragment(t *testing.T) {
	t.Parallel()
	// Arrange: accepted partial arguments, then EOF with no finish/closure.
	call := `{"name":"lookup","partialArgs":[{"jsonPath":"$.n","stringValue":"large-id"}],"willContinue":true}`
	client, _ := geminiNativeFixture(t, functionEvent(call))
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	for range stream.Events() {
	}
	// Assert: no success/materialized Args, raw value remains available for inspection.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.ErrorIs(t, status.Err, prompty.ErrStreamProtocol)
	require.Len(t, status.Result.Content, 1)
	require.Empty(t, status.Result.Content[0].(prompty.ToolCallPart).Args)
	require.Equal(t, call, string(status.Result.Annotations[0].Value))
}

func TestNativePartialOrdinaryNumbersStayProvisionalUntilComplete(t *testing.T) {
	t.Parallel()
	// Arrange: pointer-valued SDK numeric fragments; no original-wire precision promise.
	client, _ := geminiNativeFixture(
		t,
		functionEvent(`{"id":"a","name":"lookup","willContinue":true}`),
		functionEvent(
			`{"id":"a","partialArgs":[{"jsonPath":"$.zero","numberValue":0},{"jsonPath":"$.one","numberValue":1},{"jsonPath":"$.fraction","numberValue":0.25}],"willContinue":true}`,
		),
		functionEvent(`{"id":"a"}`),
		`{"candidates":[{"finishReason":"STOP"}]}`,
	)
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	provisional := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		for _, part := range frame.Content {
			if call, ok := part.(prompty.ToolCallPart); ok && call.Args == "" {
				provisional++
			}
		}
	}
	// Assert: numeric progress alone cannot dispatch a completed call.
	require.Positive(t, provisional)
	require.Equal(t, prompty.StreamCompleted, stream.Status().State)
	require.Equal(
		t,
		`{"fraction":0.25,"one":1,"zero":0}`,
		stream.Status().Result.Content[0].(prompty.ToolCallPart).Args,
	)
}
