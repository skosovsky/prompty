# Task44 — independent full errors audit r1

Decision: **REJECTED**. **3 открытых подтверждённых P2 класса ошибок**. Full green lint/test не отменяют executed adversarial failures.

## Snapshot, independence, checks

Immutable manifest `.cursor/task/reports/task44/snapshot-r1.json`, SHA256 `009bc30132932a54064010c8dff38fec07089a67ff29f5aa26cbbe3521150c4a`, 757 files, base59402ca. Manifest identity и все hashes проверены до/после: bad=[]. Logs `/tmp/task44-errors-r1-integrity.log`, `/tmp/task44-errors-r1-integrity-final.log`.

Проверен весь original task44 scope, общий README заданий/contracts, новый core model/wire/stream contract, mappings всех четырёх providers и Responses, extension sync/stream и explicit resource wrappers, examples/migration/capability matrix. Peer current report не читала; implementation не делегировала/не меняла; commits не создавала. Clean checks `/tmp/prompty-task44-errors-checks-r1`, own probes `/tmp/prompty-task44-errors-probes-r1` собраны из manifest-listed verified files. Дополнительные test files только в probes copy.

Independent full `make lint` all12 serial: exit0 `/tmp/task44-errors-r1-lint.log`. Independent full `make test` all12/race/generated consumers/Examples: exit0 `/tmp/task44-errors-r1-test.log`. GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache. Actual SDK/in-memory own probes описаны отдельно; их failing assertions — воспроизведения ошибок, не failure full suite.

## F1 — P2: malformed raw model становится достоверным observed model через SDK coercion

Confirmed locations:

- `adapter/openai/openai.go:537` Chat sync ParseResponse;
- `adapter/openai/openai.go:663` Chat native StreamEvents;
- `adapter/anthropic/anthropic.go:748` Messages sync ParseResponse;
- `adapter/openai/responses_parse.go:50` Responses Create parsing; preceding SDK Model.Valid/equality check на33–35 не обеспечивает source string type.

New Model mappings берут SDK string value без подтверждения исходного типа. Actual SDK normalizes raw JSON number/bool к строке. Новый portable Response.Model объявлен provider-reported observed metadata; malformed source не даёт такого знания.

Executed public checked HTTP reproductions:

1. OpenAI Chat valid one-choice completion/text/usage, raw model:42 или model:true. `adapter.NewClient(...).Execute` возвращает error=nil, OutcomeCompleted, Model="42"/"true", continuation available. Positive source string="observed" сохраняется правильно; null становится empty/unobserved.
2. Same checked Chat native SSE: initial content="known" with raw model:42/true, valid stop frame and [DONE]. StreamCompleted, error=nil, Model="42"/"true". Logs `/tmp/task44-errors-r1-model-probe.log`, `/tmp/task44-errors-r1-native-model-probe.log`.
3. Anthropic valid message/text/end_turn/usage, raw model42/true. Checked Execute succeeds with normalized Model strings. `/tmp/task44-errors-r1-anthropic-model-probe.log`.
4. Responses caller/profile model="42", valid completed output, raw model numeric42. ActualSDK checked Create succeeds; SDK Valid/equality accepts coerced string42 and response.Model="42". `/tmp/task44-errors-r1-responses-model-probe.log`.

Same parsers reused by buffered/batch/retrieval paths are adjacent affected code paths; отдельное executed evidence выше относится к перечисленным public sync/native/Create operations. Не утверждается исполнение всех комбинаций того же parser. Required fix boundary: malformed source model должен давать typed explicit source/protocol error, без публикации coerced model. Already verified prefix/known model/usage/handle должны оставаться available как partial evidence; invalid model не должен тихо превращаться в supported observed string. Caller-owned typed SDK string без raw source — отдельный low-level input, не обвиняется в coercion.

Adjacent checks: Anthropic native raw message_start model защищён whole-event genuine-string guard; Responses native record guard тоже проверяет string. Gemini actual checked HTTP numeric/bool modelVersion explicit rejects ErrProviderState, positive string passes (`/tmp/task44-errors-r1-gemini-model-probe.log`). Ollama actual HTTP sync/native malformed numeric/bool explicit errors; positive string passes (`/tmp/task44-errors-r1-ollama-model-probe.log`). Таким образом подтверждённый класс локализован, а не приписан всем SDK.

## F2 — P2: retained refusal/incomplete скрывает известную failure telemetry

Locations `ext/otelprompty/telemetry.go:272–284`, `operationSpan.finish:110–120`. classifyError проверяет resp.Outcome refusal/incomplete до известного net.Error и ErrStreamProtocol. finish исключает эти provider categories из Error span status независимо от фактического error cause.

Executed public middleware/in-memory exporter reproduction: custom BYOT Invoker returns `Response{Outcome:Refusal (или Incomplete), Model:"observed", FinishReason:"known"}` together with `*net.DNSError{IsTimeout:true}`. WithTracing preserves original response/error, но exported error.type и prompty.error.category — provider_refusal/provider_incomplete; span Status=Unset. Known transport failure исчезает из machine classification. Error secret text не экспортируется (privacy тут правильная).

`/tmp/task44-errors-r1-otel-probe.log`, own `audit_task44_classification_test.go`, race. Two failing outcome cases. Precedence for ErrStreamProtocol has same directly inspected gap; separate protocol reproduction не объявляется confirmed executed case.

Task44 требует раздельную диагностику outcome и transport failure. Refusal сам по себе или соответствующий OutcomeError не обязан создавать network error, но сохранённый provider outcome не может маскировать независимый explicit transport failure. Нужно сохранить outcome и классифицировать наблюдённую ошибку отдельно.

