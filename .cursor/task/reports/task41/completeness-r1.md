# Task41 — независимая приёмка полноты r1

**REJECT: 15/17 = 88.24%.** C4 и A6 partial, не засчитаны. Подтверждён один дефект explicit caller boundary, затрагивающий оба требования. Это результат аудита конкретного snapshot, не математическая гарантия отсутствия ошибок. Финальная paired приёмка также требует независимого аудитора ошибок.

## Объект и метод

Frozen `/tmp/prompty-task41-snapshot-01`, база `39b6c4e`. Manifest identity SHA256 `980e9e9b8cecc6f2adef9f489231f688b1b14bd71c8c14f2e1ac37d1439c812a`. Все 747 hashes проверены до/после в frozen; logs `/tmp/task41-integrity-r1-before.log`, `/tmp/task41-integrity-r1-after.log`. Frozen/working implementation не менялись. Probes/test/example в отдельной копии `/tmp/prompty-task41-audit-r1`. Peer r1 report не читался; implementation/delegation не выполнялись.

Из полного task41.md перечислены 8 контрактных требований C1–C8 и 8 acceptance A1–A8. M1 включает общие обязательные README правила/closeout: clear break, migration/examples/checks и library boundaries. Denominator17 не ограничен чекбоксами; сопутствующие source contracts проверены в матрице. Прочитаны весь spec, README, contracts/closeout/checks, ext/truncate code/docs/tests, middleware/contextx code/docs/tests и offline example; сопоставлены existing clone/wire/state/execution contracts.

## Матрица

