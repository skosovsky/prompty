# Task44 — независимый аудит полноты r2

Решение: **принимать по полноте**. Полностью выполнено **19 / 19 = 100%**. Частичных или отсутствующих обязательств не обнаружила. Результат относится к проверенному snapshot, не является математической гарантией отсутствия ошибок и не заменяет отдельную приёмку ошибок.

## Объект и метод

База `59402ca`; manifest `snapshot-r2.json`, SHA256 `d5bfab48e77c1d41d23e486091141e823b29b3c7da8dffe94774cbe6a999aeb6`. Все **761** file hashes проверены до копирования и после аудита: совпадают. Реализацию исследовала в `/tmp/prompty-task44-audit-r2`, probes добавлены только в эту копию. В implementation не участвовала, peer r2 report не читала, frozen/source не изменяла.

Полный original scope сохранён: C1–C10 (все prose requirements), A1–A8, M1 общий closeout и границы README. Denominator **19**, без исключения inconvenient требований. Перечитаны original task44/README, contracts и актуальные public docs. Документы capability matrix/migration/root README идентичны r1; повторно сверены с кодом и проходящими provider fixtures, а не автоматически приняты по прежнему проценту. Acceptance/commit в progress остаются pending до двух независимых решений — это не ложное утверждение о принятии.

## Матрица

| ID | Полное обязательство | Код / тест / evidence | Статус |
|---|---|---|---|
| C1 | Официальная закреплённая версия/status conventions, стандартные и extension attrs | `telemetry.go:SemanticConventionsRevision`, extension README/contracts; официальный Development snapshot `4f85037...`; стандартные operation/provider/model/finish/usage, extension total/resource/lifecycle документированы | complete |
| C2 | Достоверные request/response model/provider/operation/outcome/finish и presence-aware usage без повторного суммирования | `telemetry.go:normalizeObservation/usageAttributes`; response/clone/wire/stream Model; provider mapper fields. Known zero/unknown tests проходят. observedModel/observedMessageModel проверяют genuine unique source string; actual SDK HTTP/SSE/batch model_source/response_model fixtures и собственный r1 probe проходят | complete |
| C3 | Response+error metadata и отдельная классификация preflight/transport/refusal/incomplete/cancel/stop | sync defer/finish и stream lifecycle сохраняют Response+Err; tests capture/schema/privacy. classifyError проверяет known protocol/transport и unknown error до outcome; собственный retained transport probe + OutcomeSeparateFromFailures и exporter regression проходят | complete |
| C4 | Logical operation не physical attempts, без invented retry/double count | `prompty.observation.scope=logical`, extension README; singleton finish; отдельные SDK spans не агрегируются; нет retry/attempt counter | complete |
| C5 | Caller tracer/provider, OTel только optional extension | WithTracer/WithTracerProvider/resolveConfig и тест owned instrumentation scope; зависимость изолирована ext module | complete |
| C6 | Default privacy, host opt-in projection, safe categories | request/result attrs не включают payload/URL/IDs/error text; status пустой; no RecordError/events; projection owned clone. Privacy и projection ownership exporter tests | complete |
| C7 | Lazy actual lifecycle, once completion, unused/Close/start/cancel/stop | Original handle ObserveLifecycle; Stream.Close/StopReason; TestTracingLazyStreamExactlyOneSpanAndOriginalHandle все 7 scenarios; identity/finalizer tests | complete |
| C8 | Явные Responses/batch/cache ports, resource vs generation/item | operations.go WrapRemoteResponses/WrapResponseStreaming/WrapBatch/WrapCache; resource observations без повторных generation usage; retained result+error и lazy native fixtures; Get/Cancel имеют Response metadata + SuppressGenerationUsage, отдельный exporter fixture | complete |
| C9 | Единая полная capability matrix, code/fixtures, unknown/unsupported/profile | docs/provider-capability-matrix.md: Chat/Responses operations/native/hosted/batch, Anthropic native/synthetic/hosted/batch, Gemini обе backend/media/partial/batch/cache, Ollama/cache; 82 links существуют; mappings и fixtures сверены. Checked/raw/native/buffered/live distinctions явны | complete |
| C10 | Root/adapter README/godoc/examples/migration38–43, historic checkpoints | root+4 adapter README/doc.go, extension package godoc/README/examples; docs/migration.md; current matrix separate from historical task37, stale local-tools statements удалены | complete |
| A1 | Exporter sync/stream knownzero/unknown/cache/reasoning/effective model | TestTracingKnownMetadataSurvivesErrorAndDefaultPrivacy, UnknownCountersAndNoFabricatedResponseModel, LazyStreamExactlyOneSpanAndOriginalHandle; exporter asserts exact attrs | complete |
| A2 | Response+error usage/outcome retained; preflight не successful generation | retained response/error tests и preflight categories; original result/error identity сохранена, failure status; known usage остаётся | complete |
| A3 | Cancel/stop/Close/terminal once, unused no fictitious IO duration | 7 lifecycle scenarios; untouched handle observer; no span on unused/Close before consumption, once closeout | complete |
| A4 | Default content/tool/reasoning/state/error secrets absent, explicit projection works | in-memory exporter privacy assertions attrs/events/status; projection mutates owned snapshots и не core DTO | complete |
| A5 | Responses/batch/cache runnable examples и no double usage | operations examples + tests resource-only Get/Cancel/batch/cache vs Create usage; original native lazy observation | complete |
| A6 | Каждый positive matrix пункт с code/fixture, no implicit model/live claim | 17 operation/API rows inspected with linked source/test; full adapter fixture suite passes; matrix explicitly offline, profile-owned support, no live verification | complete |
| A7 | README consistency; examples compile/use checked guarantees | root/adapter README crosschecked; examples executed within independent module race suite; raw examples отдельно обозначены raw surface | complete |
| A8 | Clear-break38–43 migration, no compat/ignore-error advice | docs/migration.md covers schema/codegen/toolscope/context/snapshot/truncate/fidelity/hosted/paused/strict/scopedannotations; explicit removed APIs and rejection, no bridge | complete |
| M1 | Общие границы/closeout: BYOT, core/vendor separation, checks/migration/limits | contracts.md (включая correction invariants) + public docs/extension README + progress/checks reports; core additions minimal metadata/lifecycle; no backend/metrics pricing/storage/secret scanner/loop/retry. Host exporter/policy/retention explicitly owned | complete |


