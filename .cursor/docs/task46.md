# task46 — Контракт tool dispatch и проверяемая композиция с host

Источник: https://github.com/skosovsky/prompty/issues/5
Статус: выполнено. Реализация, независимая приёмка, релиз и closeout подтверждены в [итоговом отчёте](../task/reports/task46/closeout.md).
Основание: issue и код сверены 7 октября 2026; комментариев к issue нет. Нумерация продолжает существующие task1–45 во всех каталогах задач.

## Цель и архитектурное решение

Дать потребителю проверенный путь от полного `Response` до исполнения инструментов и продолжения диалога без потери correlation, provider state и смысла результатов. Сохранить универсальность prompty: core отвечает за свои execution/schema/response/stream-контракты; приложение — за admission policy, approval, эффекты и жизненный цикл операции.

**Выбранный вариант из issue:** `ExecuteWithTypedTools` остаётся JSON-only one-shot helper. `ToolInvoker.InvokeTool(ctx, name, argsJSON)` не получает native call ID и не объявляется портом полноценного lifecycle. Caller, которому нужны ID внутри исполнения, approval или richer outcomes, использует полный `Response` и собственный dispatcher. Не добавлять ID-only расширение helper, создающее ложное обещание поддержки lifecycle.

Обратная совместимость не требуется. Если для соблюдения выбранного контракта потребуется существенное изменение API/поведения, выполнить clear break, удалить заменённые пути и обновить consumers; не создавать aliases или compatibility bridges. Само разрешение clear break не является причиной менять работающий API.

## Подтверждённая исходная точка

- `tool_orchestration.go`: helper уже проверяет execution scope, original schemas и весь batch до handlers; сохраняет call ID в результирующем сообщении, но не передаёт его исполнителю. Обычная ошибка handler не останавливает batch; исходные причины доступны через `errors.Is`/`errors.As`.
- `docs/tool-execution-contract.md`: one-shot и ownership уже описаны. Нужны точные ограничения и исполняемый полный сценарий, а не повтор реализации этих гарантий.
- `task45_tool_boundary_example_test.go`: пример вызывает только fake caller port; полного dispatch и реальной интеграции он не доказывает.
- `Response.AssistantMessage()` и response wire уже предназначены для сохранения continuation. Проекция только в `TextContent` для этого недостаточна.
- `stream.go`: `Stream.Close()` запрашивает отмену без ожидания unwind; это намеренный callback-safe контракт.

Перед реализацией сверить delta с актуальным кодом и повторно использовать существующие regression tests. Материалы аудита, упомянутые в issue, не считать обязательным локальным prerequisite: перенести нужные fixtures в репозиторий или воспроизвести их по самостоятельным сценариям ниже.

## Объём реализации

### 1. Сначала исполняемый контракт

Обновить `docs/tool-execution-contract.md`, добавить `docs/host-tool-dispatch.md`, включить ссылку в `docs/README.md` и README. Зафиксировать inputs/results/errors, ownership, поддерживаемые проекции и точки отказа до реализации examples и исправлений.

Явно различить:

- simple helper: один model call, JSON-result, обычные handler errors продолжают заранее допущенный batch, cancellation останавливает последующие вызовы;
- full dispatch: полный `Response`, native ID внутри исполнения, caller-owned outcomes, admission и выбранная приложением stop-on-control policy;
- ordinary error, business failure при `err == nil`, lifecycle control и cancellation — разные случаи; core не распознаёт внешние sentinel/errors, не запускает approval и не подменяет pause отменой контекста.

### 2. Runnable full-dispatch recipe

Создать optional module `examples/host_dispatch` с исполняемым примером и AAA tests. Пример работает без credentials и платных запросов, через локальные provider fixtures; внешние интеграционные зависимости остаются внутри этого модуля.

Путь: checked `Invoker` → canonical `Response` → admission всего batch → caller dispatcher → явная проекция tool outcomes → следующее checked execution. Конкретные adapter types остаются в примере, доменные DTO принадлежат caller; общего Agent/Message/Outcome-контейнера в core не появляется.

Обязательные свойства:

