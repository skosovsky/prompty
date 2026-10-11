# Задачи по результатам архитектурного аудита prompty

Основание: аудит всех Go-пакетов 6 октября 2026 и повторная сверка реализации при подготовке задач. Базовый commit: `70fc301`. Статусы реализации и независимой приёмки фиксируются в заданиях и reports/taskNN.

Каталог `.cursor/task` создан по указанию владельца. Нумерация продолжает task1–37 из `.cursor/docs`; исторические задания не переносить и не переписывать. Новые задачи дополняют [task37](../docs/task37.md), не отменяя его уже реализованные гарантии. Перед реализацией проверять delta к текущему HEAD, чтобы не строить существующие возможности повторно.

## Общие требования

- Spec-First / Contract-First: сначала описать публичные inputs/results/errors/invariants, обновить применимые JSON Schema, godoc и conformance fixtures, затем реализацию. Имена новых типов выбирать после сверки текущих интерфейсов.
- **Clear break. Обратная совместимость не нужна.** Заменять мешающие сигнатуры и семантику; удалять устаревшие пути, aliases, adapters совместимости и silent fallback. Старые несовместимые wire/checkpoint форматы явно отвергать, а не чинить с потерями. Обновлять все затронутые примеры и consumers в репозитории.
- Core остаётся универсальным: BYOT, небольшие порты, caller-owned DTO и политики. SDK-типы и vendor mapping остаются в adapter-модулях. Не вводить обязательный собственный контейнер приложения или зависимость на соседние библиотеки автора.
- Host владеет agent loop, routing, retries, approvals/IAM, storage, credentials, egress, retention, scheduling и оценкой качества агента. Библиотека проверяет переданные контракты и выполняет явно вызванную операцию. Cancellation не означает rollback, checkpoint рецепта не означает checkpoint агентного run.
- Не добавлять фоновые процессы, неявные сетевые probes, model catalogs, загрузки media, автоматические повторные отправки или синтетические fallback. Не обещать model/endpoint support по одному SDK-полю.
- Использовать существующие execution, schema, state, stream, capture и usage contracts. Сохранять различие unknown/zero, partial/completed, native/buffered, state/logging и checked/raw.
- Тесты — AAA и публичные сценарии отказов. Provider mapping проверять actual SDK HTTP/SSE fixtures, а не только DTO mocks. После изменения запускать targeted tests, затем обязательные `make lint` и `make test` для всех модулей. Live-проверки выделять отдельно; локальные fixtures не являются live conformance.
- В closeout указать изменённые контракты, удалённые legacy paths, миграцию вызывающего кода, выполненные проверки и оставшиеся ограничения. Тип изменения — breaking; публикация, теги, отправка сообщений и закрытие issues не входят в эти задания.

## Крупные задачи и порядок

| Задача | Приоритет | Результат | Зависимости |
| --- | --- | --- | --- |
| [task38](task38.md) | P1/P2 | Точный JSON и согласованные schema / DTO / codegen contracts | Основа для 39, 42, 43 |
| [task39](task39.md) | P1 | Проверяемый tool scope и context-aware one-shot execution | Использует точное сравнение из 38 |
| [task40](task40.md) | P1 | Snapshot, digest и materialization относятся к одним исходникам | Самостоятельна; согласовать schema composition с 38 |
| [task41](task41.md) | P2 | Сокращение контекста сохраняет текущую задачу и tool dependencies | Согласовать scope/state с 39 и 42 |
| [task42](task42.md) | P1 | Lossless tool/reasoning round-trip в существующих адаптерах | Числовой контракт из 38 |
| [task43](task43.md) | P2 | Явные provider capabilities для современных агентных запросов | После базовых гарантий 38 и 42 |
| [task44](task44.md) | P2 | Диагностируемое выполнение и достоверная capability matrix | Можно начать сразу; финальная matrix после 38–43 |

Сначала устранить нарушения имеющихся гарантий в 38–40 и 42. Исправления fidelity не должны ждать новых provider features из 43. Каждая задача имеет собственный законченный результат; промежуточная реализация не закрывает оставшиеся acceptance criteria.

## Что намеренно не добавляется

Собственные multi-agent framework, MCP runtime, memory engine, summarization agent, workflow scheduler, evaluation runner и billing. Anthropic server-tool протокол в task43 ограничен явно указанными hosted search tools; он не даёт оснований добавлять sandbox, browser/computer-use executor или управление пользовательскими сессиями.
