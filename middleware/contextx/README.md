# Budget middleware

`WithTokenBudget(Options)` applies the [truncate contract](../../ext/truncate/README.md) to the full canonical execution before upstream invocation. The default blocks errors and an unsatisfied estimate budget.

```go
invoker := contextx.WithTokenBudget(contextx.Options{
    Budget: truncate.Options{
        Limit: 4096,
        Estimator: truncate.MessageEstimator{
            Counter: &prompty.CharFallbackCounter{},
        },
    },
    Observe: func(ctx context.Context, report truncate.Report) error {
        // Store only report metadata under your own observability policy.
        return nil
    },
})(checkedProviderInvoker)
```

The same protection, ordering, estimation and observer semantics apply to sync/native/buffered requests. Input/policy/estimator failures and unreachable budgets create no upstream invocation/stream handle. Valid requests create the upstream lazy stream directly: no producer goroutine, surrogate stream or sync fallback. Budget preparation/observation is synchronous during handle creation; provider dispatch occurs only when that upstream handle is consumed. Close and single-use behavior remain owned by the actual upstream handle.

`Options.Advisory` explicitly permits a valid `BudgetUnsatisfied` result to proceed; its report still says unsatisfied. It never admits input/estimator/strategy/observer failures. `Budget.Disabled` is the explicit bypass and reports disabled. Observer errors stop dispatch, and reports contain only counts/indices, never prompt/tool/reasoning/state payload. Protection inputs are copied at middleware construction; estimator/strategy/observer runtime ports remain caller-owned.

Downstream receives an owned execution. Normal checked provider admission, original-schema validation and continuation scope/expiry are preserved; this middleware is not a provider profile, memory store or hard model context-window guarantee. Use an execution-wide host estimator for tools/schema/native framing/media/output reserve coverage. A message-only estimate stays explicitly message-only.

Breaking migration: replace `WithTokenBudget(limit, counter)` with `WithTokenBudget(contextx.Options{Budget: truncate.Options{...}})`. Handle `ErrBudgetUnsatisfied` or select advisory intentionally; nil counter/zero limit no longer mean a silent disable.

Offline example: `go run ./examples/context_budget` from the repository root.
