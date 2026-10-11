# Task41 — независимая приёмка полноты r2

**ACCEPT по полноте: 17/17 = 100%.** Partial/missing обязательных требований не найдено. Результат аудита snapshot, не математическая гарантия отсутствия всех ошибок; отдельный аудит ошибок необходим для paired acceptance.

## Объект и метод

Frozen `/tmp/prompty-task41-snapshot-02`, база `39b6c4e`. SHA256 manifest identity `73cf0f6ad2b1617a09255e15d3f2200845bf2138491e8946c4c87e2c78acfaac`. Все750 hashes проверены до/после в frozen, logs `/tmp/task41-integrity-r2-before.log`, `/tmp/task41-integrity-r2-after.log`. Frozen/source не менялись, own tests/probes в `/tmp/prompty-task41-audit-r2`. Peer текущего r2 не читался, implementation/delegation не выполнялись.

Полный scope: task41.md (8 contracts C1–C8, 8 acceptance A1–A8) + README mandatory closeout/rules M1 = denominator17. Частичные не засчитываются. Прочитаны полный spec/current contracts/closeout/checks-r2, source/doc contracts и diff immutable r1→r2. Изменённая реализация только boundary reset groups.go; остальные ext/middleware sources не изменились и проверены по полному исходному scope и repeated tests, а не только по закрытию C4/A6.

## Матрица

