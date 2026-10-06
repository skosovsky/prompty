# Task39 — независимая приёмка полноты, round 1

**Не принято. Полнота: 94,44% (17 / 18 обязательных пунктов).**

Проверен только frozen `/tmp/prompty-task39-snapshot-01`, baseline `b8bd56c`. Identity manifest `/tmp/prompty-task39-snapshot-01.sha256`: `96f119994290f79fb9d53ea67974fb0ede05e014bad8cd09a3065e11215c4e0e`. Hash manifest совпадает, `shasum -a 256 -c` всех681 файлов — OK; `/tmp/task39-integrity-r1.log`. Аудитор не участвовал в реализации и не читал текущий errors-r1 другого аудитора.

Рабочий implementation/frozen не изменялись. Targeted race tests и public probe выполнены на отдельной копии `/tmp/prompty-task39-audit-r1`; probe module `/tmp/task39-probe-r1`.

## Знаменатель

Полный task39 содержит 8 пронумерованных контрактных требований, 9 acceptance criteria и обязательный closeout по общему README: всего18. Partial не засчитывается. Дополнительно проверены BYOT, clear break, existing-contract reuse и host-owned orchestration boundaries; scope не сокращён.

## Матрица requirement → evidence → status

| ID | Обязательный пункт | Code / test evidence | Status |
| --- | --- | --- | --- |
| C1 | Absent scope != present empty Allowed; unscoped direct exec.Tools remains boundary | Private `PromptExecution.toolScope`, cloned `ToolScope()` accessor и `WithToolScope`; nil/present distinction in `ValidateExecutionToolScope`; explicit empty deny in `TestExecutionScopeRejectsInvalidDeclarations`, direct unscoped helper positive tests | complete |
| C2 | All final definitions satisfy Allowed name/schema/capabilities after overrides; required/forced present; duplicate/empty rejected | `executionToolDefinitions`, `ValidateExecutionToolScope`, `ValidateToolScope`, exact schema comparator/normalized dialect/capability inclusion; `RenderPlan.Execute` applies scope after materialization/overrides; declaration/recheck/required+forced tests | complete |
| C3 | Bound scope survives clone/direct helper/wire/changes and protects actual dispatch/handler; no old-copy authorization | Snapshot survives clone/message/history/wire, changed definitions checked at materialization/adapter/helper/serialization. **Однако alias Parameters передаётся caller SchemaValidator после scope check; callback mutation dispatches altered definition outside Allowed и no-tool response returns success.** Confirmed F1 ниже | partial |
| C4 | Declaration scope is not IAM; hosted tools separated from local handlers | Scope applies `exec.Tools` local definitions and response `ToolCallPart`, registry only resolves local names; hosted provider-tool protocol does not acquire permissions through registry. No identity/backend/approval/policy loading added; contracts/closeout document host ownership | complete |
| C5 | Context through handler/callback/TypedTool/Invoker/registry/helper; direct signature break, no Background compatibility | `ToolHandler.Handle(ctx,args)`, `NewTypedTool` callback func(ctx,args), `TypedTool.Invoke`, `ToolInvoker.InvokeTool`, registry private/public calls and helper pass context; all consumers compile; no Legacy aliases or Background adapters | complete |
| C6 | Entire batch schema/scope/correlation validated before first effect; check context before model/each handler/after handler; no next handler after cancel | `ExecuteWithTypedTools` preflight, `validateResponseToolScope`, `validateToolBatch`, `invokeToolBatch`; existing invalid-batch/correlation tests + canceled-before-model and two-call canceled-handler tests. Scope mutation boundary defect is accounted once under C3 | complete |
| C7 | Cooperative cancellation, no detached goroutine; known result/cause retained; errors.Is preserves context and handler cause; no rollback claim | Synchronous `TypedTool.invoke` checks context then handler then errors.Join(handlerErr,ctx.Err); registry preserves started/result and encode cause; batch appends known result then returns joined cancellation; `TestTypedToolCancellationKeepsCompletedResultAndErrors`, HTTP fixture, direct-result/no-synthetic-result tests | complete |
| C8 | Partial results and response+error preserved; failed model answer executes no tools; no automatic retry/compensation | `ToolDispatchError.Response` clone/Unwrap, ToolCallError response copy, returned pre-acceptance execution; `TestModelPartialResponseErrorDoesNotInvokeTools`; refusal/incomplete/metadata errors retained and zero handlers | complete |
| A1 | Allowed read vs template delete_all with empty RequiredTools denied before model/handler | `TestMaterializationRejectsDisallowedDefinitionWithoutRequiredTools` fails materialization; checked adapter preflight rejects mutated bound definition with zero translations/SDK calls; helper recheck scenario verifies zero model calls on changed definitions | complete |
| A2 | Explicit empty, missing/duplicate, forced outside scope, changed schema reject before effects | declaration validator rejects allowed/required duplicates/empty names, missing required/forced definition and definitions outside Allowed; exact large const schema test + `TestExecutionScopeRejectsInvalidDeclarations`, old/tool contract tests | complete |
| A3 | required+forced+allowed positive; direct caller-built tools works without recipe/registry requirement | `TestRequiredForcedAllowedToolExecutes` with broader registry; `TestExecuteWithTypedTools_InvokesHandler`, direct typed Invoke/registry cases; API imposes no recipe container | complete |
| A4 | Scope cloned/preserved or explicit rebind-required through wire/checkpoint and overrides | clone.go both clone paths, private snapshot/accessor, wire ToolScope field; `ExecutionWireFormat=prompty.execution.v2`; executable JSON Schema requires scope arrays; old marker rejected; recipe/runtime scope explicitly rebind through BindRuntime; recheck/isolation wire tests | complete |
| A5 | Invalid call in batch blocks all; larger registry does not expand execution permission | `validateResponseToolScope` before `validateToolBatch`; calls validated against original exec schema then registry before invocation; `TestToolExecutionScopeFailsBeforeAnyHandler`, validation orchestration/correlation tests, adversarial model mutation test | complete |
| A6 | HTTP deadline/cancel delivered, next handler not started, last not cancellation-success | HTTP fixture compares actual handler context deadline, server sees cancellation; canceled last/direct handler returns result+context error; two-call batch only first starts; no goroutine detachment in library | complete |
| A7 | Completed effect/result accessible after cancel/error, no automatic second invoke | completed result text retained in tool message, IsError on typed invocation error, both context and warning errors.Is retained; cancellation test call count1; no implicit repeat/compensation path | complete |
| A8 | Concurrent registry invokes own contexts; race passes | `TestConcurrentRegistryCallsUseOwnContexts`, registry doesn't store context; registration completed before invokes; personally run root/shared adapter targeted `-race` tests exit0 | complete |
| A9 | Constructors/examples/tests updated to context API; old API removed | Go signatures/calls reviewed; updated root/provider test consumers compile; final make test covers all modules/examples/generated consumers; no legacy/no-context wrappers found | complete |
| G1 | Closeout contracts/removed legacy/migration/checks/limitations, breaking | contracts.md + closeout.md + checks-r1.md describe private scope/wirev2/context/result/error contracts, removed signatures/fallbacks, migration/reconciliation, required checks and host boundaries | complete |