## Независимое воспроизведение и исправления r1

1. `TestAuditTransportWithPartialOutcome`: исходный Response{OutcomeIncomplete} и *net.DNSError сохраняются по identity; outcome=incomplete, error category=transport_failure, OTel status=Error. Проходит. Дополнительно `TestAuditOutcomeSeparateFromFailures` проверяет refusal/incomplete с неизвестной ошибкой, protocol error и context cancellation: категории не маскируются outcome. В reviewed implementation typed error precedence стоит перед provider outcome; неизвестная ошибка не объявляется transport автоматически.
2. `TestAuditInvalidSourceModel`: actual SDK ChatCompletion.UnmarshalJSON для model:123 по-прежнему даёт удобную SDK строку "123", но adapter ParseResponse теперь возвращает error и пустой canonical Model. Проходит. Новые actual SDK HTTP/SSE fixtures проверяют number/bool/null/object/array/duplicate model и valid/absent/raw-free typed cases. Anthropic shared parser покрыт HTTP и batch item, native message_start genuine source string guard; OpenAI Chat и Responses используют source validator. Сохранён independently verified partial content/usage без fabricated Model.
3. Responses Get/Cancel проверены отдельно: response model/outcome/finish сохраняются вместе с resource status и capture error, а все generation token attrs отсутствуют. `SuppressGenerationUsage` подавляет только usage, не весь Response. Create и actual native stream продолжают записывать generation usage; batch/cache resource counters не выдаются за model invocation billing.

Оба прежних собственных probes находятся в копии под `ext/otelprompty/audit_probe_test.go` и `adapter/openai/audit_model_probe_test.go`; дополнительный failure probe в первом файле. Лог `/tmp/task44-own-probes-r2.log`, exit 0 с race.

## Проверки и границы вывода

Независимый полный набор core + четырёх adapter modules + telemetry extension + generated consumer: `go test -race ./... ./ext/otelprompty/... ./adapter/openai/... ./adapter/anthropic/... ./adapter/gemini/... ./adapter/ollama/... ./cmd/prompty-gen/testdata/gen_out`, **exit 0**, `/tmp/task44-completeness-tests-r2.log`. Он включает examples, lifecycle/cancel/Close/privacy/projection/resource/model actual HTTP/SSE/batch fixtures. Все 82 ссылок unified matrix разрешаются в существующий code/test/documentation.

Parent `make lint` / `make test` all12 modules + generated race logs `checks/full-lint-r3.log`, `checks/full-test-r3.log` проверены: lint завершён 0 issues, test успешен. Собственный повтор all12 lint не запускала. Остальные optional modules покрыты parent checks, новых реализационных changes в них task44 не вводит.

Pinned официальные GenAI conventions в Development статусе сверены в r1 текущего дня: [client inference](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md), [operation spans](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Pin/status/standard mapping в r2 неизменны; universal future compatibility не заявляется.

Live provider/model/account conformance не проверялась и не приписывается offline evidence. Core остаётся BYOT; OTel/SDK зависимости остаются optional modules. Logical span не физический HTTP attempt. Host владеет exporter, projection/redaction, policy и retention; библиотека не добавляет backend, billing, secret scanner, retry, scheduler или agent-quality metrics. Исторический task37 checkpoint не переписан.