| ID | Обязательное требование | Code/test evidence | Статус |
|---|---|---|---|
| C1 | Сохранить system/developer/current turn, определить user и assistant/tool continuation/no-user boundary | groups.go removalGroups выбирает последний user и защищает suffix; system/developer всегда protected. Без user boundary0 защищает всю историю. Explicit CurrentTurnStart допускает caller boundary, текст не анализируется. TestCurrentTurnAndProtectedBudget + TestPolicyAndStrategyOwnOnlyRemovalMetadata. | complete |
| C2 | Tool call/result dependency целиком независимо от смежности/system, parallel/incomplete/no heuristic IDless | linkTurns пропускает system без split; linkExchanges объединяет все native ID/name across history. Parallel calls/results через message union. Unfinished/orphan/duplicates/name ambiguity/IDless защищаются консервативно. Tests ExchangesAcrossSystemAndParallelResults, IncompleteOrAmbiguousExchangeCannotBeHeuristicallyRemoved; independent cross-boundary probe. | complete |
| C3 | State/annotations/associations не dangling; signed content не rewrite; incompatible continuation диагностируется до IO | Remove-only целые messages. linkOpaqueState объединяет все state-bearing messages; protection распространяется на union. validateExecution использует canonical MarshalExecution StatePreserve до/после; malformed associations не repair deletion. StateIsRetainedOrRemovedWholeWithConservativeDependencies, InvalidStateAssociationIsNotRepairedByDeletion. Обычная checked scope/expiry/continuation admission остаётся у existing downstream; opaque payload не декодируется generic extension. | complete |
| C4 | Универсальная caller protection/strategy, без доменных agent/memory concepts, clear break | Protection CurrentTurnStart/Protected/Dependencies и copied Group metadata Strategy.Order; unknown/duplicate IDs reject. R2 boundary reset происходит до protected-role skip. Original r1 system-boundary probe проходит, system/developer author regressions и own middleware six-path probe проходят; tool/state dependencies всё ещё объединяются. | complete |
| C5 | Отчёт removed/before/after/budget/reason, unreachable default error/no dispatch; advisory explicit | Report original indices/scope/state/reason; BudgetError owned report, unmet protected/fixed/strategy-exhausted path. contextx.prepare допускает только explicit valid BudgetUnsatisfied advisory; input/counter/strategy/observer errors блокируются. Tests BudgetFailureDoesNotCreateUpstreamRequest, ExplicitAdvisoryAndDisable, observer failure. | complete |
| C6 | Явная область message vs execution, tools/schema/media/framing/reserve; BYOT estimator, unknown!=zero, no automatic tokenizer/limits | Estimate Known/Scope, MessageEstimator явно ScopeMessages; EstimatorFunc accepts full owned execution. validate negative/unknown/scopechange. Every reduction re-estimates entire candidate without additive assumption. ExecutionCoverageIncludesFixedToolsAndSchemaCosts, EstimatorRecomputesNonAdditiveProjectionAndOwnsClone. Docs отмечают provider EstimateTokens current message coverage и host-owned full projection. | complete |
| C7 | Same sync/native/buffered, lazy/single-use no upstream invalid/unreachable, clone input | contextx uses same prepare/Apply; failed stream ErrorStream without next handle; valid returns real next.ExecuteStream handle напрямую. No surrogate Events/goroutine/sync fallback. Tests PolicySameAcrossSyncNativeBufferedAndOwned, StreamBudgetPreservesLazySingleUseAndClose, CallerProtectionIsCapturedAndDoesNotBypassBudget; independent clone probe. | complete |
| C8 | Negative/zero budget, nil counter, counter errors/invalid values, explicit disable | Limit<=0/nil typed port reject; counter nil/negative/overflow/error; unknown/negative/scopeinvalid estimate reject. Disabled distinct unknown estimate state, still validates input. InvalidBudgetAndEstimatesFailExplicitly, DisabledBudgetIsDistinctAndOwned; independent typed-nil/unknown/negative probe. | complete |
| A1 | System + long latest user не отправляется system-only при малом бюджете | Current-turn group protects latest user; TestCurrentTurnAndProtectedBudget system plus latest cannot fit; middleware protected oversize case всех modes zero handles/dispatch. | complete |
| A2 | System/developer-only oversize unsatisfied + zero dispatch | Protected-only case TestCurrentTurnAndProtectedBudget; middleware generic unsatisfied gate rejects before next. Same execution Apply контракт используется всеми modes, protected set содержит обе roles. | complete |
| A3 | User→call→system→result no orphan после сокращения | TestExchangesAcrossSystemAndParallelResults удаляет indices1,2,4,5 вместе, остаются systems/current. Independent boundary3 result protected связывает предыдущие call/user без removals. | complete |
| A4 | Parallel/partial exchange correlation и current turn сохранены | Two lookup IDs a/b, reversed results объединены; unfinished, orphan, duplicate и IDless защищены целиком, default latest suffix всегда retained. Existing tests + independent cross-turn graph probe. | complete |
| A5 | State/annotations валидны; signed content unchanged | State-bearing required opaque payload и annotation retained/remove целыми; after ValidatePartAssociations; invalid old state reject до deletion. TestStateIsRetainedOrRemovedWholeWithConservativeDependencies и InvalidStateAssociationIsNotRepairedByDeletion. | complete |
| A6 | Caller policy identical sync/native/buffered, old input unchanged | Same prepare/Apply и policy snapshot, owned report/result; existing mutation tests. Own TestAuditProtectedRoleBoundaryAllModes проверяет explicit start2 на system/developer для sync/native/buffered: retained2, dispatch1, satisfied, original4 unchanged. | complete |
| A7 | Counter failure/unreachable no model; report coverage объяснена | BudgetFailureDoesNotCreateUpstreamRequest все modes, counter sentinel + Advisory true всё равно reject; Report scope Known состояния explicit. Typed nil/unknown/negative invalid no satisfied independently tested. | complete |
| A8 | Большие tools/schema message-only не full-request guarantee | ExecutionCoverageIncludesFixedToolsAndSchemaCosts: messages satisfied2, BYOT execution estimate201 after old removal unsatisfied; unchanged tools/schema сохранены. README explicitly describes framing/media/outputreserve/cache costs и host contract. | complete |
| M1 | README closeout, removed legacy, migration/examples/checks, универсальность/границы | Старые DropOldest/DropOldestStrategy/TruncateWithStrategy/Strategy.Truncate removed; usages/docs migrated. ext+middleware README/root link/offline example, source contracts/closeout/checks-r1. No LLM summary/retrieval/vector store/tokenizer download/persistent memory/implicit retry/agent DB. Author full allmodule lint/test/race/generated + independent runs, ниже. | complete |


