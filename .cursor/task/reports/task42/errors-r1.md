# Task42 — независимая приёмка ошибок, round 1

**Решение: не принято. Открытых подтверждённых blockers: 1 (P1, Ollama actual HTTP known-prefix retention).** Дополнительно зафиксировано воспроизводимое отличие обработки duplicate JSON keys; оно отделено от mandatory blocker, поскольку общий текущий JSON port не задаёт единый запрет duplicates.

В implementation не участвовала; текущий peer completeness report не читала. Source/frozen не менялись, probes только на отдельной копии.

## Версия и independent evidence

Frozen `/tmp/prompty-task42-snapshot-01`, база `d829ab2`. SHA256 manifest **6149d18e1a33a1064924a6b112ee27b2423d10c9d68b4ae9e2332fe078a75021** совпал, все **719 файлов** verified до/после аудита. Логи `/tmp/task42-errors-r1-integrity.log`, `/tmp/task42-errors-r1-integrity-final.log`.

Прочитаны task42, общий README, contracts, closeout/checks-r1 и публичный provider exchange contract. Проверен весь scope changed/existing paths, а не только новые fixtures.

Независимые full runs на frozen:

- `GOCACHE=/tmp/prompty-go-cache GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache make lint`: **exit 0**, все 12 modules, zero issues. `/tmp/task42-errors-r1-lint.log`.
- `GOCACHE=/tmp/prompty-go-cache make test`: **exit 0**, все modules/race/generated consumers. `/tmp/task42-errors-r1-test.log`.

Adversarial copy `/tmp/prompty-task42-errors-probes-r1`:

- `adapter/ollama/audit_task42_r1_test.go`: actual local HTTP duplicate arguments, low-level malformed arguments with known thinking/text, checked sync/buffered/native paths. Executed с race; `/tmp/task42-errors-r1-ollama-probes-allpaths.log`.
- Дополнительная проверка общего core duplicate-source поведения: `/tmp/task42-errors-r1-ollama-duplicate-core.log`.
- `adapter/gemini/audit_task42_r1_test.go`: recursive typed float graphs rejection с сохранением known prefix и positive uint64 max/int64 negative/json.Number exact cases. **Exit 0** с race; `/tmp/task42-errors-r1-gemini-probes.log`.

SDK HTTP/SSE/JSONL fixtures вошли в независимый full run. Они не являются live model conformance; live provider calls не выполнялись.

## F1 — P1: actual Ollama HTTP decoder теряет известные thinking/text при malformed tool args

**Места:** `adapter/ollama/chat_http.go:126–137`: ordinary SDK fields уже декодированы в `empty`, но при args error возвращается `api.ChatResponse{}`. `HTTPChatClient.Chat`, строки **75–81**, возвращает decodeErr до consume callback. Дополнительные sync boundaries `adapter/ollama/ollama.go:180–184` и `adapter/client.go:52–55` возвращают nil на dispatch error; fallback документация о raw/SDK path не восстанавливает потерянные данные.

**Executed reproduction:** valid JSON HTTP frame:

```json
{"message":{"role":"assistant","thinking":"known reasoning","content":"known text","tool_calls":[{"id":"call","function":{"name":"lookup","arguments":[]}}]},"done":true,"done_reason":"stop"}
```

Frame содержит доступные отдельные reasoning и text; `arguments:[]` не является поддержанным object. Decoder успевает прочитать ordinary fields, после чего tool decode error стирает эти данные. Данные не требуется угадывать или создавать синтетически.

- `TestAuditHTTPMalformedArgsKeepsKnownParts`: actual HTTP transport → low-level Execute: **raw=nil**, `ErrMalformedArgs`.
- `TestAuditCheckedHTTPMalformedArgsKnownPrefix`: actual HTTP через checked profile/client:
  - sync: **response=nil**, ErrMalformedArgs;
  - buffered: status.Result.Content **пуст**, ErrMalformedArgs;
  - native: status.Result.Content **пуст**, ErrMalformedArgs.

Команда `GOCACHE=/tmp/prompty-go-cache go test -race -run TestAudit -v .` в temp Ollama module. Assertions всех трёх public modes падают на отсутствии known prefix; `/tmp/task42-errors-r1-ollama-probes-allpaths.log`. Никаких handler/retry/live calls нет.

**Контракт:** task42 acceptance требует explicit error с сохранением доступного partial result; §7 применяет гарантию к существующим actual sync/native paths. Новая callback DTO fixture сохраняет тот же prefix через ParseResponse/ParseStreamChunk, но actual HTTP path этого не делает. Формулировка docs «errors before any accepted canonical part» не должна делать обязательную сохранность зависимой от внутреннего порядка decode→callback: известные fields уже доступны и успешно разобраны до отказа следующей части.

