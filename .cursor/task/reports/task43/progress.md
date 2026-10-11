# Task43 — текущая реализация

Task43 не принята и не закоммичена. Полный scope задания сохранён. Аудиторы полноты/ошибок будут вызваны после завершения реализации на одном immutable snapshot.

На базе `132afc2` реализованы:

- Ollama native Format + Think, отдельные profile gates и original-schema validation sync/native. Actual SDK HTTP/NDJSON fixtures, отрицательные zero-IO случаи, offline example и migration в README.
- Responses reasoning effort/summary и explicit StrictTools/capability. Исходная strict schema проверяется без lowering; arguments остаются original-validated. Actual SDK request обнаружил числовой enum, сериализовавшийся строкой; для tool/output schemas использован существующий точный SDK numeric mapper. Есть offline example и отрицательные fixtures.
- Gemini text/image function results с native references, ID/order/MIME и owned bytes/GCS URI. Gemini API inline-only, Vertex дополнительно GCS. Execution wire → actual SDK sync/SSE fixtures, zero-IO отказ и offline example.
- Универсальный StreamPartFinalization с exact content check, atomic folding/reindexing, sealing и owned state/capture. Не добавляет SDK DTO или agent runtime. Race fixtures проверяют mixed/interleaved parts, ID-less tool correlation, atomic contradiction rejection, state rebinding rejection и retained state после late failure.

Проверки текущего промежуточного кода:

- `/tmp/task43-incremental-targeted-r1.log`: race tests root, adapter, Gemini, Ollama, OpenAI — PASS.
- `checks/incremental-lint-r10.log`: make lint всех 12 модулей — PASS после shared Create refactor.
- `/tmp/task43-test-draft-r2.log`: make test всех модулей + generated package — PASS до добавления общего finalization contract.
- `checks/incremental-test-r4.log`: повторный make test после finalization contract и shared Create refactor всех 12 модулей + generated package — PASS.

Добавлен optional core RemoteResponseStreamer/RemoteResponseStream contract. Sync Create и native CreateStream переиспользуют owned prepareCreate; старый unused preflight wrapper удалён.

Следующий промежуточный участок — `adapter/openai/responses_stream_handle.go` и `responses_sse_source.go`:

- wrapper делегирует Events/Status/Close общему Stream и отдельно сохраняет наблюдаемый remote handle; transport failure/Close не подменяют remote status;
- RemoteResult возвращает owned content/handle/diagnostics, включая итоговый capture report общего lifecycle;
- malformed metadata сохраняет только однозначный допустимый resource ID; смена ID/model, unknown status, queued после in_progress и смена terminal status отвергаются;
- HTTP middleware для SSE сохраняет status и не читает, не заменяет и не закрывает body; декодирование и closure остаются у SDK;
- race fixtures проверяют retained partial result, один terminal, clone isolation, concurrent snapshots, lazy Close и отказ при ambiguous ID. Middleware fixture отдельно проверяет отсутствие preread/close.

Private компоненты подключены к actual SDK CreateStream через `responses_stream.go` и operation-local `responses_sse_machine.go`. Shared Stream выполняет single-use, assembly/finalization, Close/cancellation и один terminal. SDK NewStreaming вызывается только при consumption; retries отключены, background отвергается до IO.

Native SSE fixtures теперь проверяют mixed reasoning + text + local function + hosted web search, interleaved text/function fragments, citations, early/late usage, known zero, original strict arguments и retained encrypted state/handle/usage при premature EOF. Полный ответ использует существующий checked parseRemoteResponse; completed content не публикуется повторно. Encrypted reasoning state финализируется на item.done, если уже доступно; missing provisional state ждёт terminal source.

