# Task42 — независимая приёмка ошибок, round 2

**Решение: принято по проверке ошибок. Открытых подтверждённых ошибок: 0.** Это вывод выполненного полного review и executed probes, не абсолютная гарантия отсутствия любых дефектов. В implementation не участвовала; текущий peer r2 report не читала.

## Snapshot и independent checks

Frozen `/tmp/prompty-task42-snapshot-02`, база `d829ab2`. Manifest SHA256 **07d402cb92cbbda50ab55413de42f4235c31647427be992d6e84cc227771ca8a** совпал. Все **726 файлов** verified до/после аудита: `/tmp/task42-errors-r2-integrity.log`, `/tmp/task42-errors-r2-integrity-final.log`. Frozen/source не изменены; probes только `/tmp/prompty-task42-errors-probes-r2`.

Прочитаны task42 целиком, общий README, contracts, closeout/checks-r2 и публичный provider exchange contract. Проверен весь scope ingress/egress/state/correlation/error paths, не только r1 changes.

Независимо выполнены на frozen:

- `GOCACHE=/tmp/prompty-go-cache GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache make lint`: **exit 0**, все 12 modules, zero issues. `/tmp/task42-errors-r2-lint.log`.
- `GOCACHE=/tmp/prompty-go-cache make test`: **exit 0**, все modules/race/generated consumers. `/tmp/task42-errors-r2-test.log`.

Own adversarial fixtures в separate copy:

- Gemini `go test -race -run TestAudit -v .`: **exit 0**, `/tmp/task42-errors-r2-gemini-probes-expanded.log`.
- Ollama `go test -race -run TestAudit -v .` после проверки полного low-level Execute→ParseResponse protocol: **exit 0**, `/tmp/task42-errors-r2-ollama-probes-expanded.log`.
- Initial unchanged r1 replay лог `/tmp/task42-errors-r2-ollama-probes.log` содержит один failed Execute-only assertion о фазе ошибки; actual loss и checked assertions уже проходили. Причина и уточнённый oracle описаны ниже, failure не скрыт.

Probe sources: `adapter/gemini/audit_task42_r1_test.go`, `adapter/ollama/audit_task42_r1_test.go` внутри temp-copy. HTTP/SSE/JSONL fixtures local; live provider calls/conformance не утверждаются.

## R1 finding и наблюдение

**R1 F1 P1/actual Ollama HTTP known-prefix loss: закрыт.** Неизменённый public repro с valid thinking/text и malformed `arguments:[]` теперь во всех checked sync/buffered/native modes возвращает два known canonical parts плюс ErrMalformedArgs. Outcome и FinishReason unknown/empty, ContinuationUnavailable=true; malformed call не попадает в canonical Content. Buffered error-prefix имеет StreamDelta, native prefix также не объявляет finish. Replayed original checked assertions проходят.

**Low-level phase clarification.** Ранее самостоятельный Execute-only probe требовал immediate ErrMalformedArgs от adapter.Execute. R2 передаёт private partial SDK representation в следующий обязательный ParseResponse этап, чтобы generic checked transport-error branch не отбросил данные. Поэтому unchanged initial probe падает на прежнем phase expectation, а не на потере prefix. Независимо проверена полная пара **actual HTTP → Execute → ParseResponse**: raw содержит known prefix, ParseResponse возвращает canonical reasoning/text, parse error, unknown outcome и unavailable continuation; invalid internal SDK marker не публикуется как canonical tool call. Phase oracle уточнён явно, а behavioral requirement сохранён.

**R1 O1/duplicate argument keys.** Теперь adapter policy явно запрещает duplicate argument keys, включая nested и escaped equivalent keys до map conversion. Original duplicate-key HTTP probe больше не публикует Args={n:2} с success; получен explicit parse error с known prefix. Это ранее было observation с неустановленной global policy, теперь локальная documented policy и fixtures согласованы. Core global duplicate policy этим не объявляется.

**Consumer/cancellation error composition.** Дополнительный actual HTTP ChatClient probe получает malformed args frame, проверяет known reasoning/text и no Done/DoneReason, возвращает context.Canceled из consume. Callback вызван ровно один раз; returned error сохраняет **и ErrMalformedArgs, и context.Canceled** через errors.Is. Stop/cancellation не делает prefix успешным response и не запускает retry.

## Gemini source binding adversarial evidence

