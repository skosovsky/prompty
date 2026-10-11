# Task44 — независимый аудит полноты r1

Решение: **не принимать**. Полностью выполнено **17 / 19 = 89.47%**. C2 и C3 выполнены частично; частичные требования не засчитаны. Это результат проверки данного snapshot, а не математическая гарантия отсутствия ошибок.

## Объект и метод

База `59402ca`; manifest `snapshot-r1.json`, SHA256 `009bc30132932a54064010c8dff38fec07089a67ff29f5aa26cbbe3521150c4a`. Все 757 файлов проверены до копирования и после аудита; изменений нет. Исследовалась отдельная копия `/tmp/prompty-task44-audit-r1`. Implementation не изменяла, peer report не читала, в реализации не участвовала.

Denominator: 10 требований раздела «Требования», 8 acceptance, 1 общий обязательный контракт closeout и границ из README. Требования не ограничивались checkbox. Финальное подтверждение acceptance/commit логично остаётся pending; документы об изменённых контрактах, миграции, проверках и ограничениях оцениваются отдельно от статуса принятия.

## Матрица

| ID | Полное обязательство | Код / тест / evidence | Статус |
|---|---|---|---|
| C1 | Официальная закреплённая версия/status conventions, стандартные и extension attrs | `telemetry.go:SemanticConventionsRevision`, extension README/contracts; официальный Development snapshot `4f85037...`; стандартные operation/provider/model/finish/usage, extension total/resource/lifecycle документированы | complete |
| C2 | Достоверные request/response model/provider/operation/outcome/finish и presence-aware usage без повторного суммирования | `telemetry.go:normalizeObservation/usageAttributes`; response/clone/wire/stream Model; provider mapper fields. Known zero/unknown tests проходят. **F2**: source numeric model превращается SDK в строку и принимается как observed model | **partial** |
| C3 | Response+error metadata и отдельная классификация preflight/transport/refusal/incomplete/cancel/stop | sync defer/finish и stream lifecycle сохраняют Response+Err; tests capture/schema/privacy. **F1**: retained incomplete/refusal outcome перекрывает известную net.Error, транспортный failure становится provider outcome и Unset status | **partial** |
| C4 | Logical operation не physical attempts, без invented retry/double count | `prompty.observation.scope=logical`, extension README; singleton finish; отдельные SDK spans не агрегируются; нет retry/attempt counter | complete |
| C5 | Caller tracer/provider, OTel только optional extension | WithTracer/WithTracerProvider/resolveConfig и тест owned instrumentation scope; зависимость изолирована ext module | complete |
| C6 | Default privacy, host opt-in projection, safe categories | request/result attrs не включают payload/URL/IDs/error text; status пустой; no RecordError/events; projection owned clone. Privacy и projection ownership exporter tests | complete |
| C7 | Lazy actual lifecycle, once completion, unused/Close/start/cancel/stop | Original handle ObserveLifecycle; Stream.Close/StopReason; TestTracingLazyStreamExactlyOneSpanAndOriginalHandle все 7 scenarios; identity/finalizer tests | complete |
| C8 | Явные Responses/batch/cache ports, resource vs generation/item | operations.go WrapRemoteResponses/WrapResponseStreaming/WrapBatch/WrapCache; resource observations без повторных generation usage; retained result+error и lazy native fixtures | complete |
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
| M1 | Общие границы/closeout: BYOT, core/vendor separation, checks/migration/limits | contracts.md + public docs/extension README + progress/checks reports; core additions minimal metadata/lifecycle; no backend/metrics pricing/storage/secret scanner/loop/retry. Host exporter/policy/retention explicitly owned | complete |

## Подтверждённые findings

**F1 — C3: ошибка transport скрывается outcome retained response.** `ext/otelprompty/telemetry.go:272–284` возвращает provider_incomplete/provider_refusal раньше проверки net.Error. Probe `ext/otelprompty/audit_probe_test.go:TestAuditTransportWithPartialOutcome` в отдельной копии: custom Invoker возвращает исходный Response{OutcomeIncomplete} и *net.DNSError. Исходные result/error действительно сохранены; outcome attr=incomplete. Но category=provider_incomplete вместо transport_failure и status=Unset вместо Error. Outcome должен оставаться отдельным фактом; наличие известной транспортной ошибки не теряется из-за partial результата. Это воспроизводится без payload telemetry и не требует vendor error guessing.

**F2 — C2: неподтверждённая source model публикуется как observed.** `adapter/openai/openai.go:537` напрямую читает completion.Model. Probe `adapter/openai/audit_model_probe_test.go:TestAuditInvalidSourceModel`: actual SDK ChatCompletion.UnmarshalJSON принимает JSON model:123, SDK поле становится string "123"; ParseResponse возвращает Model="123" и nil error. Требуемая provider-reported model не должна возникать через raw scalar coercion. Probe требует rejection и отсутствия fabricated Model; падает. Тест использует actual SDK decode, а не искусственный DTO.

## Проверки и пределы

Независимо выполнено `go test -race ./... ./ext/otelprompty/... ./adapter/openai/... ./adapter/anthropic/... ./adapter/gemini/... ./adapter/ollama/... ./cmd/prompty-gen/testdata/gen_out`: exit 0; `/tmp/task44-completeness-tests-r1.log`. Два adversarial probes выше падают на неизменённой copied реализации. Полные parent make lint/test логи проверок доступны в reports/task44/checks; они не заменяют независимые probes. Остальные optional workspace modules покрыты parent all12 checks; собственного повторного lint all12 не выполняла. Live providers не проверялись.

Pinned официальные conventions сверены с [client inference](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md) и [operation spans](https://github.com/open-telemetry/semantic-conventions-genai/blob/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Development status не объявлен stable или универсальным future contract.

Для приёмки требуется новый snapshot после исправлений и повторная полная независимая проверка. Текущий snapshot не достигает 100%.