## F1 — successful old scope check authorizes changed actual model dispatch

**Места:** `tool_orchestration.go:ExecuteWithTypedTools` validates scope before `compileToolSchemas`, then after compilation checks only context before `invoker.Execute`; `compileToolSchemas` passes `definition.Parameters` raw slice directly to caller compiler; no-tool-call response returns success without scope recheck. Scope itself is correctly private and cloned; the mutable definition aliases compiler input.

Public probe `/tmp/task39-probe-r1/probe_test.go`:

1. Direct caller execution has tool `read`, parameters `{"type":"object"}`, explicit Allowed same exact schema. `WithToolScope` succeeds.
2. Set custom caller `SchemaValidator`. Its CompileSchema mutates the supplied bytes in place from `object` to `string` (same byte length), then delegates to existing JSONSchemaValidator to compile the supplied changed bytes. No false validator is returned; returned engine enforces that schema.
3. Invoke ExecuteWithTypedTools with model fixture returning completed text response and no tool calls.
4. Actual observed result: **model dispatch count1, returned error=nil, returned tool schema={"type":"string"}, ValidateExecutionToolScope(returned execution)=ErrToolScope**. Original caller execution remains cloned, but actual dispatched helper execution violated the retained declaration.

This contradicts C3: validation of the previous snapshot cannot authorize a changed definition at the dispatch boundary. With an arbitrary public Invoker the helper itself must retain/check the declared boundary; checked adapters' additional preflight does not fix the direct helper contract. Existing adversarial-model test covers mutation of a separate model input clone, but not compiler callback aliasing before dispatch.

Reproduction (reads only separate snapshot copy):

```sh
GOWORK=/tmp/task39-probe-r1/go.work GOSUMDB=off GOCACHE=/tmp/prompty-go-cache go test -v /tmp/task39-probe-r1/probe_test.go
```

`TestCompilerMutationScope` fails: `bound execution mutated during preflight dispatched outside declared scope`. Consumer compiles and execution succeeds; failure is semantic assertion. Required fix: prevent mutation of retained definitions via callback input and ensure actual dispatched/returned boundary cannot differ after preflight. Add public regression asserting zero out-of-scope model/handler dispatch and valid retained scope with a mutating compiler callback; repeat paired audit on new frozen snapshot.

## Проверки и общие границы

Лично запускались root/shared adapter targeted tests с `-race`: materialization, scope clone/wire/recheck, declaration rejection, required+forced positive, typed/direct cancellation, HTTP context, concurrent registry contexts, response+error retention, invalid batch, adversarial model mutation. Exit0. Independent public compiler-mutation probe failed по подтверждённому F1. Integrity всех681 файлов OK.

Просмотрены final lint/test evidence `/tmp/task39-lint-final-01.log`, `/tmp/task39-test-final-01.log`; final all-module lint reports zero issues, race/tests/generated consumer green; checks-r1 содержит exit0 и git diff --check. Green tests не покрывают найденный callback boundary case.

BYOT и clear break сохранены: small ports, caller-owned types/schema/domain policies, context signatures изменены напрямую, wirev2 явно отвергает old marker. Existing execution/schema/state/usage contracts reused. Scope declaration не обещает IAM; hosted tool protocols не получают local-handler dispatch через registry. Не добавлены agent loop, scheduler, goroutine timeout wrapper, persistent run state, automatic retry/compensation, approvals/backend, implicit IO или provider SDK features. HTTP fixture локальная, live-provider conformance не утверждается.

**Решение:** 17 complete, 1 partial (C3), 0 missing. Task39 не принята по полноте; коммит этим отчётом не разрешён. Нужен новый frozen round после исправления F1 и обязательных проверок.
