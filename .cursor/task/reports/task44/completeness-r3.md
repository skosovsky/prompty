# Task44 — независимый аудит полноты r3

Решение: **принимать по полноте**. **19 / 19 = 100%** mandatory scope выполнено. Частичных и отсутствующих требований не обнаружила. Это результат независимого аудита конкретного snapshot, а не математическая гарантия отсутствия ошибок; отдельная error-приёмка остаётся необходимой.

## Объект и метод

База `59402ca`; immutable manifest `snapshot-r3.json`, SHA256 `880409f876fd69daaf6b58381de6b653e6e44b4e886171c92c2e91913d8e5f6f`. Все **761** hashes сверены до копирования и после аудита; совпадают. Исследование и probes только в `/tmp/prompty-task44-audit-r3`. Implementation не меняла, в её написании не участвовала, peer r3 report не читала.

Denominator неизменен: C1–C10 prose requirements, A1–A8 acceptance, M1 общий closeout/границы README. Полный original task44 и общий README перечитаны, contracts/код/fixtures сверены. Hash delta к r2 — только contracts, OpenAI source-model replay fixtures/два parsers и error tree classifier/fixtures. Остальная поверхность повторно оценена по неизменному frozen коду и свежему проходящему полному набору тестов; scope не сводился к двум исправлениям. Root/adapter docs, matrix/migration, default privacy, caller tracer, explicit resource ports и core lifecycle сохраняют оригинальные требования.

## Матрица

| ID | Полное обязательство | Код / тест / evidence | Статус |
|---|---|---|---|
| C1 | Официальная закреплённая версия/status conventions, стандартные и extension attrs | `telemetry.go:SemanticConventionsRevision`, extension README/contracts; официальный Development snapshot `4f85037...`; стандартные operation/provider/model/finish/usage, extension total/resource/lifecycle документированы | complete |
| C2 | Достоверные request/response model/provider/operation/outcome/finish и presence-aware usage без повторного суммирования | `telemetry.go:normalizeObservation/usageAttributes`; response/clone/wire/stream Model; provider mapper fields. Known zero/unknown tests проходят. observedModel/observedMessageModel проверяют genuine unique source string; actual SDK HTTP/SSE/batch model_source/response_model fixtures и собственные source/replay probes проходят; malformed source-derived history ContinuationUnavailable сохраняется через wire | complete |
| C3 | Response+error metadata и отдельная классификация preflight/transport/refusal/incomplete/cancel/stop | sync defer/finish и stream lifecycle сохраняют Response+Err; tests capture/schema/privacy. classifyError проверяет known protocol/transport и unknown error до outcome; onlyOutcomeCauses проверяет каждую leaf matching typed outcome; собственные retained transport/unknown/protocol/cancel/joined independent cause probes и exporter regression проходят | complete |
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



## Independent adversarial evidence

- Собственный предыдущий `TestAuditTransportWithPartialOutcome` проходит: retained incomplete outcome остаётся metadata, *net.DNSError даёт transport_failure и OTel Error.
- Собственный `TestAuditOutcomeSeparateFromFailures` проходит для refusal/incomplete вместе с unrelated error, protocol failure и cancellation.
- Новый собственный `TestAuditJoinedIndependentCause` проходит: errors.Join(typed matching OutcomeError, independent secret error) экспортирует operation_failure и Error. Наличие matching outcome в одной branch больше не скрывает другую. Reviewed `onlyOutcomeCauses` проходит каждую leaf, отличает pure wrapped/joined matching outcomes от mixed/mismatched/sentinel/unknown leaves и имеет conservative depth limit. Существующий exporter fixture покрывает вложенные wrapped joins, ErrUsage, capture/schema/protocol/transport, nil/pure outcome и сохраняет known-zero counters/metadata без error text.
- Предыдущий own actual SDK `TestAuditInvalidSourceModel` проходит: JSON model:123 не становится trusted canonical Model.
- Новый собственный `TestAuditMalformedSourceCannotReplay` проходит: source-derived error response имеет ContinuationUnavailable; после portable wire round-trip/AssistantMessage reuse checked execution отвергается ErrStateUnavailable до IO даже с caller profile, разрешающим provider state. HTTP counter остаётся 0.
- Reviewed actual SDK HTTP/SSE/batch source fixtures дополнительно проверяют scalar/null/object/array/duplicates, prefix retention, valid model, malformed-source wire/history и zero second dispatch для Chat и Responses, а также malformed batch item. Anthropic HTTP/shared batch/native source fixtures продолжают проходить. Response handle ID/status остаются известными фактами: запрет source-history replay не означает отзыв independently verified remote handle или неявный remote cancellation. Explicit Previous-only операция — отдельный host выбор, это не обход входной проверки malformed history.
- Responses Get/Cancel сохраняют model/outcome/finish и error вместе с resource status; SuppressGenerationUsage не стирает metadata и не повторяет counters. Batch/cache не создают synthetic generation spans или item billing из resource downloads.

Own probes: `ext/otelprompty/audit_probe_test.go`, `adapter/openai/audit_model_probe_test.go` в isolated copy, `/tmp/task44-own-probes-r3.log`, race exit 0.

## Проверки

Независимый `go test -race ./... ./ext/otelprompty/... ./adapter/openai/... ./adapter/anthropic/... ./adapter/gemini/... ./adapter/ollama/... ./cmd/prompty-gen/testdata/gen_out` — **exit 0**, `/tmp/task44-completeness-tests-r3.log`. Покрывает core/stream/wire, четыре SDK адаптера, exporter, examples и generated consumer. Все 82 ссылки matrix существуют; положительные API/mapping claims сверены с source и fixtures, включая native/buffered и checked/raw границы.

Parent full all12 `make lint`/`make test` (+generated race) — проверены логи `checks/full-lint-r4.log`, `checks/full-test-r4.log`: lint 0 issues, test successful. Собственный повтор all12 lint не запускала; остальные optional modules покрыты parent checks. Предел независимого тестового набора указан, не выдаётся за all12.

Официальный pinned GenAI Development snapshot `4f85037ef86e92c510d2ef881a58f1076f6fc0e4` сверялся в r1 этого дня: [client inference](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md), [operation spans](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Pin и mappings неизменны. Future stable compatibility не обещается.

## Границы и findings

Новых confirmed findings не обнаружила. Live provider/model/account проверки не выполнялись; локальные SDK fixtures явно не представлены как live conformance. Metadata сохраняет различия known/zero, observed/request model, partial/completed, resource/generation, logical/physical, native/buffered и checked/raw. Core остаётся BYOT; OTel dependency и SDK mapping в optional modules. Host владеет exporter, credentials, retention/projection/redaction и policies. Нет backend/dashboard/storage/billing/retry/loop/secret scanner/необоснованных quality metrics. Migration38–43 и historic task37 boundary не переписаны ради приёмки. Финальный accepted/commit closeout должен фиксироваться только после обеих independent acceptances.
