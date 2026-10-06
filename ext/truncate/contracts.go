// Package truncate provides deterministic, remove-only execution budgeting.
package truncate

import (
	"context"
	"errors"

	"github.com/skosovsky/prompty"
)

var (
	ErrInvalidInput      = errors.New("truncate: invalid input")
	ErrInvalidBudget     = errors.New("truncate: invalid budget")
	ErrInvalidEstimate   = errors.New("truncate: invalid or unknown estimate")
	ErrInvalidStrategy   = errors.New("truncate: invalid removal strategy")
	ErrBudgetUnsatisfied = errors.New("truncate: budget unsatisfied")
)

// Scope describes estimator coverage, not an attestation of tokenizer precision.
type Scope string

const (
	ScopeMessages  Scope = "messages"
	ScopeExecution Scope = "execution"
)

// Estimate distinguishes unavailable knowledge from a known zero estimate.
type Estimate struct {
	Tokens int
	Known  bool
	Scope  Scope
}

// Estimator receives an owned execution clone. Execution coverage includes all
// request components/reserves under the caller's declared estimator contract.
type Estimator interface {
	Estimate(context.Context, *prompty.PromptExecution) (Estimate, error)
}
type EstimatorFunc func(context.Context, *prompty.PromptExecution) (Estimate, error)

func (f EstimatorFunc) Estimate(ctx context.Context, execution *prompty.PromptExecution) (Estimate, error) {
	return f(ctx, execution)
}

// Protection extends deterministic current-turn/system protection. An explicit
// CurrentTurnStart is an original index; len(messages) selects no current suffix.
// Dependencies connect original message indices without inspecting opaque state.
type Protection struct {
	CurrentTurnStart *int
	Protected        []int
	Dependencies     [][]int
}

// Group is one indivisible removable set of original message indices.
type Group struct {
	ID       int
	Messages []int
}

// Strategy orders removable groups, optionally selecting a subset. It cannot
// edit execution content or select any protected group. IDs cannot repeat.
type Strategy interface {
	Order(context.Context, []Group) ([]int, error)
}
type StrategyFunc func(context.Context, []Group) ([]int, error)

func (f StrategyFunc) Order(ctx context.Context, groups []Group) ([]int, error) {
	return f(ctx, groups)
}

type Options struct {
	Limit      int
	Estimator  Estimator
	Protection Protection
	Strategy   Strategy
	Disabled   bool
}

type BudgetState string

const (
	BudgetSatisfied   BudgetState = "satisfied"
	BudgetUnsatisfied BudgetState = "unsatisfied"
	BudgetDisabled    BudgetState = "disabled"
)

type Report struct {
	Limit     int
	Before    Estimate
	After     Estimate
	Removed   []int
	Protected []int
	State     BudgetState
	Reason    string
}

type Result struct {
	Execution *prompty.PromptExecution
	Report    Report
}

// BudgetError retains an owned diagnostic report. Partial transformed execution
// is returned separately for inspection or an explicitly selected advisory mode.
type BudgetError struct{ Report Report }

func (*BudgetError) Error() string { return "truncate: budget unsatisfied" }
func (*BudgetError) Unwrap() error { return ErrBudgetUnsatisfied }

// CloneReport isolates observers and stored error diagnostics from callers.
func CloneReport(report Report) Report {
	report.Removed = append([]int(nil), report.Removed...)
	report.Protected = append([]int(nil), report.Protected...)
	return report
}
