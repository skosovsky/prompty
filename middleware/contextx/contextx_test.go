package contextx

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/ext/truncate"
)

type budgetUpstream struct {
	handles  int
	dispatch int
	received *prompty.PromptExecution
	mutate   bool
}

func (s *budgetUpstream) Execute(_ context.Context, execution *prompty.PromptExecution) (*prompty.Response, error) {
	s.dispatch++
	s.received = execution
	if s.mutate {
		execution.Messages[0].Content[0] = prompty.TextPart{Text: "changed"}
	}
	return prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "ok"}}), nil
}

func (s *budgetUpstream) ExecuteStream(
	ctx context.Context,
	execution *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	s.handles++
	return prompty.NewStream(ctx, mode, func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
		return func(yield func(*prompty.ResponseChunk, error) bool) {
			s.dispatch++
			s.received = execution
			if s.mutate {
				execution.Messages[0].Content[0] = prompty.TextPart{Text: "changed"}
			}
			yield(
				&prompty.ResponseChunk{
					Kind:     prompty.StreamFinish,
					Outcome:  prompty.OutcomeCompleted,
					Identity: prompty.StreamIdentity{PartIDs: []string{"text"}},
					Content:  []prompty.ContentPart{prompty.TextPart{Text: "ok"}},
				},
				nil,
			)
		}
	})
}
func messageBudget(limit int) Options {
	return Options{
		Budget: truncate.Options{
			Limit: limit,
			Estimator: truncate.EstimatorFunc(
				func(_ context.Context, execution *prompty.PromptExecution) (truncate.Estimate, error) {
					return truncate.Estimate{
						Tokens: len(execution.Messages),
						Known:  true,
						Scope:  truncate.ScopeMessages,
					}, nil
				},
			),
		},
	}
}
func invoke(t *testing.T, invoker prompty.Invoker, execution *prompty.PromptExecution, mode prompty.StreamMode) error {
	t.Helper()
	if mode == "" {
		_, err := invoker.Execute(t.Context(), execution)
		return err
	}
	stream := invoker.ExecuteStream(t.Context(), execution, mode)
	for range stream.Events() {
	}
	return stream.Status().Err
}

func TestBudgetFailureDoesNotCreateUpstreamRequest(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("estimate failed")
	for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
		for _, test := range []struct {
			name      string
			options   Options
			execution *prompty.PromptExecution
			expected  error
		}{
			{"protected oversize", messageBudget(1), prompty.NewExecution([]prompty.ChatMessage{prompty.NewSystemMessage("sys"), prompty.NewUserMessage("latest")}), truncate.ErrBudgetUnsatisfied},
			{"nil input", messageBudget(1), nil, truncate.ErrInvalidInput},
			{"zero", messageBudget(0), prompty.SimplePrompt("current"), truncate.ErrInvalidBudget},
			{"nil estimator", Options{Budget: truncate.Options{Limit: 1}}, prompty.SimplePrompt("current"), truncate.ErrInvalidBudget},
			{"counter error", Options{Budget: truncate.Options{Limit: 1, Estimator: truncate.EstimatorFunc(func(context.Context, *prompty.PromptExecution) (truncate.Estimate, error) {
				return truncate.Estimate{}, sentinel
			})}, Advisory: true}, prompty.SimplePrompt("current"), sentinel},
		} {
			t.Run(test.name+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange.
				upstream := &budgetUpstream{}
				invoker := WithTokenBudget(test.options)(upstream)
				// Act.
				err := invoke(t, invoker, test.execution, mode)
				// Assert.
				require.ErrorIs(t, err, test.expected)
				require.Zero(t, upstream.handles)
				require.Zero(t, upstream.dispatch)
			})
		}
	}
}

func TestBudgetPolicySameAcrossSyncNativeBufferedAndOwned(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution := prompty.NewExecution(
				[]prompty.ChatMessage{
					prompty.NewSystemMessage("sys"),
					prompty.NewUserMessage("old"),
					prompty.NewUserMessage("latest"),
				},
			)
			original := execution.Clone()
			upstream := &budgetUpstream{mutate: true}
			var report truncate.Report
			options := messageBudget(2)
			options.Observe = func(_ context.Context, value truncate.Report) error { report = value; return nil }
			invoker := WithTokenBudget(options)(upstream)
			// Act.
			err := invoke(t, invoker, execution, mode)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, 1, upstream.dispatch)
			require.Len(t, upstream.received.Messages, 2)
			require.Equal(t, []int{1}, report.Removed)
			require.Equal(t, truncate.BudgetSatisfied, report.State)
			require.Equal(t, original, execution)
		})
	}
}