**Необходимая коррекция:** actual HTTP path должен передавать известный валидный prefix дальше вместе с decode failure. Checked sync/buffered/native входы должны вернуть этот canonical partial result с unknown outcome/finish, unavailable continuation и исходной причиной через errors.Is; malformed tool call не публиковать как завершённый. Не запускать retry, handler admission или synthetic repair.

## O1 — дополнительное наблюдение: Ollama duplicate keys silently collapse

**Места:** `adapter/ollama/tool_numbers.go:17–26`, Decode в map; также frame-level raw maps в chat_http.go. `TestAuditDuplicateSourceArguments` actual HTTP fixture `arguments:{"n":9007199254740993,"n":2}` публикует **Args={"n":2}**, outcome tool_calls, error nil. Gemini observed source guard rejects duplicate keys.

Это воспроизводимая потеря source member и различие adapter policies. Однако проверка показала, что текущие общие `ToolCallArgsForTranslate` и `SchemaDocumentAsMap` тоже принимают duplicate source JSON без ошибки; task42 не вводит явный отдельный global duplicate-key policy, а JSON semantic decoding может применять last-member behavior. Поэтому O1 **не учитывается как доказанный mandatory blocker**. Рекомендация: явно определить ambiguity policy и при требовании unique source fields reject duplicates до map conversion, включая nested/identity-bearing fields. Ранее промежуточная гипотеза «core отвергает duplicates» проверена и снята; этот отчёт содержит исправленный вывод.

## Проверенный полный scope

**Gemini numeric source/restoration.** Signed и unsigned calls проходят общий original source restoration с ordinal/part-count/signature/SDK-shape comparison. Exact caller DTO должен соответствовать source прямо; float-bearing SDK representation допускает проверенную SDK normalization. Unknown direct float precision отвергается рекурсивно во всех typed maps/slices/arrays/pointers/exported struct fields; visited container identities предотвращают cyclic recursion, json.Marshal errors остаются explicit. Собственные probes подтверждают prefix retention и positive точные Go integer/json.Number representations. Core provider-specific numeric types не получает.

Actual sync/native/buffered HTTP/SSE fixtures сохраняют large positive/negative integers, nested fractions/exponents и empty object; typed int64 handler получает точное число. Signed {} не исчезает через SDK omitempty/state/wire/continuation. Final exact contents encoding применяется через existing SDK hook к generation, batch inline/JSONL и cache creation. Host raw SDK pipeline имеет ясно отделённые guarantees. SDK intermediate finite-range rejection не заменяет normal exact IO path.

**State, correlation, scopes и batch.** Native call/result IDs переносимы через response wire/AssistantMessage/следующий actual request; repeated-name IDs и result order покрыты actual HTTP fixture. Gemini correlation отвергает duplicate/unknown/changed-name results и неоднозначный mixed/ID-less history. Checked preflight автоматически выводит name-correlation requirement и требует profile support плюс actual adapter mapping port. Low-level admission boundaries отделены от checked protocol guarantees, synthetic IDs не добавлены.

Signed Gemini state codec, ordinal/signature/canonical binding, endpoint/model profile scopes и required envelopes не ослаблены. Partial argument sequences remain provisional до completion; cancellation/interleaving/changed signature fixtures прошли. Unknown outcome и continuation unavailable не превращаются в completion. Existing inline/file batch parsing используют common source fidelity; malformed/source SDK failures остаются explicit, correlation/download отделены от transport completion. Cache exact content path и digest exclusion callback/options tested.

**Reasoning и result semantics.** Ollama contiguous reasoning prefix сохраняется в Thinking отдельно от Content; adjacent reasoning concatenates, later reasoning interleaving explicitly unsupported. Actual sync/buffered/native thinking→wire→next HTTP fixtures прошли. Direct typed-float/cycle SDK callbacks сохраняют accepted prefix с ошибкой и без finish; actual malformed HTTP mismatch — F1 выше. Thinking не логируется автоматически, host capture policies независимы.

Gemini IsError mapping native error/output и FunctionResponse.ID retained. OpenAI Chat/Ollama IsError rejects до dispatch; OpenAI Responses explicit rejection сохранён, Anthropic native is_error unchanged. Existing unsupported multimodal result shapes явно отвергаются, новые mappings task43 не добавлены. Pointer/value tool result forms покрыты suites. SDK request/callback/cancellation/race/schema/public-boundary checks проходили; no retry/agent loop/storage framework появился.

## Решение

Независимые full suites зелёные, но actual public Ollama malformed-frame retention probe подтверждает F1. Требуется исправить этот путь и повторить независимую приёмку нового frozen snapshot. Других обязательных подтверждённых blockers в выполненном scope не найдено; O1 оставлен отдельно как доказанное наблюдение с неустановленной mandatory policy. Вывод не является абсолютной гарантией отсутствия других дефектов.
