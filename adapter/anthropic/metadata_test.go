package anthropic

import (
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestAnthropicDerivedUsageOverflowPreservesResponse(t *testing.T) {
	t.Parallel()
	maximum := int64(int(^uint(0) >> 1))
	for _, test := range []struct {
		name                       string
		input, read, write, output int64
		promptKnown                bool
	}{
		{"positive wrap input", maximum, maximum, 3, 1, false},
		{"total overflow", maximum, 0, 0, 1, true},
		{"negative input", -1, 0, 0, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: actual SDK JSON retains presence, including explicit cache zeros.
			var raw anthropic.Message
			body := fmt.Sprintf(
				`{"content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens":%d}}`,
				test.input,
				test.read,
				test.write,
				test.output,
			)
			require.NoError(t, raw.UnmarshalJSON([]byte(body)))
			// Act.
			response, err := New().ParseResponse(&raw)
			// Assert: no wrapped known total and no loss of successful provider content.
			require.ErrorIs(t, err, prompty.ErrUsage)
			require.NotNil(t, response)
			require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
			require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "answer"}}, response.Content)
			require.Zero(t, response.Usage.TotalTokens)
			require.NotContains(t, response.Usage.Known, prompty.UsageTotal)
			if test.promptKnown {
				require.Contains(t, response.Usage.Known, prompty.UsagePrompt)
				require.Equal(t, int(maximum), response.Usage.PromptTokens)
			} else {
				require.NotContains(t, response.Usage.Known, prompty.UsagePrompt)
				require.Zero(t, response.Usage.PromptTokens)
			}
			require.Equal(t, int(test.output), response.Usage.CompletionTokens)
			require.Equal(t, int(test.read), response.Usage.PromptTokensCached)
			require.Equal(t, int(test.write), response.Usage.PromptTokensCacheCreation)
			require.NoError(t, response.Usage.Validate())
		})
	}
}

func TestAnthropicOutcomeAndEmptyMetadata(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		reason  anthropic.StopReason
		outcome prompty.Outcome
	}{
		{anthropic.StopReasonEndTurn, prompty.OutcomeCompleted},
		{anthropic.StopReasonStopSequence, prompty.OutcomeCompleted},
		{anthropic.StopReasonRefusal, prompty.OutcomeRefusal},
		{anthropic.StopReasonMaxTokens, prompty.OutcomeIncomplete},
		{anthropic.StopReasonPauseTurn, prompty.OutcomeIncomplete},
		{"future_stop_reason", prompty.OutcomeIncomplete},
	} {
		t.Run(string(test.reason), func(t *testing.T) {
			t.Parallel()
			// Arrange: no text must not destroy a refusal/incomplete outcome or known zero.
			var raw anthropic.Message
			require.NoError(t, raw.UnmarshalJSON([]byte(`{"content":[],"stop_reason":"`+string(test.reason)+
				`","usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,`+
				`"cache_creation_input_tokens":0}}`)))
			// Act.
			response, err := New().ParseResponse(&raw)
			// Assert.
			require.NoError(t, err)
			require.Empty(t, response.Content)
			require.Equal(t, test.outcome, response.Outcome)
			require.Equal(t, prompty.UsageKnown, response.Usage.Availability())
			require.Zero(t, response.Usage.TotalTokens)
		})
	}
}

func TestAnthropicUsagePresenceAndCacheTotals(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		body      string
		known     []prompty.UsageCounter
		prompt    int
		total     int
		available prompty.UsageAvailability
	}{
		{"absent", `{}`, nil, 0, 0, prompty.UsageUnknown},
		{"null", `{"output_tokens":null}`, nil, 0, 0, prompty.UsageUnknown},
		{"partial", `{"input_tokens":10,"output_tokens":3}`, []prompty.UsageCounter{
			prompty.UsageCompletion,
		}, 0, 0, prompty.UsagePartial},
		{"cached", `{"input_tokens":10,"output_tokens":3,"cache_read_input_tokens":20,` +
			`"cache_creation_input_tokens":4}`, []prompty.UsageCounter{
			prompty.UsagePrompt, prompty.UsageCompletion, prompty.UsageTotal,
			prompty.UsageCacheRead, prompty.UsageCacheWrite,
		}, 34, 37, prompty.UsageKnown},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var raw anthropic.Usage
			require.NoError(t, raw.UnmarshalJSON([]byte(test.body)))
			// Act.
			usage, err := usageFromAnthropic(raw)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, test.known, usage.Known)
			require.Equal(t, test.prompt, usage.PromptTokens)
			require.Equal(t, test.total, usage.TotalTokens)
			require.Equal(t, test.available, usage.Availability())
			require.Equal(t, prompty.UsageCumulative, usage.Mode)
		})
	}
}

func TestAnthropicToolUseOutcome(t *testing.T) {
	t.Parallel()
	// Arrange.
	var raw anthropic.Message
	require.NoError(t, raw.UnmarshalJSON([]byte(
		`{"content":[{"type":"tool_use","id":"call","name":"lookup","input":{}}],"stop_reason":"tool_use"}`,
	)))
	// Act.
	response, err := New().ParseResponse(&raw)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeToolCalls, response.Outcome)
	require.Len(t, response.Content, 1)
}
