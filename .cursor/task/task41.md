# task41 — Безопасное сокращение контекста с явным результатом бюджета

**Статус:** принято r2: 17/17 (100%), 0 открытых подтверждённых ошибок, full lint/test/race/generated green, 7 октября 2026.
**Приоритет:** P2. **База аудита:** `70fc301`; **база реализации:** `39b6c4e`, 7 октября 2026.
**Правила:** [README](README.md). Учесть state/correlation из [task42](task42.md).

## Цель и основание

Сокращать историю без потери текущей задачи, повреждения tool exchange или ложной гарантии соблюдения бюджета. Это детерминированная трансформация переданного execution; выбор памяти и summarization strategy остаётся у host.

[ext/truncate/truncate.go](../../ext/truncate/truncate.go) защищает system/developer messages, но удаляет все остальные блоки, включая последний user turn. System message между call и result разделяет их на разные removable segments. Если защищённая часть больше бюджета, функция возвращает success. [middleware/contextx/contextx.go](../../middleware/contextx/contextx.go) безусловно передаёт результат дальше. [token.go](../../token.go) и [adapter/token_estimate.go](../../adapter/token_estimate.go) уже разделяют message counting и provider execution estimation.

## Контракт

1. Сохранить system/developer messages и текущий turn по умолчанию. Явно определить turn для истории с user message и для продолжения, заканчивающегося assistant/tool без нового user. При отсутствии user marker использовать консервативную защиту текущего exchange либо требовать caller boundary; не угадывать по тексту.
2. Удалять call/results как связанный набор, независимо от физической смежности и промежуточных system messages. Учитывать parallel calls, несколько результатов и незавершённый exchange. Не фабриковать IDs. ID-less correlation допустима только в рамках существующего подтверждённого контракта; неоднозначные зависимости не обрезать эвристически.
3. Scoped provider-state, annotations и part associations не должны ссылаться на удалённые parts. Не редактировать signed content; сохранять или удалять целиком связанное состояние. Если transform несовместим с требуемым continuation, вернуть диагностируемую ошибку до provider IO.
4. Caller может явно задать защищённые границы/сообщения через универсальный policy/strategy contract. Не вводить доменные понятия agent goal, project memory или task database. Нынешний Strategy API можно заменить clear break.
5. Результат содержит достаточный отчёт: что удалено, оценка до/после, budget satisfied/unsatisfied и причина невозможности. При недостижимом бюджете middleware по умолчанию возвращает error и не dispatch. Если нужен advisory режим, он должен быть явным; oversize success по умолчанию недопустим.
6. Указать, что именно ограничивает budget: messages или весь provider request. Message-only counter не гарантирует общий context window с tools/schema/media/framing/output reserve. Для общего бюджета использовать имеющийся execution estimator/инъецируемый порт; неизвестная оценка не становится нулём. Не загружать tokenizer и model limits автоматически.
7. Sync и stream middleware имеют одинаковые semantics. Stream остаётся lazy/single-use; invalid input и невозможный бюджет не запускают upstream. Все трансформации работают на clone и не изменяют исходную историю.
8. Проверить отрицательные/нулевые бюджеты, nil counter, ошибки и некорректные значения counter; определить явное disable поведение без двусмысленного success.

## Acceptance

- [x] System `sys` + длинный последний user при бюджете только на system не превращаются в успешно отправленный system-only запрос.
- [x] Только system/developer выше бюджета → unsatisfied + zero dispatch.
- [x] User → assistant call → system policy → tool result: после сокращения не остаётся orphan result/call.
- [x] Parallel calls и частично завершённый exchange сохраняют correlation и текущий turn.
- [x] State/annotations остаются валидны после удаления; подписанное содержимое не переписывается.
- [x] Переданный caller policy применяется одинаково в sync/native/buffered paths; старый input неизменен.
- [x] Ошибка counter и недостижимый бюджет не вызывают модель. Report объясняет фактическую область оценки.
- [x] Сценарий с большими tools/schema доказывает, что message-only оценка не выдаётся за полный provider budget.

Не добавлять summarization LLM call, retrieval, vector store, persistent memory, implicit compaction или автоматическую повторную генерацию. Обновить документацию ext/middleware и examples; проверки по README.