## F3 — P2: fetch теряет known response metadata при response + error

Locations `ext/otelprompty/operations.go:60–87`: resource path записывает only handle status/provider; known result.Response не участвует в observation normalization/classification/result attrs. Get/Cancel тем самым пропускают observed model/outcome/finish, хотя отсутствие повторного usage можно обеспечить отдельно.

Executed public WrapRemoteResponses.Get/in-memory exporter: original manager returns result.Handle.Status=Completed и Response{Model:"observed", Outcome:Refusal, FinishReason:"refusal"} together with ErrCapture. Wrapper correctly returns original result+error, exports capture_failure and resource completed, но gen_ai.response.model/prompty.outcome/gen_ai.response.finish_reasons отсутствуют. Same nil-error fetch variant ранее выполнен с тем же omission. Final log `/tmp/task44-errors-r1-otel-probe.log` содержит response+capture-error reproducer; own external test `TestAuditTask44FetchRetainsNonUsageMetadata` failing с race.

Requirements2/3 требуют available effective model/outcome/finish, включая response+post-dispatch capture/schema error. Не нужно повторно экспортировать generation token counters или придумывать model generation span для Get. Original Response metadata и resource status — различные факты; их следует сохранить без повторного usage. Cancel shares the inspected resource path, но отдельно не воспроизводился.

Official pinned Fetch response conventions также предусматривают response model/finish/status независимо от inference usage; это согласуется с source contract, а не оправдывает повторный token accounting.

## Full mandatory scope assessed

**Conventions and universal boundary.** Официальный pinned snapshot `4f85037ef86e92c510d2ef881a58f1076f6fc0e4` непосредственно прочитан: [client inference](https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md), [operation spans](https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Development status/pin и implemented subset документированы. Standard input/output/cache-read/cache-write/reasoning attributes сопоставлены с presence counters; total остаётся explicit prompty extension. Default sensitive fields/IDs/URLs omitted даже когда conventions рекомендуют opt-in observation. Существование pinned conventions не доказывает live model support.

**Core model/wire/data integrity.** Reviewed Response.Model optional/observed versus profile/request; ResponseChunk.Model, buffered propagation, final terminal metadata, CloneResponse/deep DTO metadata, optional wire schema/model round-trip. No SDK/OTel dependency added to core. Existing content/state/association schemas и replay semantics остаются. Known unknown/zero usage remains presence-aware, fields не пересчитывают subsets в total. F1 — newly consumed provider field source integrity failure.

**Sync tracing/classification/privacy.** Caller tracer/provider configurable без обязательного global singleton; default fallback explicitly documented. One logical operation span, no fabricated attempt count/transport retries. Known response attrs recorded on error; callback projection receives owned execution/response/status/resource copies; original result/error returned. No default prompt/tools/reasoning/state/account/URL/credential/raw error text/events. Status uses machine categories. In-memory full race suite covers knownzero/unknown/cache/reasoning/effective model/response+error/secret fixtures and ownership; F2 identifies classification precedence gap.

**Streaming/cancellation/concurrency.** ObserveLifecycle stays on original handle; no new producer/Events goroutine. Span begins at actual lazy consumption, unused/Close-before creates no fictitious generation duration; sync.Once finalizes exactly once. StopReason distinguishes consumer, explicit Close and parent cancel; terminal status immutable and Status/observer clones retain known metadata. Reviewed source context/span binding without replacing cancellation channel, original finalizer/capture closeout, error registration fallback and explicit RemoteResult retention. Full race tests exercise terminal/consumer/Close/running cancellation/original handle lifecycle/concurrent stream isolation. New Model scalar integrity native gap recorded in F1; no additional confirmed lifecycle/race bug found.

**Explicit operation ports and ownership.** Reviewed Responses Create/Get/Cancel, separate native streaming port wrapper, batch Submit/Get/Cancel/Results, cache Create/Get/UpdateExpiry/Delete. No implicit optional port, poll/retry/resume/storage, double Events loop or HTTP attempt claims. Resource/job states separate from item outcomes and resource TokenCount distinct from billed generation usage. Repeated Get/Results/cache operations omit generation usage. Resource clones isolate handles/bindings/source JSON/capture/diagnostics/token pointers and callback mutation cannot change original result; errors preserve identity under documented non-mutation rule. F3 identifies too-broad omission of non-usage metadata.

**All provider mappings/regressions.** Reviewed OpenAI Chat sync/native/buffered, Responses parse/native/handles, Anthropic sync/native, Gemini parse/refusal/native metadata and exact source, Ollama complete/partial/native paths. SDK/vendor DTOs remain adapters. Model identity remains distinct from selected profile rather than inference; batching reuses original checked parse contracts and resource boundary. Full suite retains source numeric/state/IsError/correlation/hosted/pause/schema/capture/cancellation regressions from tasks38–43. Adjacent model probes delineate known affected versus rejecting paths.

**Docs/matrix/examples/migration.** Matrix reviewed against implemented ports and offline fixture references across all adapters: native/buffered, local/hosted tools, reasoning/state, strict/original schema, media/function results, batch/cache, checked/raw limitations. Updated adapter README/godoc no longer state local tools/history absent. Root quick-start remains checked. Migration covers clear-break38–43 and extension README old telemetry keys/safe categories/new observed metadata; historical37 evidence unchanged. Runnable OTel operation examples distinguish item/resource billing and full checks compile/test all examples/generated consumers. No live provider certification, pricing, auto-redactor/backend/eval/agentloop/storage claims added.

No additional confirmed errors beyond F1–F3 in reviewed full scope. Probes do not provide exhaustive proof for every malformed transport combination or live model/account. Acceptance requires these3 classes fixed and new frozen full paired audit.