1. Проверять ошибку dispatch, metadata/outcome и допустимость continuation до effects. Ответ с ошибкой сохраняется как evidence; его calls автоматически не исполняются.
2. Перед первым effect проверить весь batch: execution scope, forced tool, original schemas, валидность raw JSON, correlation и caller validation. Invalid последний call означает ноль effects во всём batch. Validation ports не выполняют effects.
3. Передать caller ID, name и исходные raw args без промежуточного `float64`. Два одинаковых name/args с разными IDs исполняются и коррелируются раздельно. Duplicate IDs и неоднозначные ID-less calls отвергаются; подтверждённый name-correlation режим следует существующему контракту.
4. Сохранить неизменяемый snapshot original schemas для admission/ToolScope. Схемы, упрощённые для provider transport, не заменяют исходные ограничения. Переиспользовать существующие validators; не копировать весь private orchestration в пример. Если публичных primitives недостаточно, выделить минимальный универсальный pure-validation port после фиксации его контракта; никакой policy или исполнения в нём.
5. При host-managed pause/approval по выбранной stop-on-control policy следующие effects не начинаются. Причина сохраняется с исходным типом, завершённые результаты и pending call ID остаются доступны host. Не создавать фиктивный successful/error model result для ещё не исполненного вызова. Продолжение после решения выполняется явно host с тем же pending ID; завершённые calls не переисполняются автоматически.
6. Caller проверяет business failure даже при `err == nil` и формирует error result, если provider mapping это поддерживает; иначе возвращает явную unsupported-ошибку. Audience filtering, controls/effects, binary/media и sensitive diagnostics обрабатываются host до model projection. Controls и приватные данные не превращаются в текст для модели по умолчанию.
7. Использовать `AssistantMessage()` и явную wire policy для response persistence/restore. Сохранять scope provider/endpoint/model, opaque required state, media, error flags, part/message annotations и их associations. Неподдерживаемый mapping явно отвергать, не терять данные молча; logging projection не использовать для continuation.

### 3. Mapping и lifecycle recipes

В `docs/host-tool-dispatch.md` добавить таблицу поддерживаемого mapping: canonical field → caller representation → обратный mapping → unsupported cases → test. Покрыть как минимум tool IDs/raw args/results, provider scope/state, media, error, annotations и continuation availability. Сопоставить её с реальным контрактом подключённого optional consumer, включая его декларацию поддерживаемого mapping; core не получает consumer-specific policy.

Показать lifecycle recipe с одним владельцем `Events()`: `Close()` можно вызвать из callback или другого потока; завершение cleanup подтверждается выходом владельца из consumption и отдельным caller-owned completion signal. Не ждать этот signal внутри callback; не добавлять блокирующий join в `Close()`, polling по status или скрытый producer goroutine. Close до consumption не запускает provider. Cancellation не доказывает rollback или отсутствие remote effect.

### 4. Реальные optional integrations и CI

Использовать реальные imports и публичные API consumers из связанных issue для проверки границ tool execution, context mapping и stream lifecycle. Fake ports оставить для изолированных unit tests, но не считать заменой integration acceptance. Не менять чужие репозитории в рамках этой задачи.

Добавить переносимый runner и CI job для optional module:

- workspace/local-source режим через явно переданные пути или checkout sources; никаких абсолютных путей автора;
- отдельно опубликованный набор зависимостей с `GOWORK=off`, без local replace и без незаметной подмены исходников;
- в обоих режимах запускать semantic tests с `-race -count=1`, не ограничиваться build;
- фиксировать фактически проверенные dependency refs и команды в отчёте. Если опубликованный набор не предоставляет необходимый контракт, явно фиксировать блокер, не пропускать тест и не объявлять acceptance выполненной.

Core и существующие production modules не получают новых обязательных зависимостей на consumers. Подключение fixtures к CI/runner не должно нарушить inventory и release graph; пример не становится обязательным runtime dependency.

## Acceptance criteria

