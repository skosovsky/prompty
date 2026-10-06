package truncate

import (
	"context"
	"errors"
	"slices"

	"github.com/skosovsky/prompty"
)

// Apply removes whole dependency groups without editing signed content. Every
// estimate is recomputed from the retained execution; no additive assumption.
func Apply(ctx context.Context, execution *prompty.PromptExecution, options Options) (*Result, error) {
	if execution == nil {
		return nil, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	working := execution.Clone()
	if err := validateExecution(working); err != nil {
		return nil, err
	}
	result := &Result{
		Execution: working,
		Report: Report{
			Limit:     options.Limit,
			Before:    Estimate{Tokens: 0, Known: false, Scope: ""},
			After:     Estimate{Tokens: 0, Known: false, Scope: ""},
			Removed:   nil,
			Protected: nil,
			State:     "",
			Reason:    "",
		},
	}
	if options.Disabled {
		result.Report.State, result.Report.Reason = BudgetDisabled, "disabled"
		return result, nil
	}
	if options.Limit <= 0 || nilPort(options.Estimator) {
		result.Report.Reason = "invalid_budget"
		return result, ErrInvalidBudget
	}
	groups, protected, err := removalGroups(working.Messages, options.Protection)
	result.Report.Protected = protected
	if err != nil {
		return result, err
	}
	before, err := estimate(ctx, options.Estimator, working)
	result.Report.Before, result.Report.After = before, before
	if err != nil {
		result.Report.Reason = "estimate_unavailable"
		return result, err
	}
	if before.Tokens <= options.Limit {
		result.Report.State, result.Report.Reason = BudgetSatisfied, "within_budget"
		return result, nil
	}
	order, err := removalOrder(ctx, options.Strategy, groups)
	if err != nil {
		return result, err
	}
	return reduceGroups(ctx, result, groups, order, options)
}

func reduceGroups(ctx context.Context, result *Result, groups []Group, order []int, options Options) (*Result, error) {
	working := result.Execution
	before := result.Report.Before
	removed := make([]bool, len(working.Messages))
	for _, groupID := range order {
		for _, index := range groups[groupID].Messages {
			removed[index] = true
		}
		messages := retainedMessages(working.Messages, removed)
		candidate := working.WithMessages(messages)
		if validationErr := validateExecution(candidate); validationErr != nil {
			return result, validationErr
		}
		after, estimateErr := estimate(ctx, options.Estimator, candidate)
		if estimateErr != nil {
			result.Report.Reason = "estimate_unavailable"
			return result, estimateErr
		}
		if after.Scope != before.Scope {
			result.Report.Reason = "coverage_changed"
			return result, ErrInvalidEstimate
		}
		result.Execution, result.Report.After = candidate, after
		result.Report.Removed = removedIndices(removed)
		if after.Tokens <= options.Limit {
			result.Report.State, result.Report.Reason = BudgetSatisfied, "removed_history"
			return result, nil
		}
	}
	result.Report.State, result.Report.Reason = BudgetUnsatisfied, "retained_execution_exceeds_budget"
	if len(order) < len(groups) {
		result.Report.Reason = "strategy_exhausted"
	}
	return result, &BudgetError{Report: CloneReport(result.Report)}
}

func validateExecution(execution *prompty.PromptExecution) error {
	// Reuse the canonical projection contract; runtime ports are not persisted.
	if _, err := prompty.MarshalExecution(execution, prompty.WirePolicy{State: prompty.StatePreserve}); err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	for _, message := range execution.Messages {
		switch message.Role {
		case prompty.RoleSystem, prompty.RoleDeveloper, prompty.RoleUser, prompty.RoleAssistant, prompty.RoleTool:
		default:
			return ErrInvalidInput
		}
	}
	return nil
}

func removalOrder(ctx context.Context, strategy Strategy, groups []Group) ([]int, error) {
	order := make([]int, len(groups))
	for i := range order {
		order[i] = i
	}
	if nilPort(strategy) {
		return order, nil
	}
	copyGroups := make([]Group, len(groups))
	for i, group := range groups {
		copyGroups[i] = Group{ID: group.ID, Messages: slices.Clone(group.Messages)}
	}
	selected, err := strategy.Order(ctx, copyGroups)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool, len(selected))
	for _, id := range selected {
		if id < 0 || id >= len(groups) || seen[id] {
			return nil, ErrInvalidStrategy
		}
		seen[id] = true
	}
	return slices.Clone(selected), ctx.Err()
}

func retainedMessages(messages []prompty.ChatMessage, removed []bool) []prompty.ChatMessage {
	retained := make([]prompty.ChatMessage, 0, len(messages))
	for i, message := range messages {
		if !removed[i] {
			retained = append(retained, message)
		}
	}
	return retained
}
func removedIndices(removed []bool) []int {
	var indices []int
	for i, value := range removed {
		if value {
			indices = append(indices, i)
		}
	}
	return indices
}
