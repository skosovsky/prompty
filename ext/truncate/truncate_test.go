package truncate

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type fixedCounter struct {
	value int
	err   error
}

func (c fixedCounter) Count(string) (int, error)                     { return c.value, c.err }
func (c fixedCounter) CountMessage(prompty.ChatMessage) (int, error) { return c.value, c.err }
func budget(limit int) Options {
	return Options{Limit: limit, Estimator: MessageEstimator{Counter: fixedCounter{value: 1}}}
}

func TestCurrentTurnAndProtectedBudget(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		messages []prompty.ChatMessage
		limit    int
		removed  []int
		unmet    bool
	}{
		{"latest user remains", []prompty.ChatMessage{prompty.NewSystemMessage("sys"), prompty.NewUserMessage("old"), prompty.NewUserMessage("latest")}, 2, []int{1}, false},
		{"system plus latest cannot fit", []prompty.ChatMessage{prompty.NewSystemMessage("sys"), prompty.NewUserMessage("latest")}, 1, nil, true},
		{"protected only", []prompty.ChatMessage{prompty.NewSystemMessage("sys"), {Role: prompty.RoleDeveloper, Content: []prompty.ContentPart{prompty.TextPart{Text: "policy"}}}}, 1, nil, true},
		{"no user marker conservatively protected", []prompty.ChatMessage{{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "old"}}}, {Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "latest"}}}}, 1, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution := prompty.NewExecution(test.messages)
			original := execution.Clone()
			// Act.
			result, err := Apply(t.Context(), execution, budget(test.limit))
			// Assert.
			if test.unmet {
				require.ErrorIs(t, err, ErrBudgetUnsatisfied)
				require.Equal(t, BudgetUnsatisfied, result.Report.State)
			} else {
				require.NoError(t, err)
				require.Equal(t, BudgetSatisfied, result.Report.State)
			}
			require.Equal(t, test.removed, result.Report.Removed)
			require.Equal(t, original, execution)
			require.Equal(t, ScopeMessages, result.Report.After.Scope)
		})
	}
}

func TestExchangesAcrossSystemAndParallelResults(t *testing.T) {
	t.Parallel()
	// Arrange: old turn crosses a system policy with two parallel call/result pairs.
	execution := prompty.NewExecution([]prompty.ChatMessage{
		prompty.NewSystemMessage("sys"), prompty.NewUserMessage("old"),
		{
			Role: prompty.RoleAssistant,
			Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"},
				prompty.ToolCallPart{ID: "b", Name: "lookup", Args: "{}"},
			},
		},
		prompty.NewSystemMessage("policy"),
		{
			Role:    prompty.RoleTool,
			Content: []prompty.ContentPart{prompty.ToolResultPart{ToolCallID: "b", Name: "lookup"}},
		},
		{
			Role:    prompty.RoleTool,
			Content: []prompty.ContentPart{prompty.ToolResultPart{ToolCallID: "a", Name: "lookup"}},
		},
		prompty.NewUserMessage("latest"),
	})
	// Act.
	result, err := Apply(t.Context(), execution, budget(3))
	// Assert.
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 4, 5}, result.Report.Removed)
	require.Equal(t, []int{0, 3, 6}, result.Report.Protected)
	require.Len(t, result.Execution.Messages, 3)
}

func TestIncompleteOrAmbiguousExchangeCannotBeHeuristicallyRemoved(t *testing.T) {
	t.Parallel()
	for _, parts := range [][]prompty.ContentPart{
		{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}},
		{prompty.ToolCallPart{Name: "lookup", Args: "{}"}},
		{prompty.ToolResultPart{ToolCallID: "missing", Name: "lookup"}},
		{prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}, prompty.ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}},
	} {
		t.Run("exchange", func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution := prompty.NewExecution(
				[]prompty.ChatMessage{
					prompty.NewUserMessage("old"),
					{Role: prompty.RoleAssistant, Content: parts},
					prompty.NewUserMessage("latest"),
				},
			)
			// Act.
			result, err := Apply(t.Context(), execution, budget(1))
			// Assert.
			require.ErrorIs(t, err, ErrBudgetUnsatisfied)
			require.Empty(t, result.Report.Removed)
			require.Len(t, result.Execution.Messages, 3)
		})
	}
}

