package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageInvalidMetadataRejectedByMarshalAndRestore(t *testing.T) {
	t.Parallel()
	for _, usage := range []Usage{
		{PromptTokens: -1}, {CompletionTokens: -1}, {TotalTokens: -1},
		{PromptTokensCached: -1}, {PromptTokensCacheCreation: -1}, {CompletionTokensReasoning: -1},
		{Known: []UsageCounter{UsageTotal, UsageTotal}}, {Known: []UsageCounter{"unknown"}}, {Mode: "unknown"},
	} {
		// Arrange.
		response := NewResponse([]ContentPart{TextPart{Text: "known content"}})
		response.Usage = usage
		wire := ResponseWire{Format: ResponseWireFormat, Outcome: OutcomeCompleted, Usage: usage}
		// Act.
		_, marshalErr := MarshalResponse(response, WirePolicy{State: StatePreserve})
		_, restoreErr := UnmarshalResponse(wire)
		// Assert.
		require.ErrorIs(t, marshalErr, ErrUsage)
		require.ErrorIs(t, restoreErr, ErrUsage)
	}
}

func TestUsageDeltaAndPartialCumulativeDoNotDoubleCountBreakdowns(t *testing.T) {
	t.Parallel()
	// Arrange: total is provider-defined, not prompt+cache+reasoning arithmetic.
	updates := []Usage{
		{PromptTokens: 10, TotalTokens: 20, PromptTokensCached: 4, CompletionTokensReasoning: 3,
			Known: []UsageCounter{UsagePrompt, UsageTotal, UsageCacheRead, UsageReasoning}},
		{TotalTokens: 2, PromptTokensCached: 1, CompletionTokensReasoning: 1,
			Known: []UsageCounter{UsageTotal, UsageCacheRead, UsageReasoning}, Mode: UsageDelta},
		{TotalTokens: 22, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative},
	}
	var acc Usage
	// Act.
	for _, update := range updates {
		merged, err := mergeUsage(acc, update)
		require.NoError(t, err)
		acc = merged
	}
	// Assert.
	require.Equal(t, 22, acc.TotalTokens)
	require.Equal(t, 10, acc.PromptTokens)
	require.Equal(t, 5, acc.PromptTokensCached)
	require.Equal(t, 4, acc.CompletionTokensReasoning)
	require.Equal(t, UsagePartial, acc.Availability())
	require.Equal(t, UsageCumulative, acc.Mode)
	// Arrange/Act: an unknown partial estimate is preserved, not made known.
	unknown := Usage{PromptTokens: 3}
	require.NoError(t, unknown.Validate())
	// Assert.
	require.Equal(t, UsageUnknown, unknown.Availability())
}

func TestUsageFailureKeepsPreviousStreamUpdateAtomic(t *testing.T) {
	t.Parallel()
	const maxInt = int(^uint(0) >> 1)
	for _, update := range []Usage{
		{TotalTokens: 1, Known: []UsageCounter{UsageTotal}, Mode: UsageDelta},
		{TotalTokens: 1, Known: []UsageCounter{UsageTotal, UsageTotal}, Mode: UsageDelta},
		{TotalTokens: -1, Known: []UsageCounter{UsageTotal}},
		{Mode: "unknown"},
	} {
		// Arrange.
		stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
			return func(yield func(*ResponseChunk, error) bool) {
				if !yield(
					&ResponseChunk{
						Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
						Kind:     StreamDelta,
						Content:  []ContentPart{TextPart{Text: "kept"}},
						Usage:    Usage{TotalTokens: maxInt, Known: []UsageCounter{UsageTotal}},
					},
					nil,
				) {
					return
				}
				if !yield(
					&ResponseChunk{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
						Kind:    StreamDelta,
						Content: []ContentPart{TextPart{Text: "rejected"}},
						Usage:   update,
					},
					nil,
				) {
					return
				}
				yield(&ResponseChunk{Kind: StreamFinish, Outcome: OutcomeCompleted}, nil)
			}
		})
		// Act.
		terminals := 0
		for event, err := range stream.Events() {
			if event.Kind == StreamTerminal {
				terminals++
				require.ErrorIs(t, err, ErrUsage)
				require.ErrorIs(t, err, ErrStreamProtocol)
			}
		}
		// Assert.
		require.Equal(t, 1, terminals)
		require.Equal(t, StreamFailed, stream.Status().State)
		require.Equal(t, maxInt, stream.Status().Result.Usage.TotalTokens)
		require.Equal(t, []ContentPart{TextPart{Text: "kept"}}, stream.Status().Result.Content)
	}
}

func TestStructuredHelperRejectsMalformedUsageWithoutRepairOrRetry(t *testing.T) {
	t.Parallel()
	// Arrange: valid JSON does not excuse a malformed canonical result.
	calls := 0
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		calls++
		response := NewResponse([]ContentPart{TextPart{Text: `{}`}})
		response.Usage = Usage{Known: []UsageCounter{UsageTotal, UsageTotal}}
		return response, nil
	}}
	// Act.
	result, err := ExecuteWithStructuredOutput[struct{}](context.Background(), invoker, SimplePrompt("fixture"))
	// Assert.
	require.ErrorIs(t, err, ErrUsage)
	require.Nil(t, result)
	require.Equal(t, 1, calls)
	var repair *ValidationError
	require.NotErrorAs(t, err, &repair)
}

func TestToolHelperRejectsMalformedUsageBeforeHandler(t *testing.T) {
	t.Parallel()
	// Arrange.
	invoked := 0
	tool, err := NewTypedTool("fn", func(_ context.Context, _ struct{}) (string, error) {
		invoked++
		return "fixture", nil
	})
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, tool))
	exec := SimplePrompt("fixture")
	exec.Tools = registry.Definitions()
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		response := NewResponse([]ContentPart{ToolCallPart{ID: "a", Name: "fn", Args: `{}`}})
		response.Usage = Usage{Mode: "invalid"}
		return response, nil
	}}
	// Act.
	_, err = ExecuteWithTypedTools(context.Background(), invoker, exec, registry)
	// Assert.
	require.ErrorIs(t, err, ErrUsage)
	require.Zero(t, invoked)
}