| ID | Обязательное требование | Code/test evidence | Статус |
|---|---|---|---|
| C1 | Сохранить system/developer/current turn, определить user и assistant/tool continuation/no-user boundary | groups.go removalGroups выбирает последний user и защищает suffix; system/developer всегда protected. Без user boundary0 защищает всю историю. Explicit CurrentTurnStart допускает caller boundary, текст не анализируется. TestCurrentTurnAndProtectedBudget + TestPolicyAndStrategyOwnOnlyRemovalMetadata. | complete |
| C2 | Tool call/result dependency целиком независимо от смежности/system, parallel/incomplete/no heuristic IDless | linkTurns пропускает system без split; linkExchanges объединяет все native ID/name across history. Parallel calls/results через message union. Unfinished/orphan/duplicates/name ambiguity/IDless защищаются консервативно. Tests ExchangesAcrossSystemAndParallelResults, IncompleteOrAmbiguousExchangeCannotBeHeuristicallyRemoved; independent cross-boundary probe. | complete |
| C3 | State/annotations/associations не dangling; signed content не rewrite; incompatible continuation диагностируется до IO | Remove-only целые messages. linkOpaqueState объединяет все state-bearing messages; protection распространяется на union. validateExecution использует canonical MarshalExecution StatePreserve до/после; malformed associations не repair deletion. StateIsRetainedOrRemovedWholeWithConservativeDependencies, InvalidStateAssociationIsNotRepairedByDeletion. Обычная checked scope/expiry/continuation admission остаётся у existing downstream; opaque payload не декодируется generic extension. | complete |
| C4 | Универсальная caller protection/strategy, без доменных agent/memory concepts, clear break допустим | Protection CurrentTurnStart/Protected/Dependencies и Strategy.Order над copied Group metadata; нельзя select protected или переписать execution. Unknown/duplicate IDs reject. PolicyAndStrategyOwnOnlyRemovalMetadata, InvalidStrategyAndChangingCoverageFail; middleware policy snapshot test. Но advertised CurrentTurnStart на system/developer не разделяет old/current группы: новый independent probe падает; см. blocker. | **partial** |
| C5 | Отчёт removed/before/after/budget/reason, unreachable default error/no dispatch; advisory explicit | Report original indices/scope/state/reason; BudgetError owned report, unmet protected/fixed/strategy-exhausted path. contextx.prepare допускает только explicit valid BudgetUnsatisfied advisory; input/counter/strategy/observer errors блокируются. Tests BudgetFailureDoesNotCreateUpstreamRequest, ExplicitAdvisoryAndDisable, observer failure. | complete |
| C6 | Явная область message vs execution, tools/schema/media/framing/reserve; BYOT estimator, unknown!=zero, no automatic tokenizer/limits | Estimate Known/Scope, MessageEstimator явно ScopeMessages; EstimatorFunc accepts full owned execution. validate negative/unknown/scopechange. Every reduction re-estimates entire candidate without additive assumption. ExecutionCoverageIncludesFixedToolsAndSchemaCosts, EstimatorRecomputesNonAdditiveProjectionAndOwnsClone. Docs отмечают provider EstimateTokens current message coverage и host-owned full projection. | complete |
| C7 | Same sync/native/buffered, lazy/single-use no upstream invalid/unreachable, clone input | contextx uses same prepare/Apply; failed stream ErrorStream without next handle; valid returns real next.ExecuteStream handle напрямую. No surrogate Events/goroutine/sync fallback. Tests PolicySameAcrossSyncNativeBufferedAndOwned, StreamBudgetPreservesLazySingleUseAndClose, CallerProtectionIsCapturedAndDoesNotBypassBudget; independent clone probe. | complete |
| C8 | Negative/zero budget, nil counter, counter errors/invalid values, explicit disable | Limit<=0/nil typed port reject; counter nil/negative/overflow/error; unknown/negative/scopeinvalid estimate reject. Disabled distinct unknown estimate state, still validates input. InvalidBudgetAndEstimatesFailExplicitly, DisabledBudgetIsDistinctAndOwned; independent typed-nil/unknown/negative probe. | complete |
| A1 | System + long latest user не отправляется system-only при малом бюджете | Current-turn group protects latest user; TestCurrentTurnAndProtectedBudget system plus latest cannot fit; middleware protected oversize case всех modes zero handles/dispatch. | complete |
| A2 | System/developer-only oversize unsatisfied + zero dispatch | Protected-only case TestCurrentTurnAndProtectedBudget; middleware generic unsatisfied gate rejects before next. Same execution Apply контракт используется всеми modes, protected set содержит обе roles. | complete |
| A3 | User→call→system→result no orphan после сокращения | TestExchangesAcrossSystemAndParallelResults удаляет indices1,2,4,5 вместе, остаются systems/current. Independent boundary3 result protected связывает предыдущие call/user без removals. | complete |
| A4 | Parallel/partial exchange correlation и current turn сохранены | Two lookup IDs a/b, reversed results объединены; unfinished, orphan, duplicate и IDless защищены целиком, default latest suffix всегда retained. Existing tests + independent cross-turn graph probe. | complete |
| A5 | State/annotations валидны; signed content unchanged | State-bearing required opaque payload и annotation retained/remove целыми; after ValidatePartAssociations; invalid old state reject до deletion. TestStateIsRetainedOrRemovedWholeWithConservativeDependencies и InvalidStateAssociationIsNotRepairedByDeletion. | complete |
| A6 | Caller policy identical sync/native/buffered, old input unchanged | Same prepare transform, observer own report, immutable captured protection; upstream mutations не меняют original. PolicySameAcrossSyncNativeBufferedAndOwned и CallerProtectionIsCapturedAndDoesNotBypassBudget + pure Apply strategy metadata mutation test. Общий prepare одинаков, но допустимый explicit boundary на system/developer фактически не применяется, поэтому эта caller policy не выполнена ни в одном mode. | **partial** |
| A7 | Counter failure/unreachable no model; report coverage объяснена | BudgetFailureDoesNotCreateUpstreamRequest все modes, counter sentinel + Advisory true всё равно reject; Report scope Known состояния explicit. Typed nil/unknown/negative invalid no satisfied independently tested. | complete |
| A8 | Большие tools/schema message-only не full-request guarantee | ExecutionCoverageIncludesFixedToolsAndSchemaCosts: messages satisfied2, BYOT execution estimate201 after old removal unsatisfied; unchanged tools/schema сохранены. README explicitly describes framing/media/outputreserve/cache costs и host contract. | complete |
| M1 | README closeout, removed legacy, migration/examples/checks, универсальность/границы | Старые DropOldest/DropOldestStrategy/TruncateWithStrategy/Strategy.Truncate removed; usages/docs migrated. ext+middleware README/root link/offline example, source contracts/closeout/checks-r1. No LLM summary/retrieval/vector store/tokenizer download/persistent memory/implicit retry/agent DB. Author full allmodule lint/test/race/generated + independent runs, ниже. | complete |