Actual SDK concurrent fixture держит два тела открытыми, закрывает первый handle, подтверждает Close underlying body и успешное завершение второго. Capture fixtures проверяют semantic events/HTTP status/end summary и сохранение принятого ID при required sink failure. Adversarial SSE проверяет contradictory summary/text/citations, invalid original tool arguments и unknown events. Structured output fixtures отделяют completed schema success от invalid JSON/schema, failed/incomplete и refusal. `ExampleResponses_CreateStream` runnable offline включает mixed output и rejected background с одним SDK request.

Это промежуточная реализация native port; вся streaming conformance ещё не принята. Нужны дальнейшие проверки history/wire continuation, state, lifecycle и протокольных граничных случаев на итоговой поверхности task43.

Проверки stream foundation:

- `checks/incremental-lint-r12.log`: make lint всех 12 модулей — PASS перед последней правкой сохранения diagnostics.
- `checks/openai-stream-foundation-lint-r3.log`: lint обновлённого OpenAI модуля — PASS.
- `checks/openai-stream-foundation-r3.log`: race tests всего OpenAI модуля — PASS на обновлённых файлах.
- `checks/openai-stream-foundation-r2-failed-fixture.log`: предыдущий запуск упал из-за некорректной новой capture fixture (Observed terminal без Coverage/Closed); fixture исправлена согласно CaptureReport.Validate, повторный запуск r3 прошёл. Производственный контракт capture не ослаблялся.

Проверки подключённого SDK SSE port:

- `checks/sse-openai-r9.log`: race tests всего OpenAI модуля с mixed/error/capture/Close/output fixtures и offline example — PASS.
- `checks/incremental-test-r6.log`: make test всех 12 модулей + generated package — PASS после lifecycle usage и последних native output fixtures.
- `checks/incremental-lint-r15.log`: make lint всех 12 модулей — PASS после форматирования одной request fixture. Предыдущие r13/r14 остановились на test-only style/format findings; runtime checks проходили, замечания исправлены.
- git diff --check — PASS. Это промежуточные checks, не snapshot финальной приёмки; task43 не принята и не закоммичена.

Это результаты разработки, не финальная приёмка. После следующих изменений проверки повторяются для принятого snapshot.

Следующая волна: native Responses sealing и универсальный pause outcome.

- Text/function/summary `*.done` закрывает соответствующий delta lifecycle; пустой поздний delta тоже отвергается. `content_part.done` проверяет/сохраняет объявленные citations для subsequent item projection. Actual SDK adversarial fixtures проверяют отказ до terminal response.
- `OutcomePaused` добавлен в core и исполняемый response-wire schema. Anthropic `pause_turn` больше не использует legacy `OutcomeIncomplete`. Structured-output helper не объявляет schema success; typed-tool executor не запускает handlers. Prepared adapter validation сохраняет валидную pause/state, отделяя transport completion от model completion.
- Actual SDK sync fixture: pause + signed/redacted thinking → response wire → restore → caller-invoked continuation с неизменёнными state blocks; ровно один request до explicit resume. Native fixture сохраняет pause/state без automatic resume и continuation revocation.
- `checks/sse-sealing-r2.log`, `checks/paused-anthropic-r1.log`, `checks/paused-core-r1.log` — targeted race PASS.
- `checks/incremental-lint-r16.log`: make lint всех 12 модулей — PASS. После этого изменено только старое test expectation pause_turn → paused и документация.
- `checks/incremental-test-r8.log`: make test всех 12 модулей + generated package — PASS. Предыдущий r7 обнаружил legacy test expectation pause_turn → incomplete; тест обновлён на новый контракт, compatibility fallback не добавлялся.

Canonical hosted declarations и Anthropic request mapping реализованы; hosted results/source-order history и соответствующие native/batch fixtures ещё не реализованы. Pause foundation и request mapping не закрывают весь Anthropic scope task43.

Следующая волна: canonical hosted intent и versioned Anthropic declarations.

