# Task43 — independent errors audit r2

Decision: **ACCEPTED** по критерию ошибок. **0 открытых подтверждённых ошибок** в проверенном полном scope. Это не абсолютная гарантия отсутствия дефектов.

## Snapshot and independence

Frozen workspace manifest `.cursor/task/reports/task43/snapshot-r2.json`, SHA256 `fba7ed2e47262acd7ad3f876cc02fcc469c67c50d420152adc33b2fff0315d3b`, 805 файлов, base `132afc221c7db0f69088ce0b341f4f779b4be8ce`. Identity и каждый file hash проверены до и после: bad=[]; `/tmp/task43-errors-r2-integrity.log`, `/tmp/task43-errors-r2-integrity-final.log`.

Применён весь original scope task43, общий README заданий, contracts и provider capability matrix. Проверка не ограничена исправлениями r1. Повторно оценены прежние conclusions относительно текущего frozen code; изменённые source-validation/state-machine paths прочитаны непосредственно. Текущий peer report не читала. Implementation не делегировала и не меняла. Clean checks copy `/tmp/prompty-task43-errors-checks-r2`, probes copy `/tmp/prompty-task43-errors-probes-r2`; копии собраны только из manifest-listed hash-verified files. Дополнительные test files есть только в probes copy.

## R1 blockers independently rechecked

**F1, Responses summary index panic — closed.** `responseSSEItem.validSummaryIndex` требует declared summary part, presence, nonnegative index, current association и bounds всех трёх arrays до indexing. Исходный actualSDK public HTTP/SSE reproducer для delta/text.done/part.done с summary_index=-1 теперь explicit error, без panic. Независимое расширение: 27 сценариев (три event kinds × -1, 0, 1, maxInt64, int64 overflow, null, bool, quoted integer, fraction) до объявления summary part. Все отвергнуты без panic с race. Logs `/tmp/task43-errors-r2-openai-probes.log`, `/tmp/task43-errors-r2-openai-adjacent.log`.

**F2, Anthropic malformed delta coercion/loss — closed.** Whole raw event проверяется на recursive duplicate keys и exact consumed source types до state mutation. Индекс должен быть source integer и совпадать с SDK index; delta type и mandatory text/partial_json/thinking/signature source fields должны быть настоящими JSON strings. Дополнительные guards покрывают message/block objects, identity fields, usage counters и stop reason. Неизвестные семантические варианты продолжают explicit refusal в state machine; дополнительные optional provider fields не закрыты случайным whitelist.

Исходные actualSDK public native text probes (null, missing, number, duplicate) и независимые соседние bool/object/array варианты теперь возвращают error; каждый сохраняет ровно text="known", ContinuationUnavailable=true, без successful finish и без SDK-normalized suffix. Logs `/tmp/task43-errors-r2-anthropic-probes.log`, `/tmp/task43-errors-r2-anthropic-adjacent.log`. Расширенные assertions проверяли content и unavailable, а не только наличие ошибки.

**Adjacent source boundaries.** Reviewed Responses raw scalar guards до state transitions: genuine strings, source int64 indices/sequence, nested consumed response/item/part/output/content/summary shapes; opaque reasoning content остаётся под codec validation. Duplicate keys отвергаются recursively. Reviewed Anthropic text/input JSON/thinking/signature/citation/index/usage/message and hosted/private block validation, source order и failure prefix. Полный независимый race suite исполнил новые actualSDK source-type fixtures, positive empty-string/extra-optional-field cases и существующие mixed streams; guards не требуют закрытого общего provider envelope. Подтверждённых adjacent regressions не обнаружено.

## Full original mandatory scope

**Responses controls and strict.** Endpoint-specific effort/summary whitelist и caller profile schema/capability gates сохранены; summary не включает encrypted output сам по себе. Strict — явный opt-in для local tools, исходный closed/required subset без скрытого lowering; original argument schema checks остаются. Get/Cancel/background проходят отдельные operation gates. Caller управляет модельной поддержкой, библиотека не делает discovery.