func TestStreamBudgetPreservesLazySingleUseAndClose(t *testing.T) {
	t.Parallel()
	// Arrange.
	upstream := &budgetUpstream{}
	invoker := WithTokenBudget(messageBudget(1))(upstream)
	// Act: creation has no provider dispatch, consumption dispatches once.
	stream := invoker.ExecuteStream(t.Context(), prompty.SimplePrompt("current"), prompty.StreamNative)
	require.Zero(t, upstream.dispatch)
	for range stream.Events() {
	}
	for _, err := range stream.Events() {
		require.ErrorIs(t, err, prompty.ErrStreamConsumed)
	}
	// Assert.
	require.Equal(t, 1, upstream.dispatch)
	closed := invoker.ExecuteStream(t.Context(), prompty.SimplePrompt("current"), prompty.StreamNative)
	require.NoError(t, closed.Close())
	for _, err := range closed.Events() {
		require.ErrorIs(t, err, context.Canceled)
	}
	require.Equal(t, 1, upstream.dispatch)
}

func TestExplicitAdvisoryAndDisable(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
		for _, disabled := range []bool{false, true} {
			t.Run(string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange.
				options := messageBudget(1)
				options.Advisory = true
				options.Budget.Disabled = disabled
				var report truncate.Report
				options.Observe = func(_ context.Context, value truncate.Report) error { report = value; return nil }
				upstream := &budgetUpstream{}
				invoker := WithTokenBudget(options)(upstream)
				execution := prompty.NewExecution(
					[]prompty.ChatMessage{prompty.NewSystemMessage("sys"), prompty.NewUserMessage("current")},
				)
				// Act.
				err := invoke(t, invoker, execution, mode)
				// Assert.
				require.NoError(t, err)
				require.Equal(t, 1, upstream.dispatch)
				if disabled {
					require.Equal(t, truncate.BudgetDisabled, report.State)
					require.False(t, report.Before.Known)
				} else {
					require.Equal(t, truncate.BudgetUnsatisfied, report.State)
				}
			})
		}
	}
}

func TestObserverFailureAndCallerPolicySnapshot(t *testing.T) {
	t.Parallel()
	// Arrange.
	sentinel := errors.New("observer failed")
	upstream := &budgetUpstream{}
	options := messageBudget(2)
	options.Observe = func(context.Context, truncate.Report) error { return sentinel }
	invoker := WithTokenBudget(options)(upstream)
	// Act.
	err := invoke(t, invoker, prompty.SimplePrompt("current"), "")
	// Assert.
	require.ErrorIs(t, err, sentinel)
	require.Zero(t, upstream.dispatch)
}

func TestCallerProtectionIsCapturedAndDoesNotBypassBudget(t *testing.T) {
	t.Parallel()
	// Arrange: policy inputs are mutated after middleware construction.
	boundary := 2
	options := messageBudget(2)
	options.Budget.Protection = truncate.Protection{
		CurrentTurnStart: &boundary,
		Protected:        []int{1},
		Dependencies:     [][]int{{0, 1}},
	}
	upstream := &budgetUpstream{}
	invoker := WithTokenBudget(options)(upstream)
	options.Budget.Protection.Protected[0] = 99
	options.Budget.Protection.Dependencies[0][0] = 99
	boundary = 99
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{
			prompty.NewSystemMessage("sys"),
			prompty.NewUserMessage("old"),
			prompty.NewUserMessage("current"),
		},
	)
	// Act.
	err := invoke(t, invoker, execution, prompty.StreamNative)
	// Assert: original captured protection, not the later invalid caller indices.
	require.ErrorIs(t, err, truncate.ErrBudgetUnsatisfied)
	require.Zero(t, upstream.handles)
	require.Zero(t, upstream.dispatch)
}

func TestInvalidHistoryIsNotSilentlyDeleted(t *testing.T) {
	t.Parallel()
	// Arrange: malformed older history would otherwise be removable.
	var invalid *prompty.TextPart
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{
			prompty.NewUserMessage("old"),
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{invalid}},
			prompty.NewUserMessage("current"),
		},
	)
	upstream := &budgetUpstream{}
	invoker := WithTokenBudget(messageBudget(1))(upstream)
	// Act.
	err := invoke(t, invoker, execution, "")
	// Assert.
	require.ErrorIs(t, err, truncate.ErrInvalidInput)
	require.Zero(t, upstream.dispatch)
}
