# task43 — Явные provider capabilities для reasoning, streaming и native tools

**Статус:** accepted; r2 полнота24/24=100%, открытых подтверждённых ошибок0; все проверки зелёные. Отчёты: reports/task43/completeness-r2.md и errors-r2.md.
**Приоритет:** P2; функциональное расширение после исправлений fidelity.
**База:** `70fc301`, 6 октября 2026. **Зависимости:** [38](task38.md), [42](task42.md).
**Правила:** [README](README.md). Все перечисленные ниже направления входят в acceptance; opt-in означает выбор consumer, не отложенную реализацию.

## Цель

Закрыть конкретные пробелы provider mapping, необходимые внешнему agent harness. Сохранить универсальные core contracts и фактическую проверку endpoint/model capability. Не строить собственный agent runtime и не пытаться покрыть все API провайдера.

## Delta к коду

| Область | Что уже есть | Чего нет |
| --- | --- | --- |
| OpenAI Responses | Create/Get/Cancel, background, local/hosted tools, scoped reasoning/history | `RuntimeDescriptor` сбрасывает ReasoningSettings; request отвергает любые ProviderSettings; native Responses stream отсутствует; local tools всегда Strict:false |
| Ollama | chat, local tools, native stream, точный tool decode | Output schema отвергается; native Format mapping и Think controls отсутствуют |
| Gemini | signed state, native stream/partial args, batch/cache | Tool result с media отвергается как неподдержанный mapping |
| Anthropic | Messages, native stream, thinking, synthetic/native output, batch | server_tool_use/results не разобраны; pause_turn теряет resumable semantics в общем Incomplete |

Проверенные файлы: [Responses](../../adapter/openai/responses.go), [request preparation](../../adapter/openai/responses_request.go), [tool mapping](../../adapter/openai/responses_tools.go), [Ollama schema](../../adapter/ollama/schema.go), [Gemini result mapping](../../adapter/gemini/gemini.go), [Anthropic parse](../../adapter/anthropic/anthropic.go), [outcomes](../../adapter/anthropic/state.go). Общие контракты: [remote_response.go](../../remote_response.go), [stream.go](../../stream.go), [capability_profile.go](../../capability_profile.go).

## Контракты до реализации

- Составить support matrix: provider endpoint/API × operation × model profile × setting/feature × implemented port. Наличие поля SDK не доказывает поддержку модели. Проверить свежую официальную документацию и установленный SDK.
- Для каждого направления определить inputs/results/errors, required capabilities, state/capture/usage semantics и wire changes. Не создавать vendor DTO в core и не превращать ProviderSettings в непроверяемый произвольный passthrough.
- Переиспользовать текущие Stream lifecycle, schema engine, profile identity и provider-state envelope. Provider events сохраняются в адекватном scoped representation; неизвестные обязательные events не теряются молча.
- Unknown/unsupported requirement должен отказать до IO. Capture и diagnostics не заявляют live support по локальному fixture.

## Обязательная реализация

### OpenAI Responses

1. Явные reasoning effort и summary controls, валидируемые profile ModelOptionsSchema; не переносить автоматически Chat Completions whitelist. Сохранять отличие запрашиваемого summary от opaque encrypted state.
2. Native streaming для caller-invoked foreground Responses create: text, reasoning summaries/state, local tool arguments, поддержанные hosted search records/citations, late usage, terminal outcomes и ошибки. Remote handle и observed result сохраняются, если уже доступны.
3. Интеграция со Stream single-use/Close/Status/cancellation; no sync fallback. Не эмулировать SSE циклом Get. Get/Cancel/background остаются самостоятельными explicit operations; reconnect/resume transport не входит в задачу.
4. Явная policy/capability native strict local tools. Совместимость schema с native strict проверять до dispatch; original-schema validation после ответа остаётся. Не устанавливать strict без проверки и не делать скрытый lossy lowering.

### Ollama

5. Native structured output через Format для подтверждённого endpoint/model и исходной schema. Сохранить original-schema validation в sync и доступном native stream; nonterminal JSON остаётся provisional. Для endpoint без этой возможности — явный отказ, без prompt/synthetic fallback.
6. Thinking controls через существующий validated settings/profile слой; проверять допустимые значения/комбинации endpoint/model. Не выводить поддержку из имени модели. Fidelity thinking continuation обеспечивается task42.

### Gemini

7. Native multimodal function results для представимых text/image частей: сохранить ID, порядок частей, MIME и caller-owned bytes/URI. Отдельно ограничить допустимые формы и backend support; unsupported MIME/URI shape отвергать. Никаких неявных download/upload/recache.

### Anthropic

8. Ограниченный явно выбираемый hosted web search/web fetch contract: declarations, server_tool_use и соответствующие result/error blocks, citations и pause_turn continuation. Список поддержанных kinds/schema зафиксировать до реализации; остальные kinds fail-closed.
9. Hosted execution не выдаётся за local handler call. Сохранить исходный порядок и state через response/history/wire; resumable pause не путать с completed success или разрешением исполнять local tool.
10. Продолжение выполняет caller отдельным запросом. Не добавлять loop для автоматического resume, computer-use/browser execution, code sandbox, credentials manager или session service.

## Acceptance

- [x] Actual SDK requests подтверждают reasoning/strict/Format/Think/media/hosted mappings; invalid/unknown profile даёт zero IO.
- [x] Responses SSE fixture включает mixed text + local functions + hosted search + reasoning + late usage; parts и IDs устойчивы, ровно один terminal, premature EOF — ошибка с partial result.
- [x] Отмена/Close во время чтения закрывает underlying body и сохраняет доступный handle/state/usage; unrelated concurrent stream не прерывается.
- [x] Strict tool arguments и Ollama output проверяются по original schema; refusal/incomplete/invalid JSON не маскируются schema success.
- [x] Gemini multimodal result round-trips без перестановки и потери ToolCallID; неподдержанные формы отвергаются до IO.
- [x] Anthropic hosted result/error + pause_turn сохраняются через wire и явное продолжение; local handler count остаётся нулевым для hosted calls.
- [x] Capture redaction, usage presence, operation-only Get/Cancel preflight и existing background/batch/cache APIs не регрессируют.
- [x] Для каждого направления есть offline runnable example с SDK fixture и негативными случаями; live account/model conformance обозначено отдельно.
- [x] Public docs и capability matrix описывают фактическую гранулярность, исключения и migration; устаревшие raw/legacy routes удалены при смене контрактов.

## Границы

Не добавлять Responses batch, Gemini Interactions API, Vertex batch, все Anthropic server tools, realtime/voice/WebSocket sessions или model discovery автоматически: это отдельный scope, не следствие этой задачи. Использовать текущие provider ports без дублирующего framework. Проверки всех модулей и release preparation — по README.
