# Task42 — independent errors audit r3

Decision: **REJECTED**. Открыты **2 подтверждённых P1 класса ошибок**. Green checks не отменяют adversarial findings.

## Identity and independence

Проверен только frozen `/tmp/prompty-task42-snapshot-03`, база `d829ab2`, 730 файлов. SHA256 manifest: `64025d9711139051bbc03aa66d19373a18264227ab769e74acb564eefcbe01b4`. Все hashes проверены до и после: `/tmp/task42-errors-r3-integrity.log`, `/tmp/task42-errors-r3-integrity-final.log`. Snapshot и working implementation не менялись. Пробники — `/tmp/prompty-task42-errors-probes-r3`; чистые checks — `/tmp/prompty-task42-errors-checks-r3`. Текущий peer report не читала, в реализации не участвовала.

Прочитаны task42, общий task README, contracts, closeout, provider exchange contract и текущая реализация. Scope включает precision/source binding, signed/partial state, ordinal/signatures/scope, native IDs/name-only capability admission, IsError и multimodal отказ, reasoning, sync/native/buffered, batch/cache, wire/history/next request, cancellation/consumer stop, concurrency/race, numeric/schema and library boundaries.

## F1 — P1: failed/canceled native prefix остаётся доступным для continuation

Места: `adapter/ollama/stream.go:73–77`, `adapter/gemini/stream.go:52–63,81–88`, общий `stream.go:521–527`. Callback-specific r3 fix корректно отзывает continuation для malformed arguments без собственных частей, но не покрывает ошибки до callback, EOF и завершение consumer stop. Общий Stream closeout сохраняет false.

Executed reproduction: actual Ollama HTTP/NDJSON frame1 содержит known text и complete valid tool call с ID a, Done=false. Затем EOF, `{`, `{"error":"fixture"}` либо `{"message":[]}`. Checked native Status.Result содержит известные части, Outcome empty и ошибку, но ContinuationUnavailable=false. AssistantMessage сохраняет false. Следующий checked Execute из этой истории фактически делает второй HTTP запрос (fixture counter 1→2), в EOF варианте возвращает nil error. Лог `/tmp/task42-errors-r3-late-faults.log`, `TestAuditLatePreCallbackFaults` с race: 4 reproduced failing variants. Отдельный whole-malformed JSON сценарий reasoning/text/valid call: `/tmp/task42-errors-r3-ollama-probes.log`, `TestAuditLateWholeMalformedFrame`.

То же independently executed для Gemini actual SDK SSE: known text+complete valid call в первом event, затем EOF либо candidates:[null]. Content сохранён, Outcome empty, unavailable=false; next checked request dispatch 1→2. `/tmp/task42-errors-r3-gemini-faults.log`, `TestAuditNativePartialUnavailable/late_*`.

Consumer stop: существующий public actual HTTP `TestNativeEarlyTextBeforeServerCompletion` с дополнительной unavailable assertion в temp copy. После break Status.State canceled, known early text retained, unavailable=false. `/tmp/task42-errors-r3-consumer-stop.log`. Это тот же failure closeout class.

Нарушается обязательный partial exchange contract: неизвестный/неполный exchange должен сохранять known result с unavailable continuation, а история не должна разрешать replay. Различие host-owned storage/retries и library admission эту ошибку не устраняет.

## F2 — P1: Gemini native stream отбрасывает известный prefix внутри failed SSE event

Место: `adapter/gemini/stream.go:229–231`. parseGeminiResponse возвращает prefix+error, но ранний return nil,parseErr выбрасывает этот response до заполнения canonical frame.

Executed actual SDK SSE reproduction: один event `{"candidates":[{"content":{"parts":[{"text":"known"},{"functionResponse":{"name":"lookup","response":{}}}]},"finishReason":"STOP"}]}`. FunctionResponse в assistant output явно unsupported. Native checked stream возвращает ErrUnsupportedContentType и Result.Content=[]; известный preceding TextPart не сохраняется. `/tmp/task42-errors-r3-gemini-faults.log`, `TestAuditNativePartialUnavailable/same_event_unsupported`, race. Sync/direct parsing этого класса уже сохраняет preceding prefix и возвращает error/unavailable (existing TestGeminiMixedAndUnsupportedPartsFailClosed и код parseGeminiResponse), native путь должен соблюдать тот же контракт. Не требуется публиковать unsupported part или успешный finish — требуется сохранить уже распознанный text.

## Проверки остальных требований

- R2 original per-leaf source probes перенесены в temp copy и повторены с race: typed numeric graphs, точные Go integers/json.Number, nested numeric source binding, signed/unsigned source, независимые integer/native ID/name/signature mismatch rejection — pass. `/tmp/task42-errors-r3-gemini-probes.log`.
- Ollama original malformed args actual HTTP sync/buffered/native known prefix, duplicate argument keys, Execute→ParseResponse pair и consumer stop joined cancellation+malformed causes — pass. Исправленный r2 late []args without prefix native case также pass с unavailable=true. `/tmp/task42-errors-r3-ollama-probes.log` содержит эти pass и отдельный новый failing whole-JSON case.
- Просмотрены exact request encoding для sync/native/batch/file/cache, empty args {}, finite SDK range rejection, signed source/state fidelity и immutable native metadata binding. Source permission на float rounding применяется к соответствующему leaf, не к соседним exact fields. Новых confirmed numeric defects не найдено.
- Проверены native FunctionResponse.ID, output/error, correlation validators, автоматическая name-only capability requirement и mapping port; existing wire/history/actual SDK fixtures, scope/schema and incompatible-state rejection проходят. Новых confirmed correlation/result mapping defects не найдено.
- Рассмотрены reasoning prefix granularity и explicit interleaving rejection; OpenAI Chat/Ollama IsError reject, Anthropic native error mapping и OpenAI Responses existing refusal; unsupported multimodal shapes и host-owned DTO/transport/loop/storage границы. Дополнительных confirmed defects в этих mappings не найдено.

## Full checks

Чистая копия frozen r3 без audit probes:
- `make lint`, serial, все 12 модулей: exit 0, `/tmp/task42-errors-r3-lint-clean.log`.
- `make test`, все модули, race, generated consumers: exit 0, `/tmp/task42-errors-r3-test-clean.log`.
- Caches: GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache.

Первый full test в общей probe copy пересёкся с добавлением failing audit probes и завершился exit2; поэтому выполнен заново в отдельной чистой копии. Для baseline green evidence используется исключительно clean log. Failing probes не являются изменением implementation.

Live provider calls не выполнялись. Локальные actual SDK fixtures подтверждают проверенные IO-контракты, а не live model support. Отсутствие дополнительных найденных ошибок не является абсолютной гарантией. Для приёмки требуется устранить оба подтверждённых класса и повторить независимый полный audit нового frozen snapshot.
