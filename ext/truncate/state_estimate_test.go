package truncate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func stateMessage(text string) prompty.ChatMessage {
	return prompty.ChatMessage{
		Role:    prompty.RoleAssistant,
		Content: []prompty.ContentPart{prompty.ReasoningPart{Text: text}},
		ProviderState: []prompty.PartState{
			{
				PartIndex: 0,
				Envelope: prompty.ProviderEnvelope{
					Format: prompty.ProviderStateFormat,
					Scope: prompty.ProfileIdentity{
						Provider: "fixture",
						Endpoint: "https://fixture.invalid",
						Model:    "model",
					},
					Codec:    "fixture.signed",
					Payload:  []byte(`{"signature":"unchanged"}`),
					Required: true,
				},
			},
		},
		Annotations: []prompty.PartAnnotation{
			{PartIndex: 0, Kind: "fixture", Value: prompty.JSONDocument(`{"source":1}`)},
		},
	}
}

func TestStateIsRetainedOrRemovedWholeWithConservativeDependencies(t *testing.T) {
	t.Parallel()
	for _, currentState := range []bool{false, true} {
		t.Run("state", func(t *testing.T) {
			t.Parallel()
			// Arrange: all opaque state-bearing messages form one conservative dependency.
			messages := []prompty.ChatMessage{
				prompty.NewUserMessage("old"),
				stateMessage("old signed"),
				prompty.NewUserMessage("current"),
			}
			if currentState {
				messages = append(messages, stateMessage("current signed"))
			}
			execution := prompty.NewExecution(messages)
			original := execution.Clone()
			// Act.
			result, err := Apply(t.Context(), execution, budget(1))
			// Assert.
			if currentState {
				require.ErrorIs(t, err, ErrBudgetUnsatisfied)
				require.Empty(t, result.Report.Removed)
				require.Equal(t, original, result.Execution)
			} else {
				require.NoError(t, err)
				require.Equal(t, []int{0, 1}, result.Report.Removed)
				require.Len(t, result.Execution.Messages, 1)
			}
			for _, message := range result.Execution.Messages {
				require.NoError(
					t,
					prompty.ValidatePartAssociations(message.Content, message.ProviderState, message.Annotations),
				)
			}
			require.Equal(t, original, execution)
		})
	}
}

func TestInvalidStateAssociationIsNotRepairedByDeletion(t *testing.T) {
	t.Parallel()
	// Arrange.
	invalid := stateMessage("old")
	invalid.ProviderState[0].PartIndex = 99
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{prompty.NewUserMessage("old"), invalid, prompty.NewUserMessage("current")},
	)
	// Act.
	result, err := Apply(t.Context(), execution, budget(1))
	// Assert.
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, result)
}

func TestExecutionCoverageIncludesFixedToolsAndSchemaCosts(t *testing.T) {
	t.Parallel()
	// Arrange: two messages fit, but declared execution estimate also counts fixed request material.
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{prompty.NewUserMessage("old"), prompty.NewUserMessage("current")},
	)
	execution.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object"}`)}}
	execution.ResponseFormat = &prompty.SchemaDefinition{
		Name:   "output",
		Schema: prompty.JSONDocument(`{"type":"object"}`),
	}
	messageOptions := budget(2)
	options := budget(2)
	options.Estimator = EstimatorFunc(func(_ context.Context, value *prompty.PromptExecution) (Estimate, error) {
		tokens := len(value.Messages) + 100*len(value.Tools)
		if value.ResponseFormat != nil {
			tokens += 100
		}
		return Estimate{Tokens: tokens, Known: true, Scope: ScopeExecution}, nil
	})
	// Act.
	messages, messageErr := Apply(t.Context(), execution, messageOptions)
	whole, wholeErr := Apply(t.Context(), execution, options)
	// Assert.
	require.NoError(t, messageErr)
	require.Equal(t, ScopeMessages, messages.Report.Before.Scope)
	require.ErrorIs(t, wholeErr, ErrBudgetUnsatisfied)
	require.Equal(t, ScopeExecution, whole.Report.After.Scope)
	require.Equal(t, 201, whole.Report.After.Tokens)
	require.Equal(t, []int{0}, whole.Report.Removed)
}

func TestEstimatorRecomputesNonAdditiveProjectionAndOwnsClone(t *testing.T) {
	t.Parallel()
	// Arrange: estimator is deliberately nonlinear and attempts to mutate its input.
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{
			prompty.NewUserMessage("old"),
			prompty.NewUserMessage("older"),
			prompty.NewUserMessage("current"),
		},
	)
	original := execution.Clone()
	calls := 0
	options := budget(4)
	options.Estimator = EstimatorFunc(func(_ context.Context, value *prompty.PromptExecution) (Estimate, error) {
		calls++
		count := len(value.Messages)
		value.Messages[0].Content[0] = prompty.TextPart{Text: "estimator mutation"}
		return Estimate{Tokens: count * count, Known: true, Scope: ScopeExecution}, nil
	})
	// Act.
	result, err := Apply(t.Context(), execution, options)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 9, result.Report.Before.Tokens)
	require.Equal(t, 4, result.Report.After.Tokens)
	require.Equal(t, []int{0}, result.Report.Removed)
	require.Equal(t, original, execution)
	require.Equal(t, "older", result.Execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestInvalidStrategyAndChangingCoverageFail(t *testing.T) {
	t.Parallel()
	for _, test := range []string{"invalid ID", "duplicate ID", "scope change"} {
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution := prompty.NewExecution(
				[]prompty.ChatMessage{prompty.NewUserMessage("old"), prompty.NewUserMessage("current")},
			)
			options := budget(1)
			switch test {
			case "invalid ID":
				options.Strategy = StrategyFunc(func(context.Context, []Group) ([]int, error) { return []int{99}, nil })
			case "duplicate ID":
				options.Strategy = StrategyFunc(
					func(context.Context, []Group) ([]int, error) { return []int{0, 0}, nil },
				)
			case "scope change":
				options.Estimator = EstimatorFunc(
					func(_ context.Context, value *prompty.PromptExecution) (Estimate, error) {
						scope := ScopeMessages
						if len(value.Messages) == 1 {
							scope = ScopeExecution
						}
						return Estimate{Tokens: len(value.Messages), Known: true, Scope: scope}, nil
					},
				)
			}
			// Act.
			_, err := Apply(t.Context(), execution, options)
			// Assert.
			if test == "scope change" {
				require.ErrorIs(t, err, ErrInvalidEstimate)
			} else {
				require.ErrorIs(t, err, ErrInvalidStrategy)
			}
		})
	}
}