func TestPolicyAndStrategyOwnOnlyRemovalMetadata(t *testing.T) {
	t.Parallel()
	// Arrange: caller boundary partitions an otherwise marker-less history.
	boundary := 2
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "old"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "old2"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "current"}}},
		},
	)
	options := budget(1)
	options.Protection.CurrentTurnStart = &boundary
	options.Strategy = StrategyFunc(func(_ context.Context, groups []Group) ([]int, error) {
		groups[0].Messages[0] = 999
		return []int{0}, nil
	})
	// Act.
	result, err := Apply(t.Context(), execution, options)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, []int{0, 1}, result.Report.Removed)
	require.Equal(t, []int{2}, result.Report.Protected)
	require.Equal(t, "current", result.Execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestInvalidBudgetAndEstimatesFailExplicitly(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("counter failure")
	for _, test := range []struct {
		name     string
		options  Options
		expected error
	}{
		{"zero", budget(0), ErrInvalidBudget}, {"negative", budget(-1), ErrInvalidBudget},
		{"nil estimator", Options{Limit: 1}, ErrInvalidBudget},
		{"nil counter", Options{Limit: 1, Estimator: MessageEstimator{}}, ErrInvalidEstimate},
		{"negative counter", Options{Limit: 1, Estimator: MessageEstimator{Counter: fixedCounter{value: -1}}}, ErrInvalidEstimate},
		{"overflow", Options{Limit: 1, Estimator: MessageEstimator{Counter: fixedCounter{value: math.MaxInt}}}, ErrInvalidEstimate},
		{"counter error", Options{Limit: 1, Estimator: MessageEstimator{Counter: fixedCounter{err: sentinel}}}, sentinel},
		{"unknown", Options{Limit: 1, Estimator: EstimatorFunc(func(context.Context, *prompty.PromptExecution) (Estimate, error) {
			return Estimate{Scope: ScopeExecution}, nil
		})}, ErrInvalidEstimate},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution := prompty.NewExecution(
				[]prompty.ChatMessage{prompty.NewSystemMessage("sys"), prompty.NewUserMessage("latest")},
			)
			// Act.
			result, err := Apply(t.Context(), execution, test.options)
			// Assert.
			require.ErrorIs(t, err, test.expected)
			if result != nil {
				require.NotEqual(t, BudgetSatisfied, result.Report.State)
			}
		})
	}
}

func TestDisabledBudgetIsDistinctAndOwned(t *testing.T) {
	t.Parallel()
	// Arrange.
	execution := prompty.SimplePrompt("current")
	// Act.
	result, err := Apply(t.Context(), execution, Options{Disabled: true})
	result.Execution.Messages[0].Content[0] = prompty.TextPart{Text: "changed"}
	// Assert.
	require.NoError(t, err)
	require.Equal(t, BudgetDisabled, result.Report.State)
	require.False(t, result.Report.Before.Known)
	require.Equal(t, "current", execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestExplicitBoundaryAtSystemOrDeveloperStartsNewProtectedSuffix(t *testing.T) {
	t.Parallel()
	for _, role := range []prompty.Role{prompty.RoleSystem, prompty.RoleDeveloper} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			// Arrange: no user marker, but caller explicitly defines a protected suffix.
			boundary := 2
			execution := prompty.NewExecution([]prompty.ChatMessage{
				{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "old"}}},
				{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "old2"}}},
				{Role: role, Content: []prompty.ContentPart{prompty.TextPart{Text: "policy"}}},
				{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "current"}}},
			})
			options := budget(2)
			options.Protection.CurrentTurnStart = &boundary
			// Act.
			result, err := Apply(t.Context(), execution, options)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, []int{0, 1}, result.Report.Removed)
			require.Equal(t, []int{2, 3}, result.Report.Protected)
		})
	}
}