- Generic HostedTools перенесён в PromptExecution; прежнее поле RemoteResponseRequest удалено без shim. Clone, execution wire и batch digest сохраняют settings и различают изменения intent. Core shape проверяет nonblank/unique kind и object settings; provider whitelist остаётся в адаптере.
- RuntimeDescriptor отображает implemented kind → capability. Checked preflight запрещает unimplemented mapping даже при caller profile supported; unknown/unsupported profile и forbidden combinations дают zero IO. Descriptor snapshot клонирует map. Direct Translate OpenAI Chat/Gemini/Ollama также отказывает при hosted intent.
- Responses sync/native request preparation и все текущие fixtures используют canonical declarations. RemoteResponseHandle.HostedTools остаётся observed kind list для operation-only Get/Cancel, это иной контракт.
- Anthropic request mapping поддерживает только pinned search 20250305 и fetch 20250910 через SDK typed variants. Whitelist rejects reserved/unknown/code-execution keys, duplicate JSON на любом уровне, nonpositive/fractional/overflow limits, malformed domain lists, invalid location/citations shape и коллизии local names. Профиль проверяется и при direct Translate.
- Actual SDK HTTP fixture подтверждает обе hosted declarations вместе с local tool, owned settings после мутации input, точный integer 9007199254740993 и explicit citations.enabled:false. Негативные fixtures проверяют отказ без SDK request.
- `checks/incremental-test-r9.log`: make test всех 12 модулей + generated package — PASS.
- `checks/incremental-lint-r20.log`: make lint всех 12 модулей — PASS. r17–r19 выявили struct-tag formatting и missing t.Parallel; исправлено без изменения контрактов. Для локального golines нужен --no-reformat-tags, чтобы его tag rewriting не расходился с formatter из lint.
- `checks/hosted-race-r1.log`: ранний targeted run обнаружил устаревший expected ErrRemoteResponse для settings:null; теперь это ErrHostedToolRequest portable shape. Исправлено в fixture, full r9 PASS.

Task43 остаётся in progress: итоговый immutable snapshot двум независимым аудиторам ещё не передавался; коммита task43 нет.

Следующая волна: Anthropic sync hosted response и source-order history.

- Добавлены hosted_source.go/hosted_citations.go/hosted_history.go: pinned server call/search/fetch result/error shapes, optional direct caller, recursive duplicate-key rejection, call/result ID+name linkage, canonical text/citations/local/reasoning projection и source-order replay. Hosted calls не становятся ToolCallPart.
- HostedItems возвращает owned source-verified records. Fetch document читается из RawJSON, поэтому не теряется из SDK generic search Content union. Сохраняются missing page_age/document title, nullable citation titles, encrypted search content/index, точные integers и известные server errors.
- Manifest хранит public native blocks и ссылки/digests для private thinking. Adversarial r2 выявил изменённую signature с прежним text: без digest такая mutation проходила. Добавлен SHA-256 fingerprint payload; r3 проверяет отказ до IO без public signed/redacted payload copy.
- Actual SDK fixture проверяет mixed success/error/citation/local/private content → response wire → execution wire → explicit continuation с исходным native block order. Pause authorizes zero handlers даже для присутствующего local tool. Нет автоматического resume.
- Проверяются изменения role/text/citation/signature/local ID, missing/duplicate/unknown annotation, missing/reordered source, scope mismatch и public signed-state copy. Root descriptor clone/preflight tests подтверждают owned history capability lists и actual native-stream forbidden combination.
- Domain filters принимают bare domain с optional path (example.com/blog), как в актуальных official search docs; SDK HTTP fixture проверяет сохранение path. Scheme/userinfo/query/fragment/blank forms остаются вне выбранного settings contract.
- `checks/hosted-history-r3.log`: targeted Anthropic/adapter race — PASS. `checks/incremental-test-r11.log`: make test всех 12 модулей + generated package — PASS. `checks/incremental-lint-r23.log`: make lint всех 12 модулей — PASS; после последнего test run изменено только семантически эквивалентное разбиение длинного validation condition для formatter.
- `checks/hosted-cross-provider-gap-r1.log` подтверждает незакрытую границу: Anthropic hosted-only pause имеет 2 MessageAnnotations и 0 PartState; OpenAI Chat direct Translate принимает message и теряет hosted metadata. Следующая обязательная правка — generic Scope/Required annotation contract с wire/clone/state-policy/implementation gates и маркировкой Anthropic/OpenAI hosted history. Vendor-kind parsing в core не добавлять.