**Responses native stream.** Проверены lazy foreground CreateStream, single-use/Close/body cancellation, отсутствие Get polling/sync fallback/reconnect, handle ownership/retention, sequence/item/output/content correlations, summaries/encrypted state, delta/done/terminal consistency, hosted output/citations, late usage и finalization. Invalid source не меняет опубликованный verified prefix. Native protocol/transport/cancel/capture failures не превращаются в successful replay; known remote handle сохраняется. Conformance, mixed source, cancellation/concurrent close/isolation fixtures прошли в independent full race run.

**Ollama Format/Think.** Original schema переносится в native Format без lowering и проверяется на final output; native JSON остаётся provisional. Think top-level bool/null/allowlisted string проверяется effective ModelOptionsSchema/reasoning capability без model inference. Thinking отделён от JSON text. Existing actualSDK sync/native original-schema, controls и task42 partial-failure regressions проходят.

**Gemini function-result media.** Проверены capability/InputMIME/backend gates, inline supported image MIME, Vertex-only strict gs URI forms, byte ownership, source ID/name/content order, text/ref output и ordered native image parts. Несовместимые формы отвергаются, text-only encoding сохранён. Библиотека не выполняет media IO/download/upload. Sync/native/batch/cache fidelity и exact-number/signed-state task42 suite прошли.

**Anthropic hosted and pause.** Versioned declarations/settings остаются закрытым поддержанным subset; local collisions/unknown settings/unsupported types fail before IO. Hosted source call/result/error/citation/document order и linkage сохраняются как required scoped annotations, без превращения в local calls. Source-proven prefix публикации и signed/redacted reasoning associations проверены по state machine и actualSDK fixtures. OutcomePaused означает завершённый transport control outcome; typed helpers не выдают успешный DTO и не вызывают local handlers. Resume принадлежит caller. Sync/native/batch/history/wire scenarios и error/cancellation prefix tests прошли.

**Core state, finalization and capture.** Required scoped annotations глубоко owned, wire marker/schema/version и exact scope/profile/kind gates сохранены; StateOmit удаляет scoped metadata и запрещает required continuation, optional unscoped host metadata сохраняется. Conservative truncation dependencies остаются. StreamPartFinalization проверяет content/type/policy/arguments, атомарно работает на cloned candidate, remaps associations и отвергает unknown/repeated/sealed updates. Core stream error/cancel closeout revokes retained continuation. Capture отражает semantic source и scoped metadata; redaction/projector/sink failure не теряют уже известный handle и не подменяют provider completion. Full race tests подтверждают concurrent isolation/cancellation/Close.

**Public boundary/migration/docs.** Canonical HostedTools остаётся portable intent с vendor mapping в adapters; SDK DTOs не проникли в core. Existing tool scopes/schema binding/required declarations preflight не ослаблены. Offline Examples и generated consumers выполнены full test. Матрица разделяет implemented mapping, pinned local SDK conformance и caller/live capabilities. Новые agent loop/storage/browser/sandbox/session manager/retry/download responsibilities не добавлены.

## Independent checks and limits

- Clean-copy `make lint`, serial, all 12 modules: exit0; `/tmp/task43-errors-r2-lint.log`.
- Clean-copy `make test`, all 12 modules/race/generated consumers/Examples: exit0; `/tmp/task43-errors-r2-test.log`.
- R1 public actualSDK adversarial tests повторены с race: оба exit0; probes logs выше.
- Дополнительные own public actualSDK adjacent matrices с race: оба exit0; adjacent logs выше. Первый запуск расширенного OpenAI probe имел синтаксическую ошибку только во внешнем test file; исправленный повторный запуск завершился exit0. Implementation при этом не менялась.
- GOCACHE=/tmp/prompty-go-cache; GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache.
- Snapshot identity/all hashes неизменны после checks/probes.

Live provider/account tests не выполнялись. Fixtures доказывают проверенные SDK/HTTP/SSE contracts, а не поддержку каждого live model. Новых подтверждённых findings в mandatory scope не найдено; два P1 r1 independently closed.
