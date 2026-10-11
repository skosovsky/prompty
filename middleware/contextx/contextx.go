// Package contextx applies deterministic execution budgets before upstream IO.
package contextx

import (
	"context"
	"errors"
	"slices"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/ext/truncate"
)

// Options selects the budget and explicit host policy. Advisory admits only a
// valid unsatisfied budget; estimator/input/strategy/observer failures still stop.
type Options struct {
	Budget   truncate.Options
	Advisory bool
	Observe  func(context.Context, truncate.Report) error
}

// WithTokenBudget applies the same remove-only contract to sync/native/buffered.
// Upstream stream creation remains subject to Invoker's lazy stream contract.
func WithTokenBudget(options Options) prompty.Middleware {
	options.Budget.Protection = cloneProtection(options.Budget.Protection)
	return func(next prompty.Invoker) prompty.Invoker { return &tokenBudgetInvoker{next: next, options: options} }
}
func cloneProtection(policy truncate.Protection) truncate.Protection {
	policy.Protected = slices.Clone(policy.Protected)
	dependencies := make([][]int, len(policy.Dependencies))
	for i, group := range policy.Dependencies {
		dependencies[i] = slices.Clone(group)
	}
	policy.Dependencies = dependencies
	if policy.CurrentTurnStart != nil {
		value := *policy.CurrentTurnStart
		policy.CurrentTurnStart = &value
	}
	return policy
}

type tokenBudgetInvoker struct {
	next    prompty.Invoker
	options Options
}

func (t *tokenBudgetInvoker) prepare(
	ctx context.Context,
	execution *prompty.PromptExecution,
) (*prompty.PromptExecution, error) {
	result, err := truncate.Apply(ctx, execution, t.options.Budget)
	if result == nil {
		return nil, err
	}
	if t.options.Observe != nil {
		observerErr := t.options.Observe(ctx, truncate.CloneReport(result.Report))
		if observerErr != nil {
			return nil, errors.Join(err, observerErr)
		}
	}
	advisoryAllowed := t.options.Advisory && result.Report.State == truncate.BudgetUnsatisfied &&
		errors.Is(err, truncate.ErrBudgetUnsatisfied)
	if err != nil && !advisoryAllowed {
		return nil, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	return result.Execution, nil
}

func (t *tokenBudgetInvoker) Execute(
	ctx context.Context,
	execution *prompty.PromptExecution,
) (*prompty.Response, error) {
	prepared, err := t.prepare(ctx, execution)
	if err != nil {
		return nil, err
	}
	return t.next.Execute(ctx, prepared)
}

func (t *tokenBudgetInvoker) ExecuteStream(
	ctx context.Context,
	execution *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	prepared, err := t.prepare(ctx, execution)
	if err != nil {
		return prompty.ErrorStream(ctx, mode, err)
	}
	return t.next.ExecuteStream(ctx, prepared, mode)
}
