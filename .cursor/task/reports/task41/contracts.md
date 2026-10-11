# Task41 — contract

База 39b6c4e; contract-first, clear break. Область: детерминированное remove-only сокращение canonical execution, без памяти/LLM/tokenizer/model catalog.

## API

- ext/truncate.Apply(ctx, execution, Options) возвращает Result{Execution, Report}, error. Старые message-only Strategy.Truncate/DropOldest/TruncateWithStrategy удаляются.
- Options задаёт positive Limit, Estimator, Protection и optional ordering Strategy. Disabled — единственный явный bypass, не эквивалент limit<=0/nil estimator. Disabled отражается отдельно от satisfied.
- Estimator принимает owned clone execution, возвращает Estimate{Tokens, Known, Scope}. Scope messages/execution объявляется явно. MessageEstimator использует TokenCounter и гарантирует только canonical message estimate. BYOT execution estimator охватывает tools/schema/media/framing/reserve согласно объявленному host контракту; existing adapter EstimateTokens implementations считаются message-only по текущему коду и не получают ложную execution coverage.
- Report содержит limit, before/after estimate, removed original indices, protected indices, satisfied/unsatisfied/disabled, machine-readable reason. Unknown/negative estimate и error не становятся zero. После каждого удаления выполняется свежая оценка всего кандидата: аддитивность counter не предполагается.
- Strategy выбирает порядок удаления готовых dependency groups через metadata/indices, без доступа к mutable execution и без rewrite/synthetic messages. Некорректный/повторный group ID — error. Отсутствие стратегии означает oldest-first.

## Защита и зависимости

- System/developer всегда protected. Default current turn начинается с последнего user marker и включает все последующие assistant/tool/system сообщения. При отсутствии user marker защищается вся история. Caller может явно указать CurrentTurnStart и дополнительные Protected message indices; текст не анализируется.
- Все сообщения старого user turn образуют один removal group, кроме protected system/developer; inserted system не разделяет call/result dependency.
- Call/result связываются по native ID и имени через всю историю, parallel results union групп. Незавершённые, orphan, duplicate/ambiguous и ID-less exchanges защищаются консервативно. Никаких fabricated IDs/name-only guessing. Host может расширить dependencies explicit index groups; отсутствие confirmed name contract не обходится heuristic.
- State-bearing сообщения объединяются консервативно: opaque provider dependencies не выводятся из payload. Все content/state/annotations удаляются только целыми messages; никакого редактирования signed content. Сохранённые local associations валидируются до/после, required continuation admission остаётся проверяемым до provider IO.
- Callbacks получают clone/metadata copies; исходный execution/messages/policy не меняются. Будущий downstream получает отдельный owned результат.

## Middleware

- contextx.WithTokenBudget(Options) использует тот же Apply для sync/native/buffered. По умолчанию unsatisfied/error → no upstream dispatch. Advisory разрешает только явно unsatisfied budget, не counter/policy/input errors; Disabled тоже явный.
- Report observer получает owned report. Никаких prompt/arguments в отчёте. Lazy/single-use stream contract сохраняется; error stream не вызывает upstream. Общий host capture/state/provider admission не подменяется.

## Проверки

AAA fixtures: current user-only oversize, protected-only oversize, system между call/result, parallel/incomplete/ID-less, state/annotations, custom ordering/protection, estimator errors/unknown/negative/overflow и non-additive estimates, tools/schema oversized execution estimate, mutation isolation, sync/native/buffered zero dispatch/stream laziness, explicit advisory/disable. Docs/migration/offline example, targeted + full lint/test/race/gen, затем два независимых аудита полного task41 scope.

Реализовано: существующий canonical MarshalExecution(StatePreserve) используется для structural validation перед transform; current boundary разрывает обычную turn grouping, но явные call/state dependencies всё равно сохраняются. Stream budget preparation выполняется при handle construction, provider dispatch остаётся lazy на настоящем upstream handle; никаких surrogate Events loops. Docs ext/truncate и middleware/contextx описывают ограничения scope и explicit advisory/disable.