## Независимые probes и evidence

Original own r1 file `/tmp/prompty-task41-audit-r1/ext/truncate/audit_task41_test.go` скопирован без изменения в audit-r2. Все три tests проходят:

1. TestAuditSystemBoundaryPartitionsOldHistory: assistant old0/old1, system policy2, current assistant3, explicit boundary2/limit2 теперь удаляет indices0,1 и возвращает satisfied. R1 blocker независимо закрыт.
2. TestAuditCrossTurnDependencyAndOwnedEstimate: native call до system и current result после boundary сохраняют весь connected set; estimator mutation message/tools schema bytes не меняет original или returned execution.
3. TestAuditUnknownNegativeAndTypedNil: unknown, negative, invalid scope и typed-nil estimator отвергаются без satisfied.

Новый own `/tmp/prompty-task41-audit-r2/middleware/contextx/audit_task41_boundary_test.go` TestAuditProtectedRoleBoundaryAllModes проверяет обе роли system/developer с boundary2 на sync/native/buffered: retained original indices2,3, Report satisfied, dispatch exactly1, input4 unchanged. Это доказывает применение caller policy всеми public middleware paths, а не только pure Apply.

Код groups.go сбрасывает обычный turn start **до** system/developer continue. Далее explicit dependencies, native exchange graph и opaque state graph объединяются независимо от этой границы; новая boundary не разрешает дробить linked tool/state. Protected suffix original indices сохраняется; system/developer неизменно protected.

## Проверки

- Independent broad race root/truncate/contextx/four adapters/generated: exit0 `/tmp/task41-completeness-independent-r2.log`.
- Independent targeted all truncate/contextx tests + unchanged original own probes: exit0 `/tmp/task41-completeness-targeted-r2.log`.
- Additional own middleware role/mode boundary probe: exit0 `/tmp/task41-completeness-middleware-probe-r2.log`.
- Own offline example `go run ./examples/context_budget`: exit0 `/tmp/task41-completeness-example-r2.log`, sync/native report messages16→6, removed1, retained2.
- Author full all12modules lint `/tmp/task41-lint-final-r2.log`, test/race/generated `/tmp/task41-test-final-r2.log`, targeted `/tmp/task41-boundary-r2draft.log`: completed exit0 по checks-r2.
- Frozen manifest identity совпадает и все750 file hashes unchanged before/after. Source и рабочий implementation не редактировались.

Broad command:

```sh
GOCACHE=/tmp/prompty-go-cache go test -race ./... ./ext/truncate/... ./middleware/contextx/... ./adapter/gemini/... ./adapter/ollama/... ./adapter/openai/... ./adapter/anthropic/... ./cmd/prompty-gen/testdata/gen_out
```

## Общий scope и границы

Current user/default no-user protection, indivisible parallel/incomplete/orphan/duplicate/IDless groups, opaque signed state/annotations and association structural validation before/after подтверждены source/tests. Signed content не редактируется. Existing checked downstream scope/expiry/capability/continuation validation не заменяется generic transform; payload не декодируется. Caller dependencies могут быть консервативнее и давать explicit unsatisfied.

Message estimator объявляет only messages; BYOT execution estimator получает owned full execution и объявляет coverage tools/schema/framing/media/reserve по host contract. Fixed tool/schema costs остаются после history removal. Every candidate recount, non-additive estimator, scope change/unknown/negative/overflow/counter error проверены. Known zero не смешивается с unknown. Advisory только valid unmet budget; disabled explicit и distinct. Ошибки/current oversize не вызывают upstream; реальный upstream handle lazy/single-use и close/cancellation preserved.

Input/policy/report/group copies и callback isolation подтверждены; runtime ports caller-owned immutable/concurrency-safe. Нет LLM summary/retrieval/vector store/persistent memory/implicit compaction/tokenizer/model catalog/retry/agent-domain concepts. Legacy APIs removed, consumers/docs/examples migrated, closeout описывает breaking behavior и limitations. Новых completeness findings не найдено.
