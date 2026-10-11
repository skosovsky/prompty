# task39 — Строгий tool scope и отменяемое one-shot исполнение

**Статус:** accepted — round 2: полнота100%(18/18), open confirmed errors0, full lint/test/race green.
**Приоритет:** P1. **База:** `70fc301`, 6 октября 2026.
**Зависимость:** точное сравнение schema documents из [task38](task38.md).
**Общие требования:** [README](README.md); clear break обязателен.

## Цель и текущая проблема

Переданный host scope должен ограничивать весь набор инструментов материализованного execution, а context конкретного вызова — доходить до handler. Helper остаётся одной явно вызванной операцией model → validated tool batch.

В [runtime.go](../../runtime.go) `ToolScope` назван allowed/required boundary. Однако [render_plan.go](../../render_plan.go), `Execute`, вызывает только `ValidateExecutionManifestContract`, который в [tool_contract.go](../../tool_contract.go) возвращает success при пустом `RequiredTools`. Шаблон `Tools:[delete_all]` с `Allowed:[read]` материализуется без ошибки. Далее [tool_orchestration.go](../../tool_orchestration.go) считает `exec.Tools` разрешённым набором.

[tool_binding.go](../../tool_binding.go) объявляет `Handle(args)` и function handler без context; `ToolInvoker.InvokeTool` также не принимает context. `invokeToolBatch` проверяет отмену только перед handler и может вернуть success после отмены во время последнего вызова.

## Новый контракт

1. Различать отсутствие runtime scope и явно заданный scope с пустым `Allowed`. Отсутствие дополнительного scope оставляет explicit `exec.Tools` границей caller-built execution; присутствующий пустой `Allowed` запрещает все local tools. Не трактовать пустой список как wildcard.
2. Каждый итоговый tool definition должен входить в явный `Allowed` и соответствовать его name/schema/capabilities. Проверять после всех template/runtime overrides. Вне scope — ошибка, не молчаливое удаление инструментов. Required/forced tools должны одновременно существовать в execution и удовлетворять runtime scope. Duplicate/empty names отвергать, а не перезаписывать map entry.
3. Закрепить scope в существующем execution/runtime contract так, чтобы clone, direct helper, wire restore и последующие изменения не обходили проверку. Если scope переносится через wire, обновить соответствующую JSON Schema и format marker. Повторная проверка должна защищать границу фактического dispatch/handler; успешная проверка старой копии не разрешает изменённый execution.
4. Scope — декларативное ограничение вызывающего кода, не решение IAM. Не добавлять пользователей, роли, permissions backend, approvals или загрузку policy. Hosted provider tools отделены от local handlers и не получают разрешение через registry local tools.
5. Провести `context.Context` через ToolHandler, function constructor, TypedTool.Invoke, ToolInvoker, registry и one-shot helpers. Изменить сигнатуры напрямую; не оставлять `LegacyToolInvoker`, aliases или wrappers с `context.Background()`.
6. Весь batch по-прежнему проходит schema/scope/correlation validation до первого side effect. Context проверяется до model call, до каждого handler и после завершения handler. Новый handler после cancellation не стартует.
7. Отмена кооперативная: передать context в HTTP/DB caller, не пытаться принудительно убить Go-функцию и не оборачивать её в отрываемую goroutine. Сохранить известный результат/ошибку завершившегося handler вместе с canceled/deadline статусом; не утверждать, что side effect откатился. При нескольких ошибках сохранять `errors.Is` для context и исходной причины.
8. Зафиксировать контракт частичного результата и post-dispatch ошибки, включая случай `Invoker.Execute` возвращает response вместе с error. Не исполнять tools из неуспешно принятого ответа, но не терять доступный результат для host reconciliation. Не добавлять автоматический retry или compensation.

## Acceptance

- [x] `Allowed:[read]`, template `delete_all`, пустой `RequiredTools`: materialization/checked execution не допускает инструмент; model dispatch и handler count равны нулю.
- [x] Explicit empty scope, missing/duplicate tool, forced outside scope и изменённая schema отклоняются до side effects.
- [x] Позитивный путь required + forced + allowed выполняется; прямой caller-built execution с собственным `Tools` остаётся поддержанным без обязательного recipe/registry.
- [x] Scope сохраняется или явно rebind-required при clone/wire/checkpoint согласно новой спецификации; не исчезает при overrides.
- [x] Один invalid call из нескольких блокирует весь batch до invocation; большой общий handler registry не расширяет permissions execution.
- [x] Blocking HTTP fixture получает deadline/cancel текущего handler. Следующий handler не запускается; последний не маскирует cancellation success-результатом.
- [x] Уже завершённый side effect и известный result доступны при отмене/ошибке; повторный invoke не выполняется библиотекой.
- [x] Конкурентные вызовы одного registry получают собственные contexts без утечек между вызовами; race tests проходят.
- [x] Все constructors, examples и tests обновлены на новую сигнатуру, старый API удалён.

Не вводить параллельный tool scheduler, step runner, persistent run state или agent loop. Политика повторов и реакция на частичное выполнение принадлежат host. Закрытие задачи — по проверкам и migration из общего README.

## Приёмка

Round2 frozen identity `41748a679acbf36a40cdfb6a3b446e407379de33eb9a4f27f5f0dc5f809d84ed`, 685files. [Полнота](reports/task39/completeness-r2.md), [ошибки](reports/task39/errors-r2.md), [проверки](reports/task39/checks-r2.md), [closeout/migration](reports/task39/closeout.md). Source после приёмки не менялся; финализированы только статусы документов.
