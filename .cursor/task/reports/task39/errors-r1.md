# Task39 — независимая приёмка ошибок, round 1

**Решение: не принято. Два открытых подтверждённых класса ошибок: P1 и P2.** В implementation не участвовала; отчёт второго аудитора текущего раунда не читала. Frozen/source не менялись. Пробники находятся только в отдельной копии.

## Проверяемая версия и evidence

- Frozen: `/tmp/prompty-task39-snapshot-01`; база реализации `b8bd56c`.
- SHA256 manifest: `96f119994290f79fb9d53ea67974fb0ede05e014bad8cd09a3065e11215c4e0e`.
- Identity совпала; все 681 файла manifest прошли проверку до анализа и повторно перед отчётом. Последний лог `/tmp/task39-errors-r1-integrity-report.log`.
- Прочитаны task39, общий README задач, публичный tool-execution contract, closeout и checks-r1. Проверены core scope/render/clone/wire/schema, checked adapter boundary, binding/registry/orchestration, text/structured/stream helper paths и связанные tests.
- Независимо выполнено в frozen: `GOCACHE=/tmp/prompty-go-cache go test ./... ./cmd/prompty-gen/gen ./cmd/prompty-gen/testdata/gen_out`, exit 0. Лог `/tmp/task39-errors-r1-baseline.log`.
- Проверены авторские full-module `make lint` и `make test` logs: `/tmp/task39-lint-final-01.log`, `/tmp/task39-test-final-01.log`: lint zero issues, race/module/generated-consumer suite passed. Эти full runs не выдаются за повторный независимый запуск.
- Executed adversarial probes: `/tmp/prompty-task39-errors-probes-r1/audit_task39_r1_test.go`; общий лог `/tmp/task39-errors-r1-probes.log`. Все четыре теста воспроизводимо падают на нужном нарушении контракта. Команда: `GOCACHE=/tmp/prompty-go-cache go test -run 'TestAudit(CustomInvokerCancellationStatus|StructuredHelperScopeBoundary|TextHelperRuntimeRequiredBoundary|StructuredStreamScopeBoundary)$' -v .` в temp-copy.
- Live provider calls не выполнялись.

## F1 — P1: прямые helper dispatch обходят сохранённый ToolScope с BYOT Invoker

**Места:** `structured_output.go:31–42` (`ExecuteWithStructuredOutput`); `stream_structured_output.go:50–67` (`StreamStructuredOutput`); `text_output.go:155–165` (`ExecuteAsText`).

Bound scope сохраняется в execution, но эти helper paths не вызывают `ValidateExecutionToolScope` перед фактическим model dispatch. Проверка checked adapter не закрывает поддержанный BYOT `Invoker`. Контракт task39 §3 прямо требует, чтобы direct helper и последующие изменения execution не обходили повторную проверку границы фактического dispatch. Это ограничение декларации host, а не IAM.

**Выполненные воспроизведения:**

1. `TestAuditStructuredHelperScopeBoundary`: `SimplePrompt` с tool `read`; `WithToolScope(Allowed:[read])`; затем изменение публичного `Tools[0].Name` на `delete_all`. Передача custom Invoker, считающего model calls и возвращающего JSON `{"n":1}`. `ExecuteWithStructuredOutput` вызывает модель **1 раз**, error **nil**.
2. `TestAuditStructuredStreamScopeBoundary`: тот же scoped execution и mutation, `StreamStructuredOutput` с `StreamNative`; custom stream Invoker вызывает модель **1 раз**, итог error **nil**.
3. `TestAuditTextHelperRuntimeRequiredBoundary`: tools `[read]`, scope `Allowed:[read], Required:[read]`, затем `Tools=nil`. `ExecuteAsText` вызывает модель **1 раз**, error **nil**. Отсутствие обычных `RequiredTools` позволяет пройти только text-format проверку; retained runtime Required игнорируется.

Во всех случаях ожидаются `ErrToolScope` и ноль model calls. Состояние typed handlers здесь не требуется: ошибка возникает уже на dispatch границе. Stateful helpers, делегирующие этим функциям, нуждаются в проверке вместе с ними; отдельного воспроизведения stateful path не заявляю.

**Необходимая коррекция:** повторять актуальную scope validation на всех library-owned dispatch paths независимо от реализации Invoker; добавить public BYOT boundary regressions для sync/stream/text/stateful входов, не перекладывая контракт core helper на checked adapter.

## F2 — P2: известный результат отменённого custom ToolInvoker помечается успешным

**Место:** `tool_orchestration.go:108–123`, построение `ToolResultPart` в строках 111–119 до проверки cancellation в строке 122.

Helper устанавливает `IsError` только по возвращённому `invokeErr`. Если BYOT `ToolInvoker` возвращает известный результат и nil error после отмены context во время invocation, helper сам обнаруживает cancellation и возвращает `context.Canceled`, но retained tool result остаётся `IsError=false`. Публичный contract утверждает: `IsError` обозначает invocation error, включая cancellation. Registry нормализует эту ситуацию внутри typed invocation, а custom port — нет; одинаковая semantics должна обеспечиваться core orchestration.

**Выполненное воспроизведение:** `TestAuditCustomInvokerCancellationStatus`. Model возвращает два валидных вызова `write`. Custom invoker отменяет полученный context при первом вызове и возвращает raw JSON `"written"`, nil error. Итог: handler calls **1**, следующий не стартует, `errors.Is(err, context.Canceled)` истинно, известный result сохранён, но `IsError=false`. Лог: `calls=1 IsError=false content=[{written <nil>}] error=context canceled`.

**Необходимая коррекция:** учитывать обнаруженную post-handler cancellation вместе с исходной invocation error до формирования retained результата; сохранить данные и обе причины через `errors.Is`, без retries/rollback.

## Остальной проверенный scope и предел вывода

Проверены distinction absent/empty scope, duplicate/empty definition names, required/forced existence, точное schema identity и normalized dialect, capabilities, scope cloning/getter isolation, wire v2/source numeric identity, restoration и untrusted-wire host responsibility. Scope остаётся declaration local tools и не расширяется local registry или hosted tools. Checked dispatch и typed-tool orchestration повторяют scope check; полный batch валидируется до handler effects и сохраняет original execution boundary при model mutation.

Проверены context signatures всех изменённых публичных tool ports, cooperative cancellation, no synthetic zero result до handler, joining handler/context errors, known response plus error retention/clone/no dispatch, registry definition deep clone и concurrent context tests. Context не сохраняется registry, cancellation не использует detached kill goroutines. Дополнительных подтверждённых классов ошибок в этом раунде не найдено.

Green existing checks не покрывают приведённые BYOT случаи и не отменяют findings. Решение round 1 блокировано F1/F2; необходима новая frozen версия и независимая повторная приёмка после исправлений. Это результат выполненных проверок, не абсолютная гарантия отсутствия других ошибок.