Native Anthropic hosted SSE и batch/result integration fixtures ещё не завершены. Последние зелёные checks не являются финальной приёмкой.

Следующая волна: универсальная обязательная scoped message metadata.

- MessageAnnotation получил optional owned Scope и Required; Required:true требует валидную provider/endpoint/model-or-family identity. Core не декодирует payload, не проверяет vendor prefixes и не предоставляет account authorization. Shape/scope gate покрывает metadata-only continuation без искусственного content part.
- Common preflight различает профиль и actual mapping: неизвестный Required kind не поддерживается даже при caller profile supported. ProviderStateAdapter теперь вызывается для required annotations при пустом PartState. Prepared dispatch snapshot включает scoped message metadata; оригинальный input mutation не меняет snapshot, повторная проверка перед IO подтверждена fixture.
- StatePreserve сохраняет markers; StateOmit удаляет scoped annotations, оставляет unscoped optional metadata и помечает required omission как ContinuationUnavailable. Scope pointers и Value bytes принадлежат clone/wire/response boundaries. Executable common wire schema проверяет required→scope, core shape проверяет nonblank identity и credential-free endpoint.
- Anthropic/OpenAI Responses маркируют hosted records и manifest как Required scoped metadata; legacy unmarked history не принимается. Chat/Gemini/Ollama direct Translate отвергают отсутствующий mapping. Dedicated Anthropic synthetic profile больше не заявляет HostedTools/hosted history implementation, его parser этого не реализует.
- Context reduction объединяет messages со scoped annotations и part state в одну conservative dependency group без интерпретации provider JSON. Required metadata-only history нельзя удалить отдельным старым turn, если dependency остаётся в current turn.
- Incoming history provider_state gate отделён в Responses от нового encrypted reasoning output intent. Actual SDK hosted-history fixture подтверждает отсутствие automatic reasoning.encrypted_content Include и false handle.ProviderStateRequired; прежние explicit/part-state guarantees сохраняются.
- `checks/hosted-cross-provider-fixed-r2.log`: тот же сохранённый репро теперь показывает source annotations=2, PartState=0, foreign Translate accepted=false, ErrUnsupportedCapability; сеть не вызывается. Подтверждённый gap предыдущей волны закрыт.
- `checks/scoped-annotations-r4.log`: targeted race core/adapter/OpenAI/Anthropic/truncate — PASS. `checks/incremental-test-r12.log`: make test всех 12 модулей + generated package — PASS. `checks/incremental-lint-r25.log`: make lint всех 12 модулей — PASS; последняя extra assertion об отсутствии automatic encrypted Include проверена targeted tests и локальным OpenAI lint.
- Early r1 нашёл старое profile knowledge в hosted fixture/example: добавлен явный provider_state guarantee вместо fallback/inference. Lint r24 выявил cognitive complexity и embedded-field spacing; validation loops вынесены в helpers без сокращения проверок, r25 PASS.

Task43 остаётся in progress; двойная финальная приёмка не запускалась и task43 не закоммичена. Следующий implementation scope: native Anthropic hosted SSE/source-order final manifest и batch conformance, затем оставшиеся Responses lifecycle/history fixtures и полная документация/приёмка.

Остаётся выполнить полностью:

