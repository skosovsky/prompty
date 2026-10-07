# task44 — Наблюдаемость исполнения и достоверная карта возможностей

**Статус:** accepted r3 — полнота 19/19 (100%), открытых подтверждённых ошибок 0; full lint/test/race/generated passed. **Приоритет:** P2.
**База:** `70fc301`, 6 октября 2026. **Правила:** [README](README.md).
**Зависимости:** базовую observability можно делать сразу; финальные docs сверить с результатом task38–43.

## Цель

Позволить host диагностировать стоимость в токенах, outcome, отмену, schema/capability отказ и частичные результаты без потери известных фактов и без неявной отправки содержимого промптов в telemetry. Документация должна однозначно показывать реализованную поверхность каждого адаптера.

## Основание в коде

- [ext/otelprompty/otel.go](../../ext/otelprompty/otel.go): собственные `prompty.*` attributes, total tokens только при `>0`, преимущественно request model. При `resp != nil` вместе с error sync path возвращается до записи известных usage/finish metadata.
- В streaming уже используется `ObserveLifecycle`, сохраняющий реальный handle. Этот механизм оставить, не оборачивать Events новым producer loop.
- [usage.go](../../usage.go), [response.go](../../response.go), [capture.go](../../capture.go) и [capture_metadata.go](../../capture_metadata.go) уже содержат presence-aware usage, outcomes, diagnostics и host-owned redaction/capture. Использовать их, не создавать вторую модель результата.
- [adapter/openai/README.md](../../adapter/openai/README.md) всё ещё называет local tools/history нереализованными, хотя соответствующие файлы и fixtures существуют. Остальные README и task37 evidence тоже требуют сверки с реальным snapshot.

## Требования

1. Зафиксировать поддерживаемую версию и статус OpenTelemetry GenAI semantic conventions по актуальному официальному источнику. Использовать стандартные attributes там, где семантика совпадает; расширения prompty документировать. Не обещать совместимость со всеми будущими версиями conventions.
2. Записывать доступные request/effective response model, provider/operation, outcome/finish, input/output/total/cache/reasoning usage. Unknown не равен zero; известный ноль сохраняется. Cache/reasoning subsets не суммируются с total повторно. Не вычислять денежную стоимость и не вводить pricing catalog.
3. Metadata известного response записывать и при error, включая post-dispatch capture/schema errors. Отдельно классифицировать preflight rejection, transport failure, provider refusal/incomplete, cancellation и consumer stop. Не превращать refusal в транспортную ошибку автоматически.
4. Один logical invocation/stream lifecycle не выдаётся за количество физических HTTP attempts. SDK transport instrumentation может быть вложенной; исключить двойной учёт одной операции. Не объявлять retries, которых слой не наблюдает.
5. Поддержать caller-owned tracer/provider без обязательной глобальной настройки. Текущее имя tracer может остаться настройкой, но новые API не должны вынуждать process-wide singleton. Оставить OTel dependency в extension module.
6. По умолчанию не записывать prompt, arguments/results, reasoning, opaque state, credentials и сырой payload. Request URL/account refs/high-cardinality IDs и error text требуют явной host projection/redaction; не считать произвольный error.Error() безопасным. Сохранять полезный machine-readable error classification без payload.
7. Streaming span начинается при фактическом запуске lazy stream и завершается один раз. Отсутствие consumption, Close до/после start, terminal/error и cancellation имеют проверяемые semantics без новых goroutine leaks.
8. Для existing Responses/batch/cache операций описать и реализовать минимальную instrumentation интеграцию через существующие operation/capture ports либо явные wrappers extension. Не приписывать им spans от обычного Invoker, если этот путь они не используют. Разделить job/resource outcome и результат отдельного model generation/item.
9. Составить одну capability matrix, связанную с кодом и fixtures: adapter/API, operation, native/buffered stream, local/hosted tools, reasoning controls/state, structured output mode, media/result shapes, batch/cache, checked/raw guarantees. Unknown и unsupported различать. Знание конкретной модели остаётся caller-owned profile.
10. Обновить root README, adapter README/godoc, примеры и migration по завершённым задачам. Убрать stale statements о существующих возможностях и неверные обещания универсальности. Исторические audit checkpoints не переписывать как свидетельство нового HEAD; добавить актуальное evidence отдельно.

## Acceptance

- [x] In-memory OTel exporter проверяет sync/stream attrs для known zero, unknown, cached/reasoning breakdown и effective model override.
- [x] Response + error сохраняет известный usage/outcome; preflight error не выглядит как состоявшийся provider generation.
- [x] Cancel/consumer stop/Close/terminal завершают span ровно один раз; lazy unused stream не получает фиктивную duration provider IO.
- [x] Prompt/tool/reasoning/opaque state и секрет из fixture error text отсутствуют в default exported attributes/events/status. Явная host projection работает без изменений core DTO.
- [x] Responses/batch/cache examples показывают корректный observation путь и не считают один usage дважды.
- [x] Каждый положительный пункт capability matrix имеет код/fixture evidence; отсутствие live проверки обозначено, модели не объявляются поддержанными по умолчанию.
- [x] README не содержит противоречий вроде «local tools не реализованы» рядом с работающим API; examples компилируются и используют checked paths там, где обещают гарантии.
- [x] Migration описывает clear-break изменения 38–43 и удалённые API, не рекомендует compatibility wrappers или ручное игнорирование ошибок.

## Границы

Не строить telemetry backend, dashboard, eval runner, storage, billing, общий secret scanner или auto-redactor с обещанием полной очистки. Не добавлять prompt optimization или agent quality metrics без наблюдаемых данных. Extension публикует проверяемые события и metadata; host определяет exporter, policy и retention. Проверки/closeout — по README.
