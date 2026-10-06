# task42 — Сохранность tool calls, результатов и reasoning в адаптерах

**Статус:** принято r4: 18/18 (100%), 0 открытых подтверждённых ошибок, полные lint/test/race/generated consumers green, 7 октября 2026.
**Приоритет:** P1. **База аудита:** `70fc301`; **база реализации:** `d829ab2`, 6 октября 2026.
**Зависимость:** numeric semantics [task38](task38.md). **Правила:** [README](README.md).

## Цель

Исправить потери данных в уже существующих provider paths. Один логический exchange должен сохранять точные arguments, call/result identity, порядок и требуемое continuation state через sync/stream, wire и следующий запрос. Новые provider features из task43 не являются зависимостью исправлений.

## Подтверждённые участки

- [adapter/gemini/gemini.go](../../adapter/gemini/gemini.go), `assistantContent`: `json.Unmarshal` arguments в `map[string]any` округляет integer больше 2^53.
- [adapter/gemini/response_source.go](../../adapter/gemini/response_source.go), `originalSignedGeminiPart`: unsigned parts возвращаются из SDK DTO без восстановления точных source numbers. Actual SDK HTTP fixture с `9007199254740993` публикует `9007199254740992` без ошибки.
- [adapter/gemini/gemini.go](../../adapter/gemini/gemini.go), `toolResultContent`: `FunctionResponse` создаётся по name без переноса `ToolCallID`; исходящий HTTP request теряет ID.
- [adapter/ollama/ollama.go](../../adapter/ollama/ollama.go), assistant translation: `Content`/`ToolCalls` заполняются, `Thinking` отсутствует. Parse/stream при этом публикуют `ReasoningPart`.
- Уже существуют [Gemini state codec](../../adapter/gemini/state_sdk_encoding.go), [partial state](../../adapter/gemini/partial_state.go), [Ollama exact HTTP decode](../../adapter/ollama/chat_http.go) и [tool number checks](../../adapter/ollama/tool_numbers.go). Сохранять эти гарантии, устраняя дублирование при необходимости.

## Требования

1. Числовой контракт действует для всех arguments, включая unsigned calls. Извлекать source JSON до необратимого SDK float conversion либо использовать проверенный exact path; одного `UseNumber` перед SDK может быть недостаточно из-за последующей сериализации SDK.
2. При direct parse caller-supplied SDK DTO без original source не выдумывать точность. Описать допустимые numeric representations и явно отвергать случаи, где точность нельзя удостоверить. Не отвергать все числа/все unsigned calls вместо реализации корректного IO пути.
3. Gemini переносит native call ID в `FunctionResponse.ID` и сохраняет порядок/корреляцию repeated-name parallel calls. Старый ID-less протокол остаётся возможным только при подтверждённой name-correlation capability; не генерировать фиктивный ID и не подменять ID именем.
4. Ollama переносит доступные reasoning parts в `Message.Thinking` при continuation. Зафиксировать поддерживаемую гранулярность: если canonical interleaving невозможно представить в одном native field, явно ограничить/отвергнуть такую форму, не терять части. Не смешивать thinking с пользовательским текстом.
5. Проверить IsError и multimodal shapes в существующих result mappings: сохранять представимый смысл, а непредставимое явно отвергать. Не молча превращать ошибку инструмента в success. Новые multimodal mappings — task43.
6. При исправлениях не ослаблять Gemini signed part ordinal/signature binding, endpoint/model scope, partial completion rules, unknown outcome или оригинальную schema validation. Incremental args остаются provisional до подтверждённого completion.
7. Проверить применимость исправлений к sync, native streaming, batch results и history restore. Если путь не существует, обозначить unsupported; если существует и публикует canonical calls — гарантии одинаковы. SDK DTO tests не заменяют HTTP/SSE/JSONL serialization tests.
8. Core не получает provider-specific numbers/state types. Host выбирает DTO, handlers, endpoint, transport и policies. Несовместимые codec/wire версии явно отклоняются; legacy repair отсутствует.

## Acceptance

- [x] Gemini HTTP response с unsigned `9007199254740993` даёт точные `ToolCallPart.Args`; typed int64 handler получает тот же ID.
- [x] Следующий actual SDK HTTP request сохраняет число; покрыты nested arrays/objects, дроби/exponents, отрицательные большие числа и пустой `{}`.
- [x] Sync и native stream для эквивалентных ответов публикуют одинаковые точные complete arguments; применимые batch parse paths проходят те же cases.
- [x] Wire response → AssistantMessage → next request сохраняет call ID; два вызова одной функции с разными IDs получают правильные FunctionResponse IDs.
- [x] Name-only supported path сохраняется явно; malformed/duplicate/ambiguous correlation отвергается без guessing.
- [x] Ollama thinking → canonical response → wire → next request не теряется, не попадает в ordinary text и не логируется автоматически.
- [x] Signed Gemini и частичные signed sequences сохраняют исходный порядок и signatures; altered/missing required state отвергается.
- [x] Unknown source precision и неподдержанная result semantics дают явную ошибку с сохранением доступного partial result, без автоматического retry.
- [x] Actual SDK fixtures воспроизводят исходные дефекты и доказывают исправление; race/cancellation conformance не регрессирует.

До реализации сверить используемые версии SDK и официальный протокол: [Gemini function calling](https://ai.google.dev/gemini-api/docs/generate-content/function-calling?authuser=0&hl=en), [Ollama tool calling](https://docs.ollama.com/capabilities/tool-calling). Отдельно документировать checked и low-level parse guarantees. Проверки/миграция — по README.