1. Завершить conformance native Responses CreateStream: history/wire continuation и lifecycle/state/protocol boundaries, включая terminal-only encrypted state и parent cancellation. Полный исходный acceptance task43 остаётся обязательным; описанные выше fixtures не заменяют двойной аудит.
2. Закончить Anthropic hosted native stream и sync/native/batch conformance вместе с реализованными declarations/clone/wire/digest/OutcomePaused/scoped annotations; actual SDK fixtures с нулевым local dispatch.
3. Согласовать итоговые публичные contracts/docs/matrix и всю conformance поверхность задания, убрать заменённые legacy routes, выполнить финальные проверки и двойной независимый аудит.

Архитектурные консультанты `/root/task43_stream_contract` и `/root/task43_anthropic_contract` читали код и SDK, файлов не меняли. В финальной независимой приёмке они не участвуют.

## Additional native conformance and batch/example work

- OpenAI actual SDK native→wire→explicit replay covers state at item.done and terminal-only encrypted state; parent cancellation closes body and retains observed handle/state/usage. New malformed lifecycle/usage fixtures found SDK numeric coercion/omission; raw SSE usage validation returns ErrUsage while retaining valid counters. Full module race/lint PASS: responses-conformance-r1.log, responses-conformance-lint-r1.log.
- Anthropic hosted batch fixture covers exact versioned SDK declarations, mixed hosted results/errors, paused generation within succeeded batch item, wire restore and checked history translation. Changed hosted settings cannot rebind accepted work, and forbidden batch+hosted profile combination gives zero IO. Offline ExampleHostedItems covers explicit resume and unknown-kind zero IO. Targeted race PASS: hosted-batch-example-r3.log.
- Core failure/Close previously revoked continuation only with content. Hosted-only scoped annotations now also revoke it; evidence remains available. New error/break/Close tests and scoped state tests pass race; core lint PASS: core-scoped-stream-r1.log, core-scoped-stream-lint-r2.log.
- Public docs/provider-capability-matrix.md describes mappings, endpoint/model gates, wire migration and live verification boundary. Anthropic native implementation/conformance is being completed by /root/anthropic_native43. /root/responses_conformance43 participated in implementation. Neither will accept task43 as an independent auditor.

Task43 remains unaccepted/uncommitted. Required next gate: finish native Anthropic tests, full make test/lint, then freeze a shared snapshot and perform both independent audits. Task44 has not started.

## Independent acceptance r1 and corrections

Paired independent r1 reviewed one800file frozen snapshot. Completeness rejected19/24=79.17%; errors rejected2confirmedP1 classes. Both independently reproduced hostile Responses negative summary index and malformed Anthropic delta source acceptance using actual SDK HTTP/SSE. Full clean-copy checks passed, illustrating why adversarial acceptance is required. Reports: completeness-r1.md and errors-r1.md. No implementation changed during audits.

Freeze lifted only after both reports. /root/responses_conformance43 owns guarded summary indices/regression tests; /root/anthropic_native43 owns raw native event source validation/regressions. Root owns closeout docs/checks. Neither implementer will accept r2. The next gate retains full denominator24 and scope; task44 remains untouched.

## Corrections ready for paired r2

- Responses: validSummaryIndex now requires declared lifecycle/nonnegative index/all slice bounds;27 actual SDK hostile cases verify no panic, one terminal, known prefix/handle/usage and body close. Adjacent actual SDK probes confirmed9 scalar coercion variants; raw consumed source type validation rejects numeric text and string/fractional indices before mutation.11 source regressions and full OpenAI race/lint pass; responses-source-types-r2-{test,lint}.log.
- Anthropic: recursive source uniqueness and exact required consumed types checked before mutation for all supported deltas, identities/index/stop reason/usage. Confirmed hosted partial_json:null after valid full object now fails; validemptystrings/additional optional fields/usage-onlyupdates remain valid. Full module race/lint pass; anthropic-source-r2-{race,lint}.log.
- R2 frozen snapshot805files: snapshot-r2.json SHA256 fba7ed2e47262acd7ad3f876cc02fcc469c67c50d420152adc33b2fff0315d3b. Full make test/lint r2 running. No implementation edits during r2; after checks, both independent auditors assess all24 obligations again.