- [x] AC1. Контракт явно разделяет helper/full dispatch; выбранный вариант реализован, core не знает внешних lifecycle errors и не владеет approval/policy.
- [x] AC2. Runnable example проходит полный путь до следующего checked request; actual provider SDK + local HTTP/SSE fixture проверяет отправленную continuation, а не только DTO.
- [x] AC3. Одинаковые name/args с разными native IDs сохраняют ID внутри реального caller execution и в model results; duplicate/ambiguous IDs отклоняются до effects.
- [x] AC4. Scope violation и invalid последний call дают ноль effects; original-schema ограничения продолжают действовать после transport lowering. Значение `9007199254740993` сохраняется точно на всём пути.
- [x] AC5. Host pause/approval останавливает следующие effects, сохраняет `errors.Is`/`errors.As`, pending ID и completed results; явное разрешение продолжает pending call без повторения завершённых effects. Это локальный contract test, не обещание production IAM/durable engine.
- [x] AC6. Обычная handler error в helper продолжает admitted batch; cancellation его прекращает. Business failure при `err == nil` в полном dispatcher никогда не выдаётся за success. Audience/controls/private data не утекают в model content.
- [x] AC7. Round-trip проверяет state/media/error/annotations и scope. Каждый unsupported case из mapping-таблицы имеет отрицательный test: ошибка до следующего dispatch, без silent drop. State-omitted logging checkpoint отвергается для требуемого continuation.
- [x] AC8. Callback/concurrent/repeated Close не deadlock; Close до consumption не dispatch; отдельный completion signal подтверждает cleanup после unwind. Тесты синхронизированы каналами, без sleep-based доказательств.
- [x] AC9. Реальные optional integrations проходят в обоих dependency режимах; dependency graph подтверждает независимость core. CI не зависит от локальных материалов аудита.
- [x] AC10. AAA regressions и targeted integration checks, затем `make lint`, `make test`, `make release-check` проходят; ограничения live-проверок указаны честно. Обновлены affected docs/examples/godoc и migration guidance.
- [x] AC11. Два независимых приёмочных отчёта завершены: первый — полнота реализации AC1–AC10 в процентах и evidence по каждому пункту; второй — поиск ошибок, regressions и нарушений границ с severity, воспроизведением и файлами. Каждый AC засчитывается целиком; цель — 100%, без открытых подтверждённых ошибок. Исправления проверены повторно по затронутым критериям.
- [x] AC12. Выполнены release и closeout ниже; есть release evidence, URL комментария автору и подтверждение закрытого issue. До этого задача не называется полностью завершённой.

## Release guidance и closeout

По выбранному scope ожидается `make release-patch`, если результат ограничится docs, examples, fixtures и несущественными исправлениями без существенного изменения API/поведения. При существенном изменении API, семантики или публичных контрактов — `make release-break`; обоснование выбора обязательно в отчёте. Конкретные номера релизов в этом ТЗ не задаются.

Релиз выполнять после приёмки на коммите с реализацией и с соблюдением существующего release transaction contract. Проверить фактическое опубликование refs; при неопределённом результате использовать reconciliation, не запускать новый релиз вслепую.

После успешного релиза явно обратиться к автору issue в комментарии и дать конкретную инструкцию миграции с runnable example:

- для простого JSON one-shot оставить helper; он не получает native ID внутри handler и не обрабатывает approvals;
- для ID/approval/business outcomes перейти на полный `Response` и host dispatcher, перенести admission и outcome projection в host;
- заменить сборку истории из одного `TextContent` на `AssistantMessage()` и сохранение required state;
- ожидать cleanup через завершение consumption, а не через возврат `Close()`;
- перечислить дополнительные изменения caller code, если публичный API действительно изменился, с before/after snippets.

Опубликовать результаты проверок, ограничения и ссылку на релиз, затем закрыть issue как выполненное. Комментарий с общим «исправлено» недостаточен. На этапе подготовки этого ТЗ issue не закрывать и сообщения не отправлять. Отчёты реализации/приёмки/closeout сохранять в `.cursor/task/reports/task46/`; отсутствие внешнего действия фиксировать как незавершённый closeout.

## Goal для запуска

> Реализуй `.cursor/docs/task46.md` полностью по Spec-First / Contract-First. Сначала проверь актуальный код и зафиксируй контракты, затем выполни весь scope и acceptance criteria, сохрани универсальность core и BYOT; обратную совместимость не сохраняй там, где нужен clear break. Не расширяй библиотеку до runtime приложения. После реализации запусти двух независимых субагентов: первый оценивает полноту AC1–AC10 в процентах с доказательствами, второй ищет ошибки, регрессии и нарушения архитектурных границ. Исправляй замечания до 100% полноты и отсутствия открытых подтверждённых ошибок; повторяй приёмку затронутых частей. Выполни обязательные проверки и подходящий release target по ТЗ, проверь публикацию. Явно сообщи автору issue, что изменить в вызывающем коде, приложи инструкции миграции и закрой issue после успешного closeout. Сохрани evidence в `.cursor/task/reports/task46/`. Не объявляй goal завершённым при непрошедших интеграциях, незавершённом релизе или closeout; отдельно укажи точный блокер, если он возник.
