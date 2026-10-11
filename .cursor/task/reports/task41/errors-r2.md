# Task41 — independent errors audit r2

Decision: **ACCEPTED**. Открытых подтверждённых ошибок: **0**. Это результат полного просмотра и выполненных probes, не абсолютная гарантия отсутствия дефектов.

## Integrity and independence

Frozen `/tmp/prompty-task41-snapshot-02`, base `39b6c4e`, 750 файлов. SHA256 manifest `73cf0f6ad2b1617a09255e15d3f2200845bf2138491e8946c4c87e2c78acfaac` совпал. Все hashes before/after OK: `/tmp/task41-errors-r2-integrity.log`, `/tmp/task41-errors-r2-integrity-final.log`. Прочитаны task41, общий task README, contracts/closeout, current code, ext/middleware documentation и example. Peer r2 report не читала. Frozen и working implementation не менялись, делегирования/участия в реализации не было.

Чистые checks — `/tmp/prompty-task41-errors-checks-r2`. Собственные probes — `/tmp/prompty-task41-errors-probes-r2`.

## Independent checks

- Serial `make lint`, все 12 модулей: exit0, `/tmp/task41-errors-r2-lint.log`.
- `make test`, все модули/race/generated consumers: exit0, `/tmp/task41-errors-r2-test.log`.
- Собственные r1 probes перенесены без implementation edits, повторены с race: exit0, `/tmp/task41-errors-r2-probes.log`.
- Дополнительные exhaustive boundary и cross-boundary tool/state dependency probes с race: exit0, `/tmp/task41-errors-r2-boundaries.log`.
- Offline `go run ./examples/context_budget`: exit0, `/tmp/task41-errors-r2-example.log`.

GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache. Checks запущены независимо от авторских логов. Live provider calls нет.

## Previous confirmed finding

R1 F1 исправлен. Собственный TestAuditBoundaryAtProtectedMessage проверил system и developer при CurrentTurnStart=2: Before4→After2, Removed=[0,1], Protected=[2,3], BudgetSatisfied, error=nil. Reset выполняется до protected-role skip.

Дополнительно TestAuditExhaustivePlainBoundaries проверил 1280 сочетаний: 4-message histories из assistant/user/system/developer, каждое explicit boundary0..4. При бюджете ровно на обязательную protected часть достижимый бюджет всегда достигается, old nondependent prefix не становится случайно protected. Дополнительный TestAuditBoundaryStillPreservesExplicitDependencies подтвердил, что actual tool call/result или opaque state через system boundary по-прежнему связывает protected current suffix с older dependency, возвращая честный unsatisfied, а не orphan/edited state.

## Full task41 scope

**Protection/dependencies/order.** Default current turn starts at last user marker и включает subsequent assistant/tool/system; без marker вся история protected. System/developer always retained. Explicit boundary/Protected/Dependencies use validated original indices; protection propagates через graph. Reviewed whole old-turn grouping, native-ID/name call/result linking across system и cross-turn, parallel calls/results, incomplete/orphan/duplicate/changed-name/ID-less conservative protection. Собственные edge probes cross current suffix, completed full old exchange и changed-name/duplicate result pass. Group IDs/order deterministic по original indices; unknown/repeated IDs reject, selected subset reports strategy exhaustion, default oldest-first.

**State/correlation/structural fidelity.** Whole messages removed, signed content/state/annotations not edited. Opaque state-bearing messages conservatively union; no vendor payload guessing. Canonical MarshalExecution(StatePreserve) before/after checks local associations, projection shape, private ToolScope/required tools/forced schema/request policies. Tool/state cross-boundary own probes pass. Malformed older history cannot be erased for success; normal checked provider scope/expiry/capability/continuation admission remains downstream. Full adapter/task42 fidelity and history fixtures stay green.

**Estimator/report.** Reviewed positive limit/nil and typed-nil port guards, built-in negative/overflow summation checks, Known zero vs unknown, invalid/changing scope, error/cancellation. Every removal re-estimates whole candidate, no additive assumption. Own negative/unknown/bad scope/MaxInt unsatisfied/known-zero probes pass. Non-additive/mutating estimator and invalid strategy fixtures pass. Report contains original removed/protected indices, limit/before-after scope and values, satisfied/unsatisfied/disabled and reason; estimator errors do not become budget success. Existing message counters stay ScopeMessages, tools/schema/media/native framing/output reserve coverage only through explicit host execution estimator; no implicit tokenizer/model knowledge.

**Ownership/concurrency/callbacks.** Reviewed clone before validation/transform, estimator owned clone, strategy metadata/returned-order copies, middleware Protection snapshot и report observer clones. Собственный 32-concurrent Apply race probe with mutating estimator leaves original execution unchanged and outputs preserve original current content. Shared runtime ports remain caller-owned immutable/concurrency-safe per declared contract, not cloned vendor implementations. Caller execution/policy/report nested indices not exposed through mutable transform output.

**Middleware sync/native/buffered.** Same Apply/result/report/default blocking semantics in all modes. Own estimator cancellation, observer cancellation, and estimator ErrBudgetUnsatisfied with Advisory=true probes pass: zero upstream handles/dispatch; errors do not masquerade as valid unsatisfied advisory. Existing current-user/protected-only oversize, nil/zero/invalid-input, observer error, custom protection, lazy/single-use/Close and mutation isolation fixtures pass. Valid stream receives actual upstream lazy handle; preparation synchronous at construction explicitly documented, no synthetic iterator/goroutine/fallback. Impossible budgets and invalid history never call upstream. Explicit advisory admits only valid unsatisfied report; Disabled separately reports unknown estimate/disabled, malformed canonical input still rejected.

**Migration/library limits.** Reviewed docs/offline example and absence of legacy Go DropOldest/Strategy.Truncate/TruncateWithStrategy paths. Full callers/generated consumers compile. Example sync/native retains2messages, estimate16→6 and removed=[1]. No summary LLM/retrieval/memory engine/background process/model catalog/automatic retries. Universal BYOT estimator/protection/order ports remain library-sized; host owns actual tokenization/model limits/native request overhead, loop/storage/transport/policies.

## Findings and limits

Новых подтверждённых ошибок не обнаружено. R1 blocker не воспроизводится и закрыт по независимому evidence. Консервативная защита ID-less/opaque dependencies может сделать бюджет недостижимым; это explicit contract, а не name guessing. Message-only satisfied report не гарантирует physical provider context window; это явно отражено Scope и docs. Проверки здесь не удостоверяют accuracy arbitrary host estimator или live provider capabilities.

Полный независимый error audit r2 положительный; приёмка по отсутствию открытых confirmed findings выполнена.