## Independent probes и проверки

Own `/tmp/prompty-task41-audit-r1/ext/truncate/audit_task41_test.go`:

- TestAuditCrossTurnDependencyAndOwnedEstimate: explicit boundary прямо перед tool result, matching call раньше system message; estimator пытается изменить message и ToolDefinition.Parameters bytes. Итог unsatisfied без deletion; call/result/current dependencies удержаны целиком; original и returned candidate unchanged.
- TestAuditUnknownNegativeAndTypedNil: Known=false, negative tokens, неизвестный scope и typed nil EstimatorFunc дают error, не satisfied.

`GOCACHE=/tmp/prompty-go-cache go test -race ./ext/truncate ./middleware/contextx -v`: exit0, `/tmp/task41-completeness-targeted-r1.log`, includes own probes и все existing adversarial tests.

Independent broad root/ext/middleware/four adapters/generated consumer run: exit0, `/tmp/task41-completeness-independent-r1.log`:

```sh
GOCACHE=/tmp/prompty-go-cache go test -race ./... ./ext/truncate/... ./middleware/contextx/... ./adapter/gemini/... ./adapter/ollama/... ./adapter/openai/... ./adapter/anthropic/... ./cmd/prompty-gen/testdata/gen_out
```

Own `go run ./examples/context_budget`: exit0; `/tmp/task41-completeness-example-r1.log`: message estimate16→6, removed original index1, retained2; sync/native same report. Author full checks all12modules lint/test/race/generated: `/tmp/task41-lint-final-r1b.log`, `/tmp/task41-test-final-r1b.log`; targeted `/tmp/task41-targeted-final-r1.log`, example `/tmp/task41-example-final-r1.log`: completed exit0 по checks-r1.

## Пределы результата

Budget satisfied означает declared estimate, не автоматически реальный provider context window. Execution coverage только по BYOT estimator contract; неизвестные costs не подменяются нулём. Runtime ports остаются caller-owned immutable/concurrency-safe; callback execution/message payload и report/group metadata owned copies. Opaque state/IDless dependency handling намеренно conservative; оно может сделать бюджет недостижимым и возвращает explicit unsatisfied, не нарушая protected guarantees.

Structural canonical/state/annotation input validation действует до transform. Provider-specific signature scope/expiry/capability admission сохраняется в existing checked downstream, generic transform не объявлен vendor validator. Source/oldinput не меняются. Host владеет memory/loop/retries/policy и может выбрать explicit advisory/disable; библиотека не делает implicit compaction/LLM IO. Проверен полный scope17; C4/A6 partial из-за одного подтверждённого blocker. R1 не принят.

## Подтверждённый blocker explicit boundary

После проверки основного scope root сообщил о возможном gap; он самостоятельно воспроизведён в собственной audit copy, без чтения peer report/working fix. `TestAuditSystemBoundaryPartitionsOldHistory` в `/tmp/prompty-task41-audit-r1/ext/truncate/audit_task41_test.go`: assistant old0, assistant old1, system policy2, assistant current3; CurrentTurnStart=2, Limit=2, exact message-count estimator.

Ожидается удаление original indices0,1, retained system/current2,3 и satisfied. Фактически error ErrBudgetUnsatisfied, Protected=[0,1,2,3], Removed=[]. `groups.go:56–57` пропускает system/developer через continue **до** boundary check на59: start остаётся old0 и текущий assistant3 объединяется с old0/old1. Не имеется call/state dependency, требующей этой дополнительной защиты. Это не advertised conservative opaque-state limitation, а неприменение допустимого caller boundary.

Команда `GOCACHE=/tmp/prompty-go-cache go test -race ./ext/truncate -run TestAuditSystemBoundaryPartitionsOldHistory -v` exit1; log `/tmp/task41-completeness-boundary-r1.log`. Общий middleware prepare наследует тот же дефект sync/native/buffered. Связанные C4 и A6 не засчитаны отдельно, denominator не сокращён; остальные guarantees safety/noIO остаются выполненными.

Первый broad/targeted успешный run предшествовал добавлению этого probe, поэтому его exit0 не трактуется как принятие r1. Нужны исправление и новый frozen round с повторной полной приёмкой. Final integrity frozen747 unchanged.
