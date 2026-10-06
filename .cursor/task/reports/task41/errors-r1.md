# Task41 — independent errors audit r1

Decision: **REJECTED**. Открыт **1 подтверждённый P2 finding**. Полные green checks не отменяют ошибку публичного caller policy.

## Identity and independence

Frozen `/tmp/prompty-task41-snapshot-01`, base `39b6c4e`, 747 файлов. SHA256 manifest `980e9e9b8cecc6f2adef9f489231f688b1b14bd71c8c14f2e1ac37d1439c812a` совпал. Все hashes проверены до/после: `/tmp/task41-errors-r1-integrity.log`, `/tmp/task41-errors-r1-integrity-final.log`. Прочитаны полный task41, общий README заданий, contracts, closeout, ext/middleware docs, implementation и применимые core projection/clone/scope contracts. Peer r1 report не читала; в implementation не участвовала, делегирования не было.

Frozen/working source не изменялись. Checks в отдельной `/tmp/prompty-task41-errors-checks-r1`, собственные probes — `/tmp/prompty-task41-errors-probes-r1`.

## F1 — P2: explicit CurrentTurnStart на system/developer не отделяет старую историю

Место: `ext/truncate/groups.go:54–64`, `linkTurns`. Защищённые роли пропускаются через continue до обработки i==boundary. Поэтому текущий protected suffix объединяется с ранее начавшимся обычным segment; protection propagates назад и делает старую историю неудаляемой.

Executed public reproduction: execution messages [assistant old0, assistant old1, system policy, assistant current], `Protection.CurrentTurnStart=2`, estimator messages=number of messages, Limit=2. Тот же кейс с developer. Оба должны удалить original indices [0,1], сохранить [2,3] и достичь бюджета. Фактически Apply возвращает ErrBudgetUnsatisfied, Before/After=4, Removed=[], Protected=[0,1,2,3]. Нет tool/state dependency, оправдывающей объединение.

Собственный `TestAuditBoundaryAtProtectedMessage` в отдельной копии, race: оба варианта failing. Лог `/tmp/task41-errors-r1-boundary.log`. Parent сообщил похожую root finding; я независимо выполнила свой reproduction без чтения peer report.

Нарушены явный index-based protected boundary contract и требование4/acceptance caller policy. Middleware использует тот же Apply, поэтому false unsatisfied по умолчанию блокирует достижимый sync/native/buffered request. Ошибка не является lost-data дефектом; это неверная группировка и отказ для валидной политики. Fix должен разрывать обычную turn group в точке caller boundary даже если само boundary message always-protected; explicit call/state dependencies всё ещё должны объединяться отдельно.

## Full scope audit and independent probes

- **Default protection and dependency graph.** Reviewed last-user suffix, no-user conservative protection, system/developer always protected, explicit Protected/Dependencies validation и propagation. Собственные probes: call/result через новый user boundary protects older call turn; completed exchange через inserted system при explicit empty suffix удаляется целиком; changed-name и duplicate-result exchange conservatively protected. Все эти probes pass. Parent-provided tests также проверяют parallel/system/incomplete/ID-less. Graph group ordering детерминирован через original indices; system insertions не разрывают tool dependency.
- **State/associations/correlation.** Whole message removal не переписывает signed content/state; opaque state-bearing groups консервативно связаны. Before/after canonical projection validates content/state/annotations, private ToolScope, required manifests и schema/request policies. Checked provider continuation/scope/expiry admission остаётся downstream; extension не декодирует opaque vendor state. Full suite включает task42 existing fidelity/restore fixtures и invalid older-history отказ.
- **Estimator.** Known/unknown/zero различны, отрицательные counts/overflow встроенного суммирования/invalid scope/changed coverage/error reject. Every candidate re-estimated без предположения additivity; Strategy metadata и returned order clone/validate. Собственные probes для unknown/negative/bad scope/MaxInt unsatisfied и known zero pass. Non-additive/mutation fixtures проходят. Coverage messages/execution явно объявляется; tools/schema/native media/framing/reserve не приписываются message counter. BYOT estimator correctness/tokenizer knowledge остаются host-owned, без implicit loading.
- **Ownership/concurrency.** Собственный race probe 32 concurrent Apply на одном input с mutating estimator: original input неизменен, каждый output owned и сохраняет original current content. Reviewed runtime-port boundaries, strategy metadata copies, report observer copy, middleware Protection snapshot; original context/history/policy не сохраняются в mutating shared state extension.
- **Middleware sync/native/buffered.** Собственные probes в каждом режиме: estimator cancels context then returns valid estimate; observer cancels context returns nil; estimator возвращает ErrBudgetUnsatisfied при Advisory=true. Все возвращают error с zero upstream handles/dispatch; estimator error не mistaken for valid unsatisfied advisory. `/tmp/task41-errors-r1-middleware-probes.log`. Existing tests проверяют unfit current/protected-only, nil/zero/invalid, lazy/single-use/Close, identical custom policy, observer failures, explicit advisory/disabled.
- **Cancellation/report/error.** Reviewed checks before/after callbacks и mutation isolation. Report stores original removed/protected indices, before/after declared scope, explicit disabled/satisfied/unsatisfied и retained-execution/strategy exhaustion reason. Partial candidate after failed estimate не выдаётся за successful estimate. Disabled remains distinct, malformed canonical execution не bypassed. No rollback/retry/LLM summary/IO scheduling introduced.
- **Clear break/library boundaries.** Legacy DropOldest/Strategy.Truncate signatures removed; callsites/docs/offline example migrated. Core remains provider-neutral/BYOT; port-owned transport/retries/loop/storage/models/tokenization preserved. Stream prep synchronously constructs transformation; real upstream lazy/single-use handle owns dispatch, no surrogate iterator/goroutine.

Собственные passing dependency/numeric/concurrency probes: `/tmp/task41-errors-r1-probes.log`. Boundary failing probe отдельный. Других подтверждённых ошибок в просмотренном полном scope/выполненных scenarios не обнаружено.

## Checks

- Independent clean-copy `make lint`, serial, all12modules: exit0, `/tmp/task41-errors-r1-lint.log`.
- Independent clean-copy `make test`, all modules/race/generated consumers: exit0, `/tmp/task41-errors-r1-test.log`.
- Offline `go run ./examples/context_budget`: exit0, `/tmp/task41-errors-r1-example.log`; sync/native retained messages2, estimate16→6, removed old index1.
- Cache env GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache.

Initial own probe source was accidentally created at root of temp-copy and caused package mismatch; moved to ext/truncate, rerun final passing probes. Frozen/working files untouched. Это не implementation failure и не baseline check.

Acceptance требует устранения F1 и нового независимого полного frozen audit. Отсутствие иных найденных ошибок не является абсолютной гарантией.