Собственные signed/unsigned per-leaf fixtures содержат nested exact int64 и соседний float64, который является законной SDK rounding projection того же original source number. Positive pair принимается. Независимо изменяются exact integer, call ID, name или signature при сохранении соседнего float — все conflicts отвергаются. Unrelated float больше не разрешает изменить exact source/native field. Signature проверяется независимо от Args. Лог `/tmp/task42-errors-r2-gemini-probes-expanded.log`.

Повторены r1 own typed graph probes: float32 slice, typed float64 map и pointer→struct→interface container без source reject; known preceding text остаётся partial с unknown outcome/continuation unavailable. Go uint64 max, negative int64 >2^53 и json.Number сохраняют объявленное exact JSON representation. Shared reflection guard обходит maps/slices/arrays/pointers/exported fields с visited identities, cyclic/unserializable args дают explicit error, не recursive panic. Custom JSON projections при несовпадающем exact representation не позволяют недоказанную per-field normalization.

## Весь проверенный scope

**Numeric IO.** Gemini signed и unsigned original source restoration применяются к sync, native/buffered SSE и existing batch inline/JSONL parse. Source ordinal/count/shape/signature checks сохранены. Exact source и SDK representation сравниваются по соответствующим numeric float leaves; остальные native fields независимо exact. Empty {} не исчезает из source/state/canonical/next request из-за SDK omitempty. Actual SDK fixtures сохраняют nested arrays/objects, positive/negative large integers, decimals/exponents, typed int64 handler values. No source precision guessing: direct floats reject, normal integer/json.Number path поддержан.

Final native contents encoding через existing SDK ExtrasRequestProvider работает в adapter-owned generation/streaming, inline/file batch и cache creation. Exact translated contents восстановлены после SDK intermediate conversion, host callback не может заменить их позднее в chain. Intermediate finite-range unsupported values дают explicit error; raw host SDK pipeline имеет отдельную ясно ограниченную гарантию. Cache digest не включает callback/options. Core provider-specific number/state types не добавлены.

**Correlation и history.** Gemini FunctionResponse.ID получает ToolCallID; repeated names с разными IDs и out-of-order supplied results сохраняют корректную native identity/order через wire→AssistantMessage→actual request. Duplicate IDs/results, unknown/changed-name results и неоднозначный ID-less/mixed history отвергаются. Checked profile admission автоматически выводит tool_name_correlation из ID-less history и проверяет profile support вместе с native mapping port; fabricated IDs/name-as-ID отсутствуют. Low-level mechanics не выдают checked model capability claims.

**Signed/partial state и errors.** Gemini required envelope/canonical binding/ordinal/signature/version и endpoint/model scope checks сохранены. Partial incremental calls provisional до completion; interleaved/missing/altered required state/cancel fixtures прошли. Accepted parts доступны с parse error, Outcome/Finish unknown, unavailable continuation. Batch source/download/correlation failure semantics и response wire сохраняются; retries/compensation не добавлены. Ollama malformed actual HTTP frame теперь сохраняет ordinary prefix и preceding valid calls; private invalid-call marker не входит в canonical output.

**Reasoning/results.** Ollama contiguous reasoning prefix превращается в Thinking отдельно от Content; adjacent reasoning ordered concat, later reasoning interleaving explicitly unsupported. Actual sync/buffered/native thinking→canonical→wire→next HTTP request fixtures прошли. Thinking не логируется автоматически; host capture policy отдельно. Gemini error/output native key и call IDs retained. OpenAI Chat/Ollama IsError rejects до dispatch; OpenAI Responses existing refusal и Anthropic native is_error сохранены. Value/pointer results проверены; неподдержанные multimodal forms явно reject, новые mappings task43 не добавлены.

**Boundary/regressions.** Полные adapter SDK HTTP/SSE/JSONL, state/schema/ordinal/profile/batch/cache/partial/correlation/cancellation/race fixtures и all generated consumers прошли независимый full run. Context/consumer stop не запускают новый handler/retry; available partial не превращается в finish. Host owns DTO/transport/auth/agent loop/storage/retention, core остаётся provider neutral. Несовместимые codecs/wire reject вместо legacy repair.

## Решение

R1 mandatory F1 закрыт original checked public repro и уточнённым complete low-level protocol probe. Новая source per-leaf correction и duplicate policy прошли adversarial проверки. В выполненном полном scope этой frozen версии других подтверждённых errors/regressions не обнаружено. Ошибок, блокирующих принятие task42 r2, нет.
