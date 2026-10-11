package main_test

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/ollama/ollama/api"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	anthropicadapter "github.com/skosovsky/prompty/adapter/anthropic"
	geminiadapter "github.com/skosovsky/prompty/adapter/gemini"
	ollamaadapter "github.com/skosovsky/prompty/adapter/ollama"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

// This fixture binds an actually parsed provider result to the universal helper.
// It does not claim provider constrained-output or native-stream support.
type parsedOutcomeInvoker struct {
	response *prompty.Response
	calls    int
}

func (i *parsedOutcomeInvoker) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	i.calls++
	return i.response, nil
}

func (*parsedOutcomeInvoker) ExecuteStream(
	context.Context,
	*prompty.PromptExecution,
	prompty.StreamMode,
) *prompty.Stream {
	panic("sync outcome fixture must not stream")
}

func TestParsedProviderOutcomeCannotBecomeStructuredSuccess(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name    string
		parse   func() (*prompty.Response, error)
		outcome prompty.Outcome
	}{
		{"openai incomplete", func() (*prompty.Response, error) {
			var raw openai.ChatCompletion
			if err := raw.UnmarshalJSON([]byte(`{"choices":[{"index":0,"finish_reason":"length","message":{"content":"{}"}}],"usage":{"total_tokens":7}}`)); err != nil {
				return nil, err
			}
			return openaiadapter.New().ParseResponse(&raw)
		}, prompty.OutcomeIncomplete},
		{"openai refusal", func() (*prompty.Response, error) {
			var raw openai.ChatCompletion
			if err := raw.UnmarshalJSON([]byte(`{"choices":[{"index":0,"finish_reason":"stop","message":{"refusal":"{}"}}],"usage":{"total_tokens":7}}`)); err != nil {
				return nil, err
			}
			return openaiadapter.New().ParseResponse(&raw)
		}, prompty.OutcomeRefusal},
		{"anthropic incomplete", func() (*prompty.Response, error) {
			return parseAnthropicOutcomeFixture("max_tokens")
		}, prompty.OutcomeIncomplete},
		{"anthropic refusal", func() (*prompty.Response, error) {
			return parseAnthropicOutcomeFixture("refusal")
		}, prompty.OutcomeRefusal},
		{"gemini incomplete", func() (*prompty.Response, error) {
			return parseGeminiOutcomeFixture(genai.FinishReasonMaxTokens)
		}, prompty.OutcomeIncomplete},
		{"gemini refusal", func() (*prompty.Response, error) {
			return parseGeminiOutcomeFixture(genai.FinishReasonSafety)
		}, prompty.OutcomeRefusal},
		{"ollama incomplete", func() (*prompty.Response, error) {
			return ollamaadapter.New().ParseResponse(&api.ChatResponse{Done: true, DoneReason: "length",
				Message: api.Message{Content: "{}"}, PromptEvalCount: 5, EvalCount: 2})
		}, prompty.OutcomeIncomplete},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: every mapper provides valid JSON, but no completed outcome.
			response, parseErr := fixture.parse()
			require.NoError(t, parseErr)
			require.Equal(t, fixture.outcome, response.Outcome)
			require.Equal(t, 7, response.Usage.TotalTokens)
			original, wireErr := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, wireErr)
			invoker := &parsedOutcomeInvoker{response: response}
			// Act.
			value, err := prompty.ExecuteWithStructuredOutput[struct{}](
				context.Background(),
				invoker,
				prompty.SimplePrompt("fixture"),
			)
			// Assert: outcome failure is not a repairable JSON/schema failure.
			require.Nil(t, value)
			require.ErrorIs(t, err, prompty.ErrResponseOutcome)
			var outcomeErr *prompty.OutcomeError
			require.ErrorAs(t, err, &outcomeErr)
			require.Equal(t, fixture.outcome, outcomeErr.Outcome)
			require.Equal(t, response.FinishReason, outcomeErr.FinishReason)
			var validationErr *prompty.ValidationError
			require.NotErrorAs(t, err, &validationErr)
			require.Equal(t, 1, invoker.calls)
			after, afterErr := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, afterErr)
			require.Equal(t, original, after)
		})
	}
}

func parseAnthropicOutcomeFixture(reason string) (*prompty.Response, error) {
	var raw anthropic.Message
	if err := raw.UnmarshalJSON(
		[]byte(
			`{"content":[{"type":"text","text":"{}"}],"stop_reason":"` + reason + `","usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
		),
	); err != nil {
		return nil, err
	}
	return anthropicadapter.New().ParseResponse(&raw)
}

func parseGeminiOutcomeFixture(reason genai.FinishReason) (*prompty.Response, error) {
	return geminiadapter.New().ParseResponse(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{FinishReason: reason, Content: &genai.Content{Parts: []*genai.Part{{Text: "{}"}}}},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{TotalTokenCount: 7},
	})
}
