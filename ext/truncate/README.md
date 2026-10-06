# Execution budgeting

`Apply(ctx, execution, Options)` is a remove-only transformation. It returns an owned execution and a `Report`; it performs no model call, summarization, retrieval, tokenizer download or retry.

```go
result, err := truncate.Apply(ctx, execution, truncate.Options{
    Limit: 4096,
    Estimator: truncate.MessageEstimator{
        Counter: &prompty.CharFallbackCounter{},
    },
})
```

A satisfied result means the declared **estimate** fits the limit. `ScopeMessages` includes canonical messages only. It does not attest a complete provider context window: tools, schemas, native framing, media encoding, cached resource contents and output reserve may add costs. A host can implement `Estimator` / `EstimatorFunc` over the whole execution and declare `ScopeExecution` under its explicit coverage contract. The existing adapters' `EstimateTokens` methods currently count canonical messages; wrapping one does not make it a whole-request estimate. Exact tokenization/model limits and any request projection remain caller-owned.

`Estimate.Known` distinguishes unknown from known zero. Negative counts, integer overflow in the built-in message summation, changing coverage and estimator errors fail explicitly. Every removal re-estimates the entire candidate: estimators need not be additive. Callbacks receive an owned clone; runtime ports are still caller-owned and must be immutable or concurrency-safe.

## Protected history

System/developer messages remain. By default the current turn starts at the last user message and includes all following assistant/tool messages. Without a user marker, the whole history stays protected. `Protection.CurrentTurnStart` can explicitly select an original index (including `len(messages)` for no current suffix). `Protected` adds original indices and `Dependencies` joins caller-declared index sets.

Older turns are indivisible removal groups. Native call/result IDs and names connect messages across turns and intervening system messages, including parallel calls. Unresolved, orphan, duplicate/ambiguous and ID-less exchanges are conservatively protected. This extension does not infer name-only correlation from text or consume provider-specific profile knowledge. It never fabricates IDs.

Opaque state-bearing messages form one conservative dependency group. The extension does not decode provider payloads; it retains/removes whole messages with their state/annotations and never edits signed content or part indices. Canonical projection/state/annotation shape is checked before transformation, so invalid older history cannot be silently repaired by deletion. Normal checked adapter scope/expiry/capability/continuation admission still applies before provider dispatch. Hosts should extend dependencies when their own protocol has additional relationships.

`Strategy.Order` receives copied removable-group metadata and chooses an order/subset of group IDs. It cannot rewrite execution or select protected messages. Unknown/duplicate IDs fail. The default is oldest-first. Protection propagates through dependencies; `Report.Protected` describes the actual protected original indices.

## Report and failure

`Report` contains the limit, before/after estimate and coverage, removed/protected original indices, state and reason. `BudgetSatisfied` is distinct from `BudgetUnsatisfied` and `BudgetDisabled`. A valid but unmet budget returns the retained candidate plus `BudgetError` wrapping `ErrBudgetUnsatisfied`. If the strategy stops early, the reason is `strategy_exhausted`; otherwise the retained execution still exceeds the estimate limit. Neither case claims the physical model context window is known.

Positive limits and a nonnil estimator are required. Zero/negative limit and nil ports do not disable enforcement. Only `Options.Disabled` explicitly bypasses estimation/removal and reports disabled with unknown estimates. Nil execution and malformed canonical associations remain invalid even in disabled mode.

Breaking migration: remove `DropOldest`, `DropOldestStrategy`, message-based `Strategy.Truncate` and `TruncateWithStrategy` usage. Pass the full execution to `Apply`, inspect its report/error, and implement removal ordering with `Strategy.Order`. Do not ignore an unmet budget or emulate the old oversize success.

Offline runnable example: `go run ./examples/context_budget` from the repository root. Middleware semantics: [contextx](../../middleware/contextx/README.md).
